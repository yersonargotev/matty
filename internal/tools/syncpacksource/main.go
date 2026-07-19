// Command syncpacksource is the private repository adapter for the manual
// synchronization workflow. It is intentionally outside cmd/ and release
// artifacts; deterministic authority remains in packsync, packclassification,
// and packsyncworkflow.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/packy/internal/packclassification"
	"github.com/yersonargotev/packy/internal/packsync"
	"github.com/yersonargotev/packy/internal/packsync/githubsource"
	"github.com/yersonargotev/packy/internal/packsyncworkflow"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type options struct {
	phase          string
	repositoryRoot string
	requestPath    string
	planPath       string
	evidencePath   string
	validationPath string
	outputDir      string
	sourceID       string
	format         string
	selectorMode   string
	selectorRef    string
}

var workflowSourceFactory = func() packsync.Source {
	client := newAuthenticatedGitHubHTTPClient(os.Getenv("GITHUB_TOKEN"), nil)
	return newRetryingSource(githubsource.New(client))
}

func run(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("syncpacksource", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var option options
	flags.StringVar(&option.phase, "phase", "inspect", "inspect, classify, validate, or publish")
	flags.StringVar(&option.repositoryRoot, "repository-root", ".", "sandbox Packy repository root")
	flags.StringVar(&option.requestPath, "request", "", "canonical dispatch request JSON")
	flags.StringVar(&option.planPath, "plan", "", "canonical sealed plan JSON")
	flags.StringVar(&option.evidencePath, "evidence", "", "canonical classification evidence JSON")
	flags.StringVar(&option.validationPath, "validation", "", "canonical sandbox validation proof JSON")
	flags.StringVar(&option.outputDir, "output", "", "artifact output directory")
	flags.StringVar(&option.sourceID, "source", "", "configured source id")
	flags.StringVar(&option.format, "format", "human", "human or json (legacy inspect output)")
	flags.StringVar(&option.selectorMode, "selector", "", "optional packsync selector override")
	flags.StringVar(&option.selectorRef, "ref", "", "exact prerelease tag or full commit SHA")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	var err error
	switch option.phase {
	case "inspect":
		err = inspect(ctx, option, output)
	case "classify":
		err = classify(ctx, option, output)
	case "validate":
		err = validateSandbox(ctx, option, output)
	case "publish":
		err = publish(ctx, option, output)
	default:
		err = fmt.Errorf("unsupported phase %q", option.phase)
	}
	if err != nil && option.outputDir != "" {
		_ = writeFailureArtifact(option, err)
	}
	return err
}

func writeFailureArtifact(option options, failure error) error {
	if err := os.MkdirAll(option.outputDir, 0o755); err != nil {
		return err
	}
	sourceID := option.sourceID
	var request packsyncworkflow.DispatchRequest
	if option.requestPath != "" && readJSON(option.requestPath, &request) == nil {
		sourceID = request.SourceID
	}
	if sourceID == "" {
		sourceID = os.Getenv("PACKY_SOURCE_ID")
	}
	if !packsyncworkflow.ValidSourceID(sourceID) {
		sourceID = "unknown"
	}
	context := packsyncworkflow.FailureArtifactContext{SourceID: sourceID, RunURL: actionsRunURL()}
	planPath := option.planPath
	if planPath == "" {
		planPath = filepath.Join(option.outputDir, "plan.json")
	}
	var plan packsync.Plan
	if readJSON(planPath, &plan) == nil {
		context.PlanID = plan.PlanID
		context.BaseSHA = plan.Preconditions.BaseCommit
		context.CandidateSHA = plan.Candidate.Commit
		context.ArtifactProvenance = artifactProvenance(plan)
		if plan.Status == "blocked" {
			context.Blockers = append([]string(nil), plan.Blockers...)
		}
	}
	artifact := packsyncworkflow.NewFailureArtifact(context, failure)
	data, err := artifact.CanonicalJSON()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(option.outputDir, "operational-artifact.json"), data, 0o600)
}

func inspect(ctx context.Context, option options, output io.Writer) error {
	request, checkRequest, err := inspectRequest(option)
	if err != nil {
		return err
	}
	acquisition, err := os.MkdirTemp("", "packy-pack-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(acquisition)
	checkRequest.AcquisitionDir = acquisition
	plan, err := (packsync.Engine{Source: workflowSourceFactory()}).Check(ctx, checkRequest)
	if err != nil {
		return err
	}
	if option.outputDir == "" {
		return render(output, plan, option.format)
	}
	if err := os.MkdirAll(option.outputDir, 0o755); err != nil {
		return err
	}
	if err := writeCanonical(filepath.Join(option.outputDir, "request.json"), request); err != nil {
		return err
	}
	planJSON, err := plan.CanonicalJSON()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(option.outputDir, "plan.json"), planJSON, 0o600); err != nil {
		return err
	}
	if plan.Status == "no-op" {
		if err := writeNoopArtifact(option.outputDir, request.SourceID, plan); err != nil {
			return err
		}
	}
	if request.ClassificationMode == packsyncworkflow.ClassificationHuman && len(request.HumanEvidence) == 0 && plan.Status == "review-required" {
		inspection, err := packclassification.InspectHuman(plan)
		if err != nil {
			return err
		}
		if err := writeCanonical(filepath.Join(option.outputDir, "inspection.json"), inspection); err != nil {
			return err
		}
	}
	if plan.Status == "blocked" {
		return packsyncworkflow.Failure{Kind: packsyncworkflow.FailureProvenance, Err: fmt.Errorf("Check blocked: %s", strings.Join(plan.Blockers, "; "))}
	}
	_, err = fmt.Fprintf(output, "%s\n", filepath.Join(option.outputDir, "plan.json"))
	return err
}

