package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type cliSource struct {
	name string
	text string
}

func TestCLIWorkstationLayoutOwnershipIsContracted(t *testing.T) {
	if _, err := os.Stat("paths.go"); !os.IsNotExist(err) {
		t.Fatalf("obsolete shared CLI layout file paths.go still exists")
	}

	knownArtifactParts := map[string]bool{
		".packy": true, "config.json": true, "packs.json": true,
		".agents": true, "skills": true,
		".local": true, "share": true, "packy": true,
		".codex": true, "config.toml": true, "AGENTS.md": true,
		"opencode": true, "opencode.json": true, "packy.md": true,
		"bin": true, "engram": true,
	}
	knownArtifactText := map[string]bool{
		".packy": true, "config.json": true, "packs.json": true,
		".agents": true, ".codex": true, "config.toml": true,
		"AGENTS.md": true, "opencode.json": true, "packy.md": true,
	}

	for _, source := range cliGoSources(t) {
		if strings.HasSuffix(source.name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(token.NewFileSet(), source.name, source.text, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", source.name, err)
		}
		imports := importedPackages(file)
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.FuncDecl:
				if map[string]bool{"ResolvePaths": true, "DefaultInstalledSourceRoot": true, "resolveSkillSourceRoot": true}[declaration.Name.Name] {
					t.Errorf("%s reintroduced obsolete shared layout function %s", source.name, declaration.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range declaration.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if ok && map[string]bool{"Paths": true, "SkillSource": true, "SkillSourceOrigin": true}[typeSpec.Name.Name] {
						t.Errorf("%s reintroduced obsolete shared layout type %s", source.name, typeSpec.Name.Name)
					}
				}
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.CallExpr:
				if isImportedCall(node.Fun, imports, "path/filepath", "Join") {
					for _, arg := range node.Args {
						literal, ok := arg.(*ast.BasicLit)
						if !ok || literal.Kind != token.STRING {
							continue
						}
						value, err := strconv.Unquote(literal.Value)
						if err == nil && knownArtifactParts[value] {
							t.Errorf("%s derives known artifact layout in CLI through filepath.Join component %q", source.name, value)
						}
					}
				}
				if isImportedCall(node.Fun, imports, "fmt", "Sprintf") && containsArtifactLiteral(node, knownArtifactText) {
					t.Errorf("%s derives known artifact layout in CLI through fmt.Sprintf", source.name)
				}
			case *ast.BinaryExpr:
				if node.Op == token.ADD && containsArtifactLiteral(node, knownArtifactText) {
					t.Errorf("%s derives known artifact layout in CLI through string concatenation", source.name)
				}
			}
			return true
		})
	}
}

func TestCLISourceSelectionHasOneSharedProductionRoute(t *testing.T) {
	var installedSourceResolutions, skillSourceResolutions int
	for _, source := range cliGoSources(t) {
		if strings.HasSuffix(source.name, "_test.go") {
			continue
		}
		installedSourceResolutions += strings.Count(source.text, "bootstrap.ResolveInstalledSource(")
		skillSourceResolutions += strings.Count(source.text, "skillbundle.ResolveSource(")
	}
	if installedSourceResolutions != 2 {
		t.Fatalf("CLI has %d Installed Source resolution routes, want init plus one shared command route", installedSourceResolutions)
	}
	if skillSourceResolutions != 1 {
		t.Fatalf("CLI has %d Skill Source selection routes, want one shared command route", skillSourceResolutions)
	}
}

