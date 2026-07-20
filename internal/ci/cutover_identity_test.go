package ci

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"
)

type cutoverIdentityContract struct {
	SchemaVersion         int                    `json:"schema_version"`
	FrozenBase            string                 `json:"frozen_base"`
	ProtectedFiles        []classifiedFile       `json:"protected_files"`
	TextOccurrences       []classifiedOccurrence `json:"text_occurrences"`
	IdentityPaths         []classifiedPath       `json:"identity_paths"`
	SemanticFileSHA256    map[string]string      `json:"semantic_file_sha256"`
	BehavioralEquivalence behavioralEquivalence  `json:"behavioral_equivalence"`
}

type classifiedFile struct {
	PathBase64 string `json:"path_b64"`
	SHA256     string `json:"sha256"`
}

type classifiedOccurrence struct {
	PathBase64 string `json:"path_b64"`
	LineSHA256 string `json:"line_sha256"`
	TokenCount int    `json:"token_count"`
	Class      string `json:"class"`
}

type classifiedPath struct {
	PathBase64 string `json:"path_b64"`
	TokenCount int    `json:"token_count"`
	Class      string `json:"class"`
}

type behavioralEquivalence struct {
	PackTestNormalizedSHA256 string            `json:"pack_test_normalized_sha256"`
	RootTestFunctions        map[string]string `json:"root_test_functions"`
}

type semanticPack struct {
	SchemaVersion int                `json:"schema_version"`
	ID            string             `json:"id"`
	Version       string             `json:"version"`
	Provides      []string           `json:"provides"`
	Requires      semanticRequires   `json:"requires"`
	Conflicts     []string           `json:"conflicts"`
	Resources     []semanticResource `json:"resources"`
}

type semanticRequires struct {
	Capabilities []string `json:"capabilities"`
	Tools        []string `json:"tools"`
}

type semanticResource struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Source string `json:"source"`
}

type semanticSources struct {
	SchemaVersion int `json:"schema_version"`
	Sources       []struct {
		ID         string `json:"id"`
		Provider   string `json:"provider"`
		Repository string `json:"repository"`
		Selector   struct {
			Mode string `json:"mode"`
		} `json:"selector"`
		Resources []struct {
			PackID       string `json:"pack_id"`
			Kind         string `json:"kind"`
			ResourceID   string `json:"resource_id"`
			UpstreamPath string `json:"upstream_path"`
		} `json:"resources"`
	} `json:"sources"`
}

type semanticLock struct {
	SchemaVersion    int    `json:"schema_version"`
	Generator        string `json:"generator"`
	GeneratorVersion string `json:"generator_version"`
	SourceID         string `json:"source_id"`
	Resources        []struct {
		PackID       string `json:"pack_id"`
		Kind         string `json:"kind"`
		ResourceID   string `json:"resource_id"`
		UpstreamPath string `json:"upstream_path"`
	} `json:"resources"`
}

func TestProtectedCutoverHistoryMatchesFrozenBase(t *testing.T) {
	skipRepositoryCutoverAssertions(t)
	root := cutoverRepoRoot(t)
	contract := loadCutoverIdentityContract(t, root)
	if contract.FrozenBase != "0e8971ad4ccacad5f99ec97d05ed963830b58070" {
		t.Fatalf("unexpected frozen base %q", contract.FrozenBase)
	}

	want := make(map[string]string, len(contract.ProtectedFiles))
	for _, file := range contract.ProtectedFiles {
		path := decodeContractPath(t, file.PathBase64)
		want[path] = file.SHA256
	}

	got := map[string]string{}
	for _, path := range cutoverWorktreePaths(t, root) {
		if !isProtectedCutoverPath(path) {
			continue
		}
		got[path] = fileSHA256(t, filepath.Join(root, filepath.FromSlash(path)))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("protected history differs from frozen base\nwant: %s\n got: %s", formatStringMap(want), formatStringMap(got))
	}
}

