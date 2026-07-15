package main

import (
	"bytes"
	"testing"

	"github.com/yersonargotev/matty/internal/packsync"
)

func TestRendererUsesTheEngineCanonicalHumanAndJSONPlans(t *testing.T) {
	plan := packsync.Plan{SchemaVersion: 1, PlanID: "pack-sync-test", Status: "blocked", SourceID: "source", Blockers: []string{"blocked"}, Changes: []packsync.Change{}, Discoveries: []string{}}
	for _, test := range []struct {
		format string
		want   func() []byte
	}{
		{format: "human", want: func() []byte { return []byte(plan.Human()) }},
		{format: "json", want: func() []byte { data, _ := plan.CanonicalJSON(); return data }},
	} {
		var output bytes.Buffer
		if err := render(&output, plan, test.format); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(output.Bytes(), test.want()) {
			t.Fatalf("%s renderer diverged from engine canonical plan", test.format)
		}
	}
}