func containsArtifactLiteral(node ast.Node, known map[string]bool) bool {
	found := false
	ast.Inspect(node, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil {
			return true
		}
		for artifact := range known {
			if value == artifact || strings.Contains(value, "/"+artifact) || strings.Contains(value, artifact+"/") {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func TestAmbientWorkstationReadsStayAtApprovedProcessEdges(t *testing.T) {
	allowed := map[string]map[string]bool{
		filepath.Join("..", "cli", "env.go"):  {"Getenv": true},
		filepath.Join("..", "cli", "root.go"): {"Getwd": true},
	}
	files, err := filepath.Glob(filepath.Join("..", "*", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		imports := importedPackages(file)
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || !isImportedSelector(selector, imports, "os") {
				return true
			}
			if !map[string]bool{"Getenv": true, "UserHomeDir": true, "Getwd": true}[selector.Sel.Name] {
				return true
			}
			if !allowed[filepath.Clean(path)][selector.Sel.Name] {
				t.Errorf("%s reads ambient workstation state outside the approved process edge through os.%s", path, selector.Sel.Name)
			}
			return true
		})
	}
}

func importedPackages(file *ast.File) map[string]string {
	imports := map[string]string{}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := filepath.Base(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		imports[name] = path
	}
	return imports
}

func isImportedCall(expr ast.Expr, imports map[string]string, importPath, functionName string) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != functionName {
		return false
	}
	return isImportedSelector(selector, imports, importPath)
}

func isImportedSelector(selector *ast.SelectorExpr, imports map[string]string, importPath string) bool {
	identifier, ok := selector.X.(*ast.Ident)
	return ok && imports[identifier.Name] == importPath
}

func cliGoSources(t *testing.T) []cliSource {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	sources := make([]cliSource, 0, len(files))
	for _, file := range files {
		if file == "architecture_test.go" {
			continue
		}
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, cliSource{name: file, text: string(source)})
	}
	return sources
}

func TestClassicLifecycleDeletionDoesNotRedistributePolicyInCLI(t *testing.T) {
	for _, obsolete := range []string{"plan.go", "skills.go"} {
		if _, err := os.Stat(obsolete); !os.IsNotExist(err) {
			t.Fatalf("obsolete CLI lifecycle module %s still exists", obsolete)
		}
	}

	for _, source := range cliGoSources(t) {
		file := source.name
		for _, forbidden := range []string{
			"type Plan struct",
			"type PlannedAction struct",
			"type ActionKind string",
			"func DiscoverManagedSkills(",
			"func plannedSkillLinkAction(",
			"func inspectSkillLink(",
			"skillLinkBehaviors",
			"unmanagedSymlinkSkipSummary",
		} {
			if strings.Contains(source.text, forbidden) {
				t.Fatalf("%s retained or redistributed obsolete classic lifecycle structure %q", file, forbidden)
			}
		}
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		for _, forbidden := range []string{"os.Lstat(", "os.Readlink(", "os.Symlink(", "os.Remove(", "skillbundle.Discover(", "corelifecycle.LoadState(", "corelifecycle.SaveState("} {
			if strings.Contains(source.text, forbidden) {
				t.Fatalf("%s redistributed classic lifecycle policy through %q", file, forbidden)
			}
		}
	}

	root, err := os.ReadFile("root.go")
	if err != nil {
		t.Fatal(err)
	}
	for call, want := range map[string]int{
		"corelifecycle.NewFacade(": 3,
		"lifecycle.Preview(":       3,
		"lifecycle.Apply(":         3,
	} {
		if got := strings.Count(string(root), call); got != want {
			t.Fatalf("root.go has %d occurrences of %q, want one route for each of three classic operations", got, call)
		}
	}
	for _, operation := range []string{"corelifecycle.Install", "corelifecycle.Update", "corelifecycle.Uninstall"} {
		if got := strings.Count(string(root), operation); got != 1 {
			t.Fatalf("root.go has %d production routes for %s, want 1", got, operation)
		}
	}
}

func TestSetupHealthDeletionDoesNotRedistributeDiagnosisPolicyInCLI(t *testing.T) {
	for _, obsolete := range []string{"doctor.go", "doctor_test.go"} {
		if _, err := os.Stat(obsolete); !os.IsNotExist(err) {
			t.Fatalf("obsolete CLI setup-health file %s still exists", obsolete)
		}
	}

	for _, source := range cliGoSources(t) {
		file := source.name
		for _, forbidden := range []string{
			"BuildDoctorReport",
			"buildDoctorReport",
			"RunDoctor",
			"type DoctorReport",
			"type DoctorSummary",
			"DoctorReport =",
			"DoctorSummary =",
			"doctorCheck",
			"doctorStatus",
			"stateCheck(",
			"skillChecks(",
			"engramChecks(",
			"codexChecks(",
			"openCodeChecks(",
		} {
			if strings.Contains(source.text, forbidden) {
				t.Fatalf("%s retained or redistributed obsolete setup-health structure %q", file, forbidden)
			}
		}
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		for _, forbidden := range []string{
			"corelifecycle.ObserveState(",
			"corelifecycle.ObserveManagedSkillLinks(",
			"engrambin.Diagnose",
			"opencode.Inspect(",
			"prompt.DetectExternalManagedBlocks(",
			"\"packy-state\"",
			"\"engram-binary\"",
			"\"codex-config\"",
			"\"opencode-config\"",
		} {
			if strings.Contains(source.text, forbidden) {
				t.Fatalf("%s redistributed setup-health diagnosis policy through %q", file, forbidden)
			}
		}
	}

	root, err := os.ReadFile("root.go")
	if err != nil {
		t.Fatal(err)
	}
	for call, want := range map[string]int{
		"setuphealth.Diagnose(":      1,
		"opts.SetupHealthDiagnose()": 1,
	} {
		if got := strings.Count(string(root), call); got != want {
			t.Fatalf("root.go has %d occurrences of %q, want %d", got, call, want)
		}
	}
}
