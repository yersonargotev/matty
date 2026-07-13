package capabilitypack

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type productionSource struct {
	path string
	text string
}

func internalProductionSources(t *testing.T) []productionSource {
	t.Helper()
	root := ".."
	var sources []productionSource
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sources = append(sources, productionSource{path: filepath.ToSlash(path), text: string(data)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return sources
}

func TestSurfaceAdapterArchitectureCannotRegress(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "opencode"+"activation")); !os.IsNotExist(err) {
		t.Fatal("obsolete OpenCode activation package exists")
	}

	sources := internalProductionSources(t)
	obsolete := []string{
		"Activation" + "Adapter",
		"Activation" + "Observation",
		"ResolutionAware" + "ActivationAdapter",
		"DeactivationAware" + "ActivationAdapter",
		"ReconciliationAware" + "ActivationAdapter",
		"Readiness" + "Inspector",
		"Surface" + "Inspector",
		"Inspect" + "Activation",
		"Inspect" + "Deactivation",
		"Inspect" + "Reconcile",
		"WithReadiness" + "Inspectors",
	}
	concreteAdapters := 0
	inspectionImplementations := 0
	applicationImplementations := 0
	directInspections := 0
	fingerprintRemovalSlots := 0
	for _, source := range sources {
		for _, forbidden := range obsolete {
			if strings.Contains(source.text, forbidden) {
				t.Fatalf("%s reintroduced obsolete surface structure %q", source.path, forbidden)
			}
		}
		if strings.Contains(source.path, "/codex/") || strings.Contains(source.path, "/opencode/") {
			concreteAdapters += strings.Count(source.text, "type SurfaceAdapter struct")
			inspectionImplementations += strings.Count(source.text, ") InspectSurface(")
			applicationImplementations += strings.Count(source.text, ") ApplyProjections(")
			for _, lifecycle := range []string{"capabilitypack.OperationActivate", "capabilitypack.OperationUpdate", "capabilitypack.OperationDeactivate", "capabilitypack.OperationReconcile", "capabilitypack.ActivationRequest", "capabilitypack.UpdateRequest", "capabilitypack.DeactivationRequest", "capabilitypack.ReconcileRequest"} {
				if strings.Contains(source.text, lifecycle) {
					t.Fatalf("%s redistributed lifecycle policy through %q", source.path, lifecycle)
				}
			}
		}
		if strings.Contains(source.path, "/capabilitypack/") {
			for _, hostPolicy := range []string{"internal/codex", "internal/opencode", "MergeInstructionProjection(", "MergeMCPProjection(", "ValidateInstructionProjection(", "ValidateMCPProjection("} {
				if strings.Contains(source.text, hostPolicy) {
					t.Fatalf("%s redistributed host policy through %q", source.path, hostPolicy)
				}
			}
		}
		if strings.Contains(source.path, "/cli/") {
			for _, policy := range []string{".InspectSurface(", ".ApplyProjections(", "RemovalCandidate(", "surfaceTransitionFacts(", "ProjectionPresent", "ProjectionAbsent"} {
				if strings.Contains(source.text, policy) {
					t.Fatalf("%s redistributed surface policy through %q", source.path, policy)
				}
			}
		}
		count := strings.Count(source.text, ".InspectSurface(")
		if count > 0 && source.path != "../capabilitypack/activation.go" {
			t.Fatalf("%s introduced a parallel production inspection route", source.path)
		}
		directInspections += count
		removalSlots := strings.Count(source.text, "Removal"+"Candidates")
		if removalSlots > 0 && source.path != "../capabilitypack/activation.go" {
			t.Fatalf("%s reintroduced a removal-candidate side channel", source.path)
		}
		fingerprintRemovalSlots += removalSlots
	}
	if concreteAdapters != 2 {
		t.Fatalf("found %d concrete production surface adapters, want Codex and OpenCode only", concreteAdapters)
	}
	if inspectionImplementations != 2 || applicationImplementations != 2 {
		t.Fatalf("found %d inspection and %d application implementations, want one complete implementation per host", inspectionImplementations, applicationImplementations)
	}
	if directInspections != 1 {
		t.Fatalf("found %d direct production InspectSurface calls, want the private gateway only", directInspections)
	}
	if fingerprintRemovalSlots != 1 {
		t.Fatalf("found %d removal-candidate names, want only the fixed legacy fingerprint JSON key", fingerprintRemovalSlots)
	}
}
