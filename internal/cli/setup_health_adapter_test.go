package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/yersonargotev/matty/internal/setuphealth"
)

func TestRenderSetupHealthHuman(t *testing.T) {
	report := setuphealth.Report{
		Context: setuphealth.Context{
			HomeDir:        "/home/test",
			ConfigHome:     "/home/test/config",
			StateFile:      "/home/test/.matty/config.json",
			StateStatus:    "present",
			AgentSkillsDir: "/home/test/.agents/skills",
		},
		Checks: []setuphealth.Check{{Name: "fixture", Severity: setuphealth.Warn, Detail: "inspect fixture"}},
	}
	var output bytes.Buffer

	if err := renderSetupHealthHuman(&output, report); err != nil {
		t.Fatal(err)
	}
	want := "HOME=/home/test\nCONFIG_HOME=/home/test/config\nMATTY_STATE=/home/test/.matty/config.json\nMATTY_STATE_STATUS=present\nAGENT_SKILLS=/home/test/.agents/skills\nWARN fixture: inspect fixture\n"
	if output.String() != want {
		t.Fatalf("human output = %q, want %q", output.String(), want)
	}
}

func TestRenderSetupHealthJSONV1OmitsContext(t *testing.T) {
	report := setuphealth.Report{
		SchemaVersion: 1,
		Kind:          "doctor",
		Context:       setuphealth.Context{HomeDir: "/must-not-appear"},
		Checks:        []setuphealth.Check{{Name: "fixture", Severity: setuphealth.Pass, Detail: "healthy"}},
		Summary:       setuphealth.Summary{Status: "healthy", Passes: 1},
	}
	var output bytes.Buffer

	if err := renderSetupHealthJSON(&output, report); err != nil {
		t.Fatal(err)
	}
	want := "{\"schema_version\":1,\"report\":\"doctor\",\"checks\":[{\"name\":\"fixture\",\"severity\":\"PASS\",\"detail\":\"healthy\"}],\"summary\":{\"status\":\"healthy\",\"passes\":1,\"warnings\":0,\"failures\":0}}\n"
	if output.String() != want {
		t.Fatalf("JSON output = %q, want %q", output.String(), want)
	}
	if strings.Contains(output.String(), "must-not-appear") {
		t.Fatalf("JSON v1 included report context: %s", output.String())
	}
}

func TestSetupHealthRenderersReturnWriterErrors(t *testing.T) {
	wantErr := errors.New("write failed")
	writer := failingWriter{err: wantErr}
	report := setuphealth.Report{Checks: []setuphealth.Check{{Name: "fixture", Severity: setuphealth.Pass}}}

	for name, render := range map[string]func() error{
		"human": func() error { return renderSetupHealthHuman(writer, report) },
		"json":  func() error { return renderSetupHealthJSON(writer, report) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := render(); !errors.Is(err, wantErr) {
				t.Fatalf("error = %v, want %v", err, wantErr)
			}
		})
	}
}

func TestSetupHealthErrorMapsOnlyFailures(t *testing.T) {
	if err := setupHealthError(setuphealth.Report{Summary: setuphealth.Summary{Warnings: 1}}); err != nil {
		t.Fatalf("warnings returned fatal error: %v", err)
	}
	err := setupHealthError(setuphealth.Report{Summary: setuphealth.Summary{Failures: 2}})
	if !errors.Is(err, ErrDoctorUnhealthy) || err.Error() != "doctor found failed health checks: 2" {
		t.Fatalf("failure error = %v", err)
	}
}

type failingWriter struct{ err error }

func (writer failingWriter) Write([]byte) (int, error) { return 0, writer.err }