func writeNoopArtifact(outputDir, sourceID string, plan packsync.Plan) error {
	artifact := packsyncworkflow.NoopArtifact{SchemaVersion: 2, State: "no-op", SourceID: sourceID, PlanID: plan.PlanID, BaseSHA: plan.Preconditions.BaseCommit, CandidateSHA: plan.Candidate.Commit, ArtifactProvenance: artifactProvenance(plan)}
	if err := artifact.Validate(); err != nil {
		return err
	}
	return writeCanonical(filepath.Join(outputDir, "no-op.json"), artifact)
}

func artifactProvenance(plan packsync.Plan) packsyncworkflow.ArtifactProvenance {
	return packsyncworkflow.ArtifactProvenance{SourceLockSHA256: plan.SourceLockSHA256, LockSetSHA256: plan.LockSetSHA256, ConfigSHA256: plan.Preconditions.ConfigSHA256, ManifestsSHA256: plan.Preconditions.ManifestsSHA256}
}

func inspectRequest(option options) (packsyncworkflow.DispatchRequest, packsync.CheckRequest, error) {
	if option.requestPath == "" {
		if os.Getenv("PACKY_SOURCE_ID") != "" {
			request := packsyncworkflow.DispatchRequest{SchemaVersion: 2, Operation: packsyncworkflow.OperationSynchronize, SourceID: os.Getenv("PACKY_SOURCE_ID"), Selector: packsyncworkflow.Selector(os.Getenv("PACKY_SELECTOR")), SelectorRef: os.Getenv("PACKY_SELECTOR_REF"), ClassificationMode: packsyncworkflow.ClassificationMode(os.Getenv("PACKY_CLASSIFICATION_MODE")), RequestReason: os.Getenv("PACKY_REQUEST_REASON"), RetryOfRun: os.Getenv("PACKY_RETRY_OF_RUN"), ExpectedPlanID: os.Getenv("PACKY_EXPECTED_PLAN_ID"), ExpectedBaseSHA: os.Getenv("PACKY_EXPECTED_BASE_SHA")}
			if raw := os.Getenv("PACKY_HUMAN_EVIDENCE_JSON"); raw != "" {
				request.HumanEvidence = json.RawMessage(raw)
			}
			if err := request.Validate(); err != nil {
				return request, packsync.CheckRequest{}, err
			}
			if expected := os.Getenv("PACKY_REQUEST_DIGEST"); expected != "" {
				actual, err := request.Digest()
				if err != nil || actual != expected {
					return request, packsync.CheckRequest{}, errors.New("workflow request digest does not match the canonical dispatch")
				}
			}
			check, err := checkRequestForDispatch(option.repositoryRoot, request)
			return request, check, err
		}
		request := packsyncworkflow.DispatchRequest{SchemaVersion: 1, SourceID: option.sourceID, Selector: packsyncworkflow.SelectorLatestStable, ClassificationMode: packsyncworkflow.ClassificationAI, RequestReason: "private Check renderer"}
		check := packsync.CheckRequest{RepositoryRoot: option.repositoryRoot, SourceID: option.sourceID}
		if option.selectorMode != "" {
			check.Selector = &packsync.Selector{Mode: packsync.SelectorMode(option.selectorMode), Ref: option.selectorRef}
		}
		return request, check, nil
	}
	var request packsyncworkflow.DispatchRequest
	if err := readJSON(option.requestPath, &request); err != nil {
		return request, packsync.CheckRequest{}, err
	}
	if err := request.Validate(); err != nil {
		return request, packsync.CheckRequest{}, err
	}
	check, err := checkRequestForDispatch(option.repositoryRoot, request)
	return request, check, err
}

