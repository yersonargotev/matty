package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/corelifecycle"
)

const classicLifecycleJSONSchemaVersion = 2

type classicStateTransitionJSON struct {
	FromSchemaVersion int                         `json:"from_schema_version"`
	ToSchemaVersion   int                         `json:"to_schema_version"`
	FromStatus        corelifecycle.InstallStatus `json:"from_status"`
	ToStatus          corelifecycle.InstallStatus `json:"to_status"`
}

type classicActionJSON struct {
	Kind        corelifecycle.ActionKind `json:"kind"`
	Description string                   `json:"description"`
	Path        string                   `json:"path,omitempty"`
	Target      string                   `json:"target,omitempty"`
	Command     string                   `json:"command,omitempty"`
	Args        []string                 `json:"args,omitempty"`
}

type classicLifecyclePlanJSON struct {
	SchemaVersion        int                        `json:"schema_version"`
	Report               string                     `json:"report"`
	Operation            corelifecycle.Operation    `json:"operation"`
	Outcome              corelifecycle.Outcome      `json:"outcome"`
	DesiredSurfaces      []string                   `json:"desired_surfaces"`
	PendingPrerequisites []string                   `json:"pending_prerequisites"`
	Preserved            []string                   `json:"preserved"`
	Blockers             []string                   `json:"blockers"`
	Recovery             []string                   `json:"recovery"`
	StateTransition      classicStateTransitionJSON `json:"state_transition"`
	Actions              []classicActionJSON        `json:"actions"`
	DryRun               bool                       `json:"dry_run"`
}

type classicLifecycleResultJSON struct {
	SchemaVersion        int                        `json:"schema_version"`
	Report               string                     `json:"report"`
	Operation            corelifecycle.Operation    `json:"operation"`
	Outcome              corelifecycle.Outcome      `json:"outcome"`
	DesiredSurfaces      []string                   `json:"desired_surfaces"`
	PendingPrerequisites []string                   `json:"pending_prerequisites"`
	Preserved            []string                   `json:"preserved"`
	Blockers             []string                   `json:"blockers"`
	Recovery             []string                   `json:"recovery"`
	StateTransition      classicStateTransitionJSON `json:"state_transition"`
	Committed            bool                       `json:"committed"`
	CompletedEffects     []string                   `json:"completed_effects"`
	FailedEffect         string                     `json:"failed_effect,omitempty"`
	NotStartedEffects    []string                   `json:"not_started_effects"`
	Warnings             []string                   `json:"warnings"`
	ManagedSkillCount    int                        `json:"managed_skill_count"`
}

func renderClassicLifecyclePlanJSON(w io.Writer, operation corelifecycle.Operation, plan corelifecycle.Plan) error {
	return json.NewEncoder(w).Encode(classicPlanJSON(operation, plan))
}

func renderClassicLifecycleResultJSON(w io.Writer, operation corelifecycle.Operation, plan corelifecycle.Plan, result corelifecycle.Result, committed bool) error {
	return json.NewEncoder(w).Encode(classicLifecycleResultJSON{
		SchemaVersion: classicLifecycleJSONSchemaVersion, Report: "classic-lifecycle-result", Operation: operation,
		Outcome: result.Outcome(), DesiredSurfaces: sortedStrings(plan.DesiredSurfaces()),
		PendingPrerequisites: sortedStrings(plan.PendingPrerequisites()), Preserved: sortedStrings(plan.Preserved()),
		Blockers: sortedStrings(plan.Blockers()), Recovery: sortedStrings(plan.RecoveryEvidence()),
		StateTransition: stateTransitionJSON(plan.StateTransition()), Committed: committed,
		CompletedEffects: sequenceStrings(result.CompletedEffects()), FailedEffect: result.FailedEffect(),
		NotStartedEffects: sequenceStrings(result.NotStartedEffects()), Warnings: sortedStrings(result.Warnings()),
		ManagedSkillCount: result.ManagedSkillCount(),
	})
}

func renderClassicLifecycleResultHuman(w io.Writer, plan corelifecycle.Plan, result corelifecycle.Result, committed bool) error {
	transition := plan.StateTransition()
	_, err := fmt.Fprintf(w, "Outcome: %s\nDesired surfaces: %s\nState transition: schema %d -> %d; status %s -> %s; committed=%t\nPending prerequisites: %s\nPreserved: %s\nLifecycle blockers: %s\nRecovery: %s\nCompleted effects: %s\nFailed effect: %s\nNot started effects: %s\n",
		result.Outcome(), strings.Join(sortedStrings(plan.DesiredSurfaces()), ", "),
		transition.FromSchemaVersion, transition.ToSchemaVersion, transition.FromStatus, transition.ToStatus, committed,
		joinReportValues(sortedStrings(plan.PendingPrerequisites())), joinReportValues(sortedStrings(plan.Preserved())),
		joinReportValues(sortedStrings(plan.Blockers())), joinReportValues(sortedStrings(plan.RecoveryEvidence())),
		joinReportValues(sequenceStrings(result.CompletedEffects())), joinReportValue(result.FailedEffect()),
		joinReportValues(sequenceStrings(result.NotStartedEffects())))
	return err
}

func joinReportValues(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, "; ")
}

func joinReportValue(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func classicPlanJSON(operation corelifecycle.Operation, plan corelifecycle.Plan) classicLifecyclePlanJSON {
	actions := plan.Actions()
	outputActions := make([]classicActionJSON, 0, len(actions))
	for _, action := range actions {
		outputActions = append(outputActions, classicActionJSON{
			Kind: action.Kind, Description: action.Description, Path: action.Path, Target: action.Target,
			Command: action.Command, Args: redactCLIEnvironmentArgs(action.Args),
		})
	}
	return classicLifecyclePlanJSON{
		SchemaVersion: classicLifecycleJSONSchemaVersion, Report: "classic-lifecycle-preview", Operation: operation,
		Outcome: plan.Outcome(), DesiredSurfaces: sortedStrings(plan.DesiredSurfaces()),
		PendingPrerequisites: sortedStrings(plan.PendingPrerequisites()), Preserved: sortedStrings(plan.Preserved()),
		Blockers: sortedStrings(plan.Blockers()), Recovery: sortedStrings(plan.RecoveryEvidence()),
		StateTransition: stateTransitionJSON(plan.StateTransition()), Actions: outputActions, DryRun: true,
	}
}

func stateTransitionJSON(value corelifecycle.StateTransitionView) classicStateTransitionJSON {
	return classicStateTransitionJSON{
		FromSchemaVersion: value.FromSchemaVersion, ToSchemaVersion: value.ToSchemaVersion,
		FromStatus: value.FromStatus, ToStatus: value.ToStatus,
	}
}

func sortedStrings(values []string) []string {
	result := append([]string{}, values...)
	sort.Strings(result)
	return result
}

func sequenceStrings(values []string) []string { return append([]string{}, values...) }

func redactCLIEnvironmentArgs(values []string) []string {
	result := append([]string(nil), values...)
	for i := range result {
		if result[i] == "--env" || result[i] == "-e" {
			if i+1 < len(result) {
				result[i+1] = redactCLIEnvironmentValue(result[i+1])
				i++
			}
			continue
		}
		for _, prefix := range []string{"--env=", "-e="} {
			if strings.HasPrefix(result[i], prefix) {
				result[i] = prefix + redactCLIEnvironmentValue(strings.TrimPrefix(result[i], prefix))
			}
		}
	}
	return result
}

func redactCLIEnvironmentValue(value string) string {
	if key, _, ok := strings.Cut(value, "="); ok {
		return key + "=<redacted>"
	}
	return "<redacted>"
}
