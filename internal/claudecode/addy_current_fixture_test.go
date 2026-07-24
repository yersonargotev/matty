package claudecode

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/packy/internal/addyacceptance"
	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestCanonicalAddyCurrentFixtureProducesCompleteClaudeSurface(t *testing.T) {
	current := addyacceptance.CanonicalPromotionCurrent()
	bundle := filepath.Join(t.TempDir(), "bundle")
	if err := addyacceptance.WriteCanonicalPromotionCurrent(bundle); err != nil {
		t.Fatal(err)
	}

	var manifest struct {
		SchemaVersion int                         `json:"schema_version"`
		ID            string                      `json:"id"`
		Version       string                      `json:"version"`
		Surfaces      []capabilitypack.Surface    `json:"surfaces"`
		Provides      []string                    `json:"provides"`
		Requires      capabilitypack.Requirements `json:"requires"`
		Conflicts     []string                    `json:"conflicts"`
		Resources     []capabilitypack.Resource   `json:"resources"`
		Contract      capabilitypack.Contract     `json:"contract"`
	}
	if err := json.Unmarshal(current.Manifest, &manifest); err != nil {
		t.Fatal(err)
	}
	pack := capabilitypack.Pack{
		ID: manifest.ID, Version: manifest.Version, Surfaces: manifest.Surfaces,
		Provides: manifest.Provides, Requires: manifest.Requires,
		Conflicts: manifest.Conflicts, Resources: manifest.Resources, Contract: manifest.Contract,
	}

	home := t.TempDir()
	adapter := NewSurfaceAdapter(
		bundle,
		NewCanonicalLayout(home),
		filepath.Join(home, "state"),
		"claude",
		&recordingRunner{result: Result{Stdout: "2.1.203"}},
		StaticOwnershipSnapshot(NewOwnershipSnapshot()),
	)
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Projections) != 36 {
		t.Fatalf("Claude projections = %d, want 36", len(inspection.Projections))
	}

	expected := map[string]string{}
	counts := map[string]int{}
	for _, resource := range pack.Resources {
		for _, binding := range resource.Bindings {
			if binding.Surface != capabilitypack.SurfaceClaude {
				continue
			}
			switch resource.Kind {
			case "skill":
				expected[binding.Projection+":"+binding.Name] = "skill"
				counts["skill"]++
			case "agent":
				if binding.AgentAuthority == nil {
					t.Fatalf("Addy agent %s has no strict Claude authority", resource.ID)
				}
				expected[binding.Projection+":"+binding.Name] = "agent"
				counts["agent"]++
			case "command":
				if binding.Projection != "skill" {
					t.Fatalf("Addy command %s is not a Claude command-as-skill", resource.ID)
				}
				expected["command:"+binding.Name] = "command-as-skill"
				counts["command-as-skill"]++
			}
		}
	}
	if counts["skill"] != 24 || counts["agent"] != 4 || counts["command-as-skill"] != 8 {
		t.Fatalf("current fixture projection classes = %v", counts)
	}
	for _, projection := range inspection.Projections {
		if expected[projection.ID] == "" {
			t.Fatalf("unexpected Claude projection %q", projection.ID)
		}
		delete(expected, projection.ID)
	}
	if len(expected) != 0 {
		t.Fatalf("missing Claude projections: %v", expected)
	}
}