func TestRemainingIdentitySurfaceMatchesExactClassification(t *testing.T) {
	skipNestedValidationFixture(t)
	root := cutoverRepoRoot(t)
	contract := loadCutoverIdentityContract(t, root)
	token := legacyIdentityToken()
	allowedClasses := map[string]bool{
		"frozen-baseline-equivalence": true,
		"historical-cutover-record":   true,
		"legacy-isolation-guard":      true,
		"legacy-schema-id-rejection":  true,
		"release-formula-removal":     true,
		"semantic-pack":               true,
	}

	wantOccurrences := make([]string, 0, len(contract.TextOccurrences))
	for _, occurrence := range contract.TextOccurrences {
		if omitFromExactIdentityComparison(decodeContractPath(t, occurrence.PathBase64)) {
			continue
		}
		if !allowedClasses[occurrence.Class] {
			t.Fatalf("classified occurrence has unsupported class %q", occurrence.Class)
		}
		wantOccurrences = append(wantOccurrences, occurrenceKey(
			decodeContractPath(t, occurrence.PathBase64),
			occurrence.LineSHA256,
			occurrence.TokenCount,
			occurrence.Class,
		))
	}
	sort.Strings(wantOccurrences)

	wantPaths := make([]string, 0, len(contract.IdentityPaths))
	for _, path := range contract.IdentityPaths {
		if omitFromExactIdentityComparison(decodeContractPath(t, path.PathBase64)) {
			continue
		}
		if !allowedClasses[path.Class] {
			t.Fatalf("classified identity path has unsupported class %q", path.Class)
		}
		wantPaths = append(wantPaths, pathKey(decodeContractPath(t, path.PathBase64), path.TokenCount, path.Class))
	}
	sort.Strings(wantPaths)

	classByOccurrence := map[string][]string{}
	for _, occurrence := range contract.TextOccurrences {
		baseKey := occurrenceBaseKey(decodeContractPath(t, occurrence.PathBase64), occurrence.LineSHA256, occurrence.TokenCount)
		classByOccurrence[baseKey] = append(classByOccurrence[baseKey], occurrence.Class)
	}
	classByPath := map[string]string{}
	for _, path := range contract.IdentityPaths {
		classByPath[fmt.Sprintf("%s\x00%d", decodeContractPath(t, path.PathBase64), path.TokenCount)] = path.Class
	}

	var gotOccurrences []string
	var gotPaths []string
	for _, path := range cutoverWorktreePaths(t, root) {
		if isProtectedCutoverPath(path) || omitFromExactIdentityComparison(path) {
			continue
		}
		pathCount := strings.Count(strings.ToLower(path), token)
		if pathCount > 0 {
			class := classByPath[fmt.Sprintf("%s\x00%d", path, pathCount)]
			gotPaths = append(gotPaths, pathKey(path, pathCount, class))
		}

		data := readWorktreeFile(t, filepath.Join(root, filepath.FromSlash(path)))
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			continue
		}
		for _, line := range bytes.Split(data, []byte{'\n'}) {
			count := strings.Count(strings.ToLower(string(line)), token)
			if count == 0 {
				continue
			}
			lineHash := sha256Hex(line)
			baseKey := occurrenceBaseKey(path, lineHash, count)
			classes := classByOccurrence[baseKey]
			class := ""
			if len(classes) > 0 {
				class = classes[0]
				classByOccurrence[baseKey] = classes[1:]
			}
			gotOccurrences = append(gotOccurrences, occurrenceKey(path, lineHash, count, class))
		}
	}
	sort.Strings(gotOccurrences)
	sort.Strings(gotPaths)

	if !reflect.DeepEqual(gotOccurrences, wantOccurrences) {
		t.Fatalf("textual identity surface is not exhaustively classified\nwant: %s\n got: %s", strings.Join(wantOccurrences, "\n"), strings.Join(gotOccurrences, "\n"))
	}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("path-name identity surface is not exhaustively classified\nwant: %s\n got: %s", strings.Join(wantPaths, "\n"), strings.Join(gotPaths, "\n"))
	}
}

func omitFromExactIdentityComparison(path string) bool {
	return stagedValidation() && path == "bundle/sources.json"
}

