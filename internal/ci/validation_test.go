package ci_test

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

var mattyOwnedPackages = []string{
	"./cmd/matty",
	"./internal/bootstrap",
	"./internal/bundletransaction",
	"./internal/capabilitypack",
	"./internal/ci",
	"./internal/cli",
	"./internal/codex",
	"./internal/corelifecycle",
	"./internal/engrambin",
	"./internal/localprojection",
	"./internal/opencode",
	"./internal/ownedcontainer",
	"./internal/packclassification",
	"./internal/packsync",
	"./internal/packsync/githubsource",
	"./internal/prompt",
	"./internal/release",
	"./internal/setuphealth",
	"./internal/skillbundle",
	"./internal/tools/syncpacksource",
	"./internal/version",
	"./internal/workstation",
}

func TestValidationEntrypointOwnsTheExactPackageAllowlist(t *testing.T) {
	root := repositoryRoot(t)
	script := readFile(t, filepath.Join(root, "scripts", "validate-matty.sh"))

	packages := shellArray(t, script, "readonly packages=(")
	if !reflect.DeepEqual(packages, mattyOwnedPackages) {
		t.Fatalf("validation package allowlist = %#v, want %#v", packages, mattyOwnedPackages)
	}
	for _, forbidden := range []string{"./" + "...", "bundle/", ".scratch/"} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("validation entrypoint contains non-allowlisted discovery path %q", forbidden)
		}
	}
	for _, command := range []string{"gofmt -l", "go build", "go vet", "go test", "go test -race"} {
		if !strings.Contains(script, command) {
			t.Fatalf("validation entrypoint missing %q", command)
		}
	}
	wantCommands := []string{
		`go_cache="${GOCACHE:-$(go env GOCACHE)}"`,
		`go_mod_cache="${GOMODCACHE:-$(go env GOMODCACHE)}"`,
		`go_path="${GOPATH:-$(go env GOPATH)}"`,
		`unformatted="$(gofmt -l "${go_files[@]}")"`,
		`go build "${build_packages[@]}"`,
		`go vet "${packages[@]}"`,
		`go test "${packages[@]}"`,
		`go test -race -timeout 10m "${packages[@]}"`,
	}
	if commands := validationCommands(script); !reflect.DeepEqual(commands, wantCommands) {
		t.Fatalf("validation commands = %#v, want only %#v", commands, wantCommands)
	}
}

func TestCIUsesOnlyTheValidationEntrypoint(t *testing.T) {
	workflow := readFile(t, filepath.Join(repositoryRoot(t), ".github", "workflows", "ci.yml"))
	if strings.Count(workflow, "run: ./scripts/validate-matty.sh") != 1 {
		t.Fatal("CI must invoke the repository validation authority exactly once")
	}
	for _, unsafe := range []string{"go test", "go vet", "go build", "gofmt"} {
		if strings.Contains(workflow, "run: "+unsafe) {
			t.Fatalf("CI bypasses validation entrypoint with %q", unsafe)
		}
	}
}

func TestValidationEntrypointIgnoresHostileUnownedGoContent(t *testing.T) {
	if os.Getenv("MATTY_VALIDATION_NESTED") == "1" {
		t.Skip("nested validation invoked by hostile-content tracer")
	}

	sourceRoot := repositoryRoot(t)
	tempRoot := filepath.Join(t.TempDir(), "repo")
	copyRepository(t, sourceRoot, tempRoot)

	writeFile(t, filepath.Join(tempRoot, "bundle", "hostile-load", "broken.go"), "package hostile\nfunc broken(\n")
	sentinel := filepath.Join(tempRoot, "hostile-executed")
	writeFile(t, filepath.Join(tempRoot, "bundle", "hostile-execute", "hostile_test.go"), `package hostile

import (
	"os"
	"testing"
)

func TestHostile(t *testing.T) {
	if err := os.WriteFile(os.Getenv("HOSTILE_SENTINEL"), []byte("executed"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Fatal("vendored upstream test was executed")
}
`)
	writeFile(t, filepath.Join(tempRoot, ".scratch", "hostile", "broken.go"), "package hostile\nfunc broken(\n")

	operatorHome := filepath.Join(tempRoot, "operator-home")
	operatorXDG := filepath.Join(tempRoot, "operator-xdg")
	cmd := exec.Command("bash", filepath.Join(tempRoot, "scripts", "validate-matty.sh"))
	cmd.Dir = tempRoot
	cmd.Env = append(os.Environ(),
		"HOME="+operatorHome,
		"XDG_CONFIG_HOME="+operatorXDG,
		"GOCACHE="+goEnv(t, "GOCACHE"),
		"GOMODCACHE="+goEnv(t, "GOMODCACHE"),
		"GOPATH="+goEnv(t, "GOPATH"),
		"HOSTILE_SENTINEL="+sentinel,
		"MATTY_VALIDATION_NESTED=1",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("validation entrypoint failed with hostile unowned content: %v\n%s", err, output)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("hostile vendored test executed: %v", err)
	}
	for _, path := range []string{operatorHome, operatorXDG} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("validation wrote operator path %s: %v", path, err)
		}
	}
}

func goEnv(t *testing.T, key string) string {
	t.Helper()
	output, err := exec.Command("go", "env", key).CombinedOutput()
	if err != nil {
		t.Fatalf("go env %s: %v: %s", key, err, output)
	}
	return strings.TrimSpace(string(output))
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate validation test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}

func shellArray(t *testing.T, script, opening string) []string {
	t.Helper()
	start := strings.Index(script, opening)
	if start < 0 {
		t.Fatalf("validation entrypoint missing %q", opening)
	}
	after, found := strings.CutPrefix(script[start:], opening)
	if !found {
		t.Fatalf("validation entrypoint missing %q", opening)
	}
	body, _, found := strings.Cut(after, "\n)")
	if !found {
		t.Fatalf("validation entrypoint has unterminated %q", opening)
	}
	return strings.Fields(body)
}

func validationCommands(script string) []string {
	var commands []string
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "go ") || strings.Contains(line, "$(go ") || strings.Contains(line, "gofmt ") {
			commands = append(commands, line)
		}
	}
	return commands
}

func copyRepository(t *testing.T, sourceRoot, destinationRoot string) {
	t.Helper()
	err := filepath.Walk(sourceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		if info.IsDir() && (relative == ".git" || relative == ".codegraph" || relative == ".scratch") {
			return filepath.SkipDir
		}
		destination := filepath.Join(destinationRoot, relative)
		if info.IsDir() {
			return os.MkdirAll(destination, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		destinationFile, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			source.Close()
			return err
		}
		_, copyErr := io.Copy(destinationFile, source)
		sourceCloseErr := source.Close()
		closeErr := destinationFile.Close()
		if copyErr != nil {
			return copyErr
		}
		if sourceCloseErr != nil {
			return sourceCloseErr
		}
		return closeErr
	})
	if err != nil {
		t.Fatalf("copy repository fixture: %v", err)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