func checkRequestForDispatch(repositoryRoot string, request packsyncworkflow.DispatchRequest) (packsync.CheckRequest, error) {
	selector := packsync.Selector{}
	if request.ClassificationMode == packsyncworkflow.ClassificationHuman && len(request.HumanEvidence) != 0 {
		var evidence packsync.ClassificationEvidenceSet
		if err := json.Unmarshal(request.HumanEvidence, &evidence); err != nil || evidence.Candidate.Commit != request.SelectorRef {
			return packsync.CheckRequest{}, errors.New("human evidence candidate does not match the exact second-dispatch commit")
		}
		if evidence.Candidate.Release != nil {
			if evidence.Candidate.Release.Prerelease {
				selector.Mode, selector.Ref = packsync.SelectorPrerelease, evidence.Candidate.Release.Tag
			} else {
				selector.Mode = packsync.SelectorStableRelease
			}
		} else {
			selector.Mode, selector.Ref = packsync.SelectorCommit, evidence.Candidate.Commit
		}
		return packsync.CheckRequest{RepositoryRoot: repositoryRoot, SourceID: request.SourceID, Selector: &selector}, nil
	}
	switch request.Selector {
	case packsyncworkflow.SelectorLatestStable:
		selector.Mode = packsync.SelectorStableRelease
	case packsyncworkflow.SelectorPrerelease:
		selector.Mode, selector.Ref = packsync.SelectorPrerelease, request.SelectorRef
	case packsyncworkflow.SelectorCommit:
		selector.Mode, selector.Ref = packsync.SelectorCommit, request.SelectorRef
	}
	return packsync.CheckRequest{RepositoryRoot: repositoryRoot, SourceID: request.SourceID, Selector: &selector}, nil
}

func classify(ctx context.Context, option options, output io.Writer) error {
	if option.requestPath == "" || option.planPath == "" || option.outputDir == "" {
		return errors.New("classify requires request, plan, and output paths")
	}
	var request packsyncworkflow.DispatchRequest
	var plan packsync.Plan
	if err := readJSON(option.requestPath, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if err := readJSON(option.planPath, &plan); err != nil {
		return err
	}
	if request.ExpectedPlanID != "" && (request.ExpectedPlanID != plan.PlanID || request.ExpectedBaseSHA != plan.Preconditions.BaseCommit || request.SelectorRef != plan.Candidate.Commit) {
		return classificationFailure(errors.New("human evidence request is stale against the fresh Check"))
	}
	var evidence packsync.ClassificationEvidenceSet
	var err error
	if len(plan.AffectedPacks) == 0 {
		if plan.Status != "review-required" {
			return fmt.Errorf("plan status %q does not require classification", plan.Status)
		}
		evidence = packsync.ClassificationEvidenceSet{}
		if err := os.MkdirAll(option.outputDir, 0o755); err != nil {
			return err
		}
		name := filepath.Join(option.outputDir, "classification.json")
		if err := writeCanonical(name, evidence); err != nil {
			return err
		}
		_, err = fmt.Fprintln(output, name)
		return err
	}
	switch request.ClassificationMode {
	case packsyncworkflow.ClassificationAI:
		model, modelErr := newGitHubModel()
		if modelErr != nil {
			return modelErr
		}
		service, _ := packclassification.NewAIService(model, 1)
		evidence, err = service.Classify(ctx, plan)
		if err == nil {
			if makeErr := os.MkdirAll(option.outputDir, 0o755); makeErr != nil {
				return makeErr
			}
			if traceErr := writeCanonical(filepath.Join(option.outputDir, "classifier-trace.json"), model.traces); traceErr != nil {
				return traceErr
			}
		}
	case packsyncworkflow.ClassificationHuman:
		if len(request.HumanEvidence) == 0 {
			return errors.New("human inspection is intentionally terminal until an evidence dispatch")
		}
		inspection, inspectionErr := packclassification.InspectHuman(plan)
		if inspectionErr != nil {
			return inspectionErr
		}
		if err := json.Unmarshal(request.HumanEvidence, &evidence); err != nil {
			return fmt.Errorf("decode human evidence: %w", err)
		}
		evidence, err = packclassification.SupplyHumanEvidence(plan, inspection, evidence)
	}
	if err != nil {
		return classificationFailure(err)
	}
	if err := os.MkdirAll(option.outputDir, 0o755); err != nil {
		return err
	}
	name := filepath.Join(option.outputDir, "classification.json")
	if err := writeCanonical(name, evidence); err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, name)
	return err
}

func classificationFailure(err error) error {
	return packsyncworkflow.Failure{Kind: packsyncworkflow.FailureClassification, Blocker: "Classification evidence is unavailable, invalid, or stale for the exact sealed plan.", Recovery: "Retry only an explicitly transient model failure; otherwise provide new evidence for a fresh exact inspection without changing classifier mode.", Err: err}
}

func render(output io.Writer, plan packsync.Plan, format string) error {
	switch format {
	case "human":
		_, err := io.WriteString(output, plan.Human())
		return err
	case "json":
		data, err := plan.CanonicalJSON()
		if err != nil {
			return err
		}
		_, err = output.Write(data)
		return err
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func readJSON(name string, destination any) error {
	data, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	return nil
}

func writeCanonical(name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(name, append(data, '\n'), 0o600)
}