func TestSemanticPackIdentitySurvivesFieldByField(t *testing.T) {
	skipNestedValidationFixture(t)
	root := cutoverRepoRoot(t)
	contract := loadCutoverIdentityContract(t, root)
	token := legacyIdentityToken()
	semanticPaths := map[string]string{
		"guidance":             filepath.Join("bundle", "instructions", token+"-guidance.md"),
		"workflow_conventions": filepath.Join("bundle", "instructions", token+"-workflow-conventions.md"),
		"pack_manifest":        filepath.Join("bundle", "packs", token, "pack.json"),
		"sources":              filepath.Join("bundle", "sources.json"),
		"sources_lock":         filepath.Join("bundle", "sources/mattpocock-skills.lock.json"),
	}
	if !stagedValidation() {
		for role, rel := range semanticPaths {
			if role == "sources" {
				continue
			}
			if got, want := fileSHA256(t, filepath.Join(root, rel)), contract.SemanticFileSHA256[role]; got != want {
				t.Fatalf("semantic file %s changed: got %s want %s", role, got, want)
			}
		}
	}

	var pack semanticPack
	readJSONFile(t, filepath.Join(root, semanticPaths["pack_manifest"]), &pack)
	if pack.SchemaVersion != 1 || pack.ID != token || pack.Version == "" || (!stagedValidation() && pack.Version != "2.0.0") {
		t.Fatalf("semantic pack identity changed: schema=%d id=%q version=%q", pack.SchemaVersion, pack.ID, pack.Version)
	}
	if !reflect.DeepEqual(pack.Provides, []string{"workflow:" + token}) || len(pack.Requires.Capabilities) != 0 || len(pack.Requires.Tools) != 0 || len(pack.Conflicts) != 0 {
		t.Fatalf("semantic pack contracts changed: provides=%v requires=%+v conflicts=%v", pack.Provides, pack.Requires, pack.Conflicts)
	}
	expectedResources := expectedSemanticResources(token)
	if !reflect.DeepEqual(pack.Resources, expectedResources) {
		t.Fatalf("semantic pack resources changed\nwant: %+v\n got: %+v", expectedResources, pack.Resources)
	}

	var sources semanticSources
	readJSONFile(t, filepath.Join(root, semanticPaths["sources"]), &sources)
	if sources.SchemaVersion != 1 || len(sources.Sources) == 0 {
		t.Fatalf("semantic source envelope changed: %+v", sources)
	}
	expectedSourceID := strings.TrimSuffix(filepath.Base(semanticPaths["sources_lock"]), ".lock.json")
	sourceIndex := -1
	for index, candidate := range sources.Sources {
		if candidate.ID == expectedSourceID {
			sourceIndex = index
			break
		}
	}
	if sourceIndex < 0 {
		t.Fatalf("expected semantic source is missing: %+v", sources)
	}
	source := sources.Sources[sourceIndex]
	if source.ID != expectedSourceID || source.Provider != "github" || source.Repository != "mattpocock/skills" || source.Selector.Mode != "stable-release" {
		t.Fatalf("semantic source identity changed: %+v", source)
	}
	assertSemanticSourceResources(t, token, expectedResources, source.Resources)

	var lock semanticLock
	readJSONFile(t, filepath.Join(root, semanticPaths["sources_lock"]), &lock)
	if lock.SchemaVersion != 1 || lock.Generator != "packy-packsync" || lock.GeneratorVersion != "1" || lock.SourceID != source.ID {
		t.Fatalf("source lock product/semantic boundary changed: %+v", lock)
	}
	if len(lock.Resources) != len(source.Resources) {
		t.Fatalf("source lock resource count = %d, want %d", len(lock.Resources), len(source.Resources))
	}
	sourceResources := make(map[string]string, len(source.Resources))
	for _, resource := range source.Resources {
		sourceResources[resource.Kind+"\x00"+resource.ResourceID] = resource.UpstreamPath
	}
	for i, resource := range lock.Resources {
		key := resource.Kind + "\x00" + resource.ResourceID
		wantPath, ok := sourceResources[key]
		if resource.PackID != token || !ok || resource.UpstreamPath != wantPath {
			t.Fatalf("source lock resource %d changed or is not declared by the source: %+v", i, resource)
		}
		delete(sourceResources, key)
	}
	if len(sourceResources) != 0 {
		t.Fatalf("source resources missing from lock: %v", sourceResources)
	}
}

