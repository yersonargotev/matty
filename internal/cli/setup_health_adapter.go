package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/yersonargotev/packy/internal/setuphealth"
)

// ErrDoctorUnhealthy identifies a completed diagnostic report containing one
// or more failed health checks. Warnings alone do not make a report unhealthy.
var ErrDoctorUnhealthy = errors.New("doctor found failed health checks")

type doctorHealthError struct{ failedChecks int }

func (err doctorHealthError) Error() string {
	return fmt.Sprintf("%s: %d", ErrDoctorUnhealthy, err.failedChecks)
}

func (err doctorHealthError) Unwrap() error { return ErrDoctorUnhealthy }

func renderSetupHealthHuman(w io.Writer, report setuphealth.Report) error {
	context := report.Context
	if _, err := fmt.Fprintf(w, "HOME=%s\nCONFIG_HOME=%s\nPACKY_STATE=%s\nPACKY_STATE_STATUS=%s\nAGENT_SKILLS=%s\n", context.HomeDir, context.ConfigHome, context.StateFile, context.StateStatus, context.AgentSkillsDir); err != nil {
		return err
	}
	for _, check := range report.Checks {
		if _, err := fmt.Fprintf(w, "%s %s: %s\n", check.Severity, check.Name, check.Detail); err != nil {
			return err
		}
	}
	return nil
}

type setupHealthJSONCheck struct {
	Name     string               `json:"name"`
	Severity setuphealth.Severity `json:"severity"`
	Detail   string               `json:"detail"`
}

type setupHealthJSONReport struct {
	SchemaVersion int                    `json:"schema_version"`
	Report        string                 `json:"report"`
	Checks        []setupHealthJSONCheck `json:"checks"`
	Summary       setupHealthJSONSummary `json:"summary"`
}

type setupHealthJSONSummary struct {
	Status   string `json:"status"`
	Passes   int    `json:"passes"`
	Warnings int    `json:"warnings"`
	Failures int    `json:"failures"`
}

func renderSetupHealthJSON(w io.Writer, report setuphealth.Report) error {
	checks := make([]setupHealthJSONCheck, 0, len(report.Checks))
	for _, check := range report.Checks {
		checks = append(checks, setupHealthJSONCheck{Name: check.Name, Severity: check.Severity, Detail: check.Detail})
	}
	summary := report.Summary
	return json.NewEncoder(w).Encode(setupHealthJSONReport{
		SchemaVersion: report.SchemaVersion,
		Report:        report.Kind,
		Checks:        checks,
		Summary: setupHealthJSONSummary{
			Status: summary.Status, Passes: summary.Passes, Warnings: summary.Warnings, Failures: summary.Failures,
		},
	})
}

func setupHealthError(report setuphealth.Report) error {
	if report.Summary.Failures > 0 {
		return doctorHealthError{failedChecks: report.Summary.Failures}
	}
	return nil
}