func TestClassifiedProductRenamePreservesBaselineBehavioralTests(t *testing.T) {
	skipNestedValidationFixture(t)
	root := cutoverRepoRoot(t)
	contract := loadCutoverIdentityContract(t, root)

	packTest := readWorktreeFile(t, filepath.Join(root, "internal", "cli", "pack_test.go"))
	if got, want := sha256Hex(reverseProductIdentity(packTest)), contract.BehavioralEquivalence.PackTestNormalizedSHA256; got != want {
		t.Fatalf("normalized pack CLI behavioral suite differs from frozen baseline: got %s want %s", got, want)
	}

	rootTestPath := filepath.Join(root, "internal", "cli", "root_test.go")
	rootTest := readWorktreeFile(t, rootTestPath)
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, rootTestPath, rootTest, 0)
	if err != nil {
		t.Fatalf("parse root CLI tests: %v", err)
	}
	got := map[string]string{}
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil {
			continue
		}
		if _, required := contract.BehavioralEquivalence.RootTestFunctions[function.Name.Name]; !required {
			continue
		}
		start := fset.Position(function.Pos()).Offset
		end := fset.Position(function.End()).Offset
		got[function.Name.Name] = sha256Hex(reverseProductIdentity(rootTest[start:end]))
	}
	if !reflect.DeepEqual(got, contract.BehavioralEquivalence.RootTestFunctions) {
		t.Fatalf("normalized lifecycle behavioral tests differ from frozen baseline\nwant: %s\n got: %s", formatStringMap(contract.BehavioralEquivalence.RootTestFunctions), formatStringMap(got))
	}
}

func skipNestedValidationFixture(t *testing.T) {
	t.Helper()
	if os.Getenv("PACKY_VALIDATION_NESTED") == "1" {
		t.Skip("top-level validation already executes the cutover acceptance contract")
	}
}

func skipRepositoryCutoverAssertions(t *testing.T) {
	t.Helper()
	skipNestedValidationFixture(t)
	if stagedValidation() {
		t.Skip("copied staged checkout does not contain the frozen repository history")
	}
}

func stagedValidation() bool {
	return os.Getenv("PACKY_VALIDATION_STAGED") == "1"
}

func expectedSemanticResources(token string) []semanticResource {
	skills := []string{
		"ask-matt", "code-review", "codebase-design", "diagnosing-bugs", "domain-modeling",
		"grill-with-docs", "implement", "improve-codebase-architecture", "prototype", "research",
		"resolving-merge-conflicts", "setup-matt-pocock-skills", "tdd", "to-spec", "to-tickets",
		"triage", "wayfinder", "loop-me", "grill-me", "grilling", "handoff", "teach",
		"writing-great-skills",
	}
	resources := make([]semanticResource, 0, len(skills)+2)
	for _, id := range skills {
		group := "engineering"
		if id == "loop-me" {
			group = "in-progress"
		} else if id == "grill-me" || id == "grilling" || id == "handoff" || id == "teach" || id == "writing-great-skills" {
			group = "productivity"
		}
		resources = append(resources, semanticResource{Kind: "skill", ID: id, Source: filepath.ToSlash(filepath.Join("skills", group, id))})
	}
	resources = append(resources,
		semanticResource{Kind: "instruction", ID: token + "-guidance", Source: "instructions/" + token + "-guidance.md"},
		semanticResource{Kind: "instruction", ID: token + "-workflow-conventions", Source: "instructions/" + token + "-workflow-conventions.md"},
	)
	return resources
}

func assertSemanticSourceResources(t *testing.T, token string, packResources []semanticResource, got []struct {
	PackID       string `json:"pack_id"`
	Kind         string `json:"kind"`
	ResourceID   string `json:"resource_id"`
	UpstreamPath string `json:"upstream_path"`
}) {
	t.Helper()
	var want []semanticResource
	for _, resource := range packResources {
		if resource.Kind == "skill" {
			want = append(want, resource)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("semantic source resources = %d, want %d", len(got), len(want))
	}
	wantByID := make(map[string]semanticResource, len(want))
	for _, resource := range want {
		wantByID[resource.ID] = resource
	}
	for i, resource := range got {
		expected, ok := wantByID[resource.ResourceID]
		if !ok || resource.PackID != token || resource.Kind != expected.Kind || resource.UpstreamPath != expected.Source {
			t.Fatalf("semantic source resource %d changed: got %+v want %+v", i, resource, expected)
		}
		delete(wantByID, resource.ResourceID)
	}
	if len(wantByID) != 0 {
		t.Fatalf("semantic source resources are missing: %+v", wantByID)
	}
}

func loadCutoverIdentityContract(t *testing.T, root string) cutoverIdentityContract {
	t.Helper()
	var contract cutoverIdentityContract
	readJSONFile(t, filepath.Join(root, "internal", "ci", "testdata", "packy-cutover-identity-v1.json"), &contract)
	if contract.SchemaVersion != 1 {
		t.Fatalf("identity contract schema version = %d, want 1", contract.SchemaVersion)
	}
	return contract
}

func readJSONFile(t *testing.T, path string, destination any) {
	t.Helper()
	data := readWorktreeFile(t, path)
	if err := json.Unmarshal(data, destination); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func cutoverRepoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve acceptance test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func cutoverWorktreePaths(t *testing.T, root string) []string {
	t.Helper()
	if stagedValidation() {
		return cutoverCheckoutPaths(t, root)
	}
	command := exec.Command("git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("list worktree files: %v", err)
	}
	parts := bytes.Split(output, []byte{0})
	paths := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		path := filepath.ToSlash(string(part))
		if !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

func cutoverCheckoutPaths(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatalf("list copied staged checkout files: %v", err)
	}
	sort.Strings(paths)
	return paths
}

func isProtectedCutoverPath(path string) bool {
	token := legacyIdentityToken()
	for _, prefix := range []string{
		".scratch/",
		"bundle/history/" + token + "/",
		"bundle/compatibility/" + token + "/",
	} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	for number := 1; number <= 9; number++ {
		if strings.HasPrefix(path, fmt.Sprintf("docs/adr/%04d-", number)) {
			return true
		}
	}
	return false
}

func legacyIdentityToken() string {
	return strings.Join([]string{"ma", "tty"}, "")
}

func reverseProductIdentity(data []byte) []byte {
	token := legacyIdentityToken()
	replacements := [][2]string{
		{strings.ToUpper("packy"), strings.ToUpper(token)},
		{"Packy", strings.ToUpper(token[:1]) + token[1:]},
		{"packy", token},
	}
	result := append([]byte(nil), data...)
	for _, replacement := range replacements {
		result = bytes.ReplaceAll(result, []byte(replacement[0]), []byte(replacement[1]))
	}
	return result
}

func decodeContractPath(t *testing.T, encoded string) string {
	t.Helper()
	decoded, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode contract path %q: %v", encoded, err)
	}
	return filepath.ToSlash(string(decoded))
}

func readWorktreeFile(t *testing.T, path string) []byte {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			t.Fatalf("read link %s: %v", path, err)
		}
		return []byte(target)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	return sha256Hex(readWorktreeFile(t, path))
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func occurrenceBaseKey(path, lineHash string, count int) string {
	return fmt.Sprintf("%s\x00%s\x00%d", path, lineHash, count)
}

func occurrenceKey(path, lineHash string, count int, class string) string {
	return fmt.Sprintf("%s class=%q sha256=%s count=%d", path, class, lineHash, count)
}

func pathKey(path string, count int, class string) string {
	return fmt.Sprintf("%s class=%q count=%d", path, class, count)
}

func formatStringMap(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var lines []string
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s=%s", key, values[key]))
	}
	return strings.Join(lines, "\n")
}

func TestStagedExactIdentityComparisonOmitsOnlyCanonicalSources(t *testing.T) {
	t.Setenv("PACKY_VALIDATION_STAGED", "1")
	if !omitFromExactIdentityComparison("bundle/sources.json") {
		t.Fatal("staged exact identity comparison retained canonical sources")
	}
	semanticSourceLock := "bundle/sources/" + strings.Join([]string{"ma", "tty"}, "") + ".lock.json"
	for _, path := range []string{"bundle/packs.json", semanticSourceLock, "docs/sources.json"} {
		if omitFromExactIdentityComparison(path) {
			t.Fatalf("staged exact identity comparison omitted %s", path)
		}
	}

	t.Setenv("PACKY_VALIDATION_STAGED", "")
	if omitFromExactIdentityComparison("bundle/sources.json") {
		t.Fatal("normal exact identity comparison omitted canonical sources")
	}
}
