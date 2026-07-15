package ci_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/yersonargotev/matty/internal/packsyncworkflow"
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
	"./internal/packsyncworkflow",
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

func TestSyncWorkflowIsManualPinnedLeastPrivilegeAndPhaseSeparated(t *testing.T) {
	workflow := readFile(t, filepath.Join(repositoryRoot(t), ".github", "workflows", "sync-pack-source.yml"))
	for _, forbidden := range []string{"schedule:", "push:", "pull_request:", "repository_dispatch:", "cancel-in-progress: true", "issues: write", "actions: write", "auto-merge"} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("synchronization workflow contains forbidden capability %q", forbidden)
		}
	}
	for _, required := range []string{
		"workflow_dispatch:", "permissions: {}", "group: sync-pack-source-${{ inputs.source_id }}", "cancel-in-progress: false",
		"run-name: sync-pack-source / ${{ inputs.source_id }} / ${{ inputs.request_digest }}", "MATTY_REQUEST_DIGEST: ${{ inputs.request_digest }}",
		"inspect:", "classify:", "validate:", "publish:", "needs: [inspect, classify, validate]", "contents: write", "pull-requests: write",
		"--phase validate", "steps.route.outputs.noop", "matty-sync/inspect/no-op.json", "pack-source-publication-${{ github.run_id }}", "retention-days: 30",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("synchronization workflow missing %q", required)
		}
	}
	for _, line := range strings.Split(workflow, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "uses:") && !strings.HasPrefix(line, "- uses:") {
			continue
		}
		at := strings.LastIndex(line, "@")
		if at < 0 || len(strings.TrimSpace(line[at+1:])) != 40 {
			t.Fatalf("action is not pinned by a full SHA: %q", line)
		}
	}
	inspect := workflowSection(t, workflow, "  inspect:", "  classify:")
	classify := workflowSection(t, workflow, "  classify:", "  validate:")
	validate := workflowSection(t, workflow, "  validate:", "  publish:")
	publish := workflow[strings.Index(workflow, "  publish:"):]
	for name, section := range map[string]string{"inspect": inspect, "validate": validate, "publish": publish} {
		if !strings.Contains(section, "GITHUB_TOKEN: ${{ github.token }}") {
			t.Fatalf("%s acquisition does not receive the job-scoped GitHub token", name)
		}
	}
	if strings.Contains(inspect, "contents: write") || strings.Contains(inspect, "pull-requests: write") || strings.Contains(classify, "contents: write") || strings.Contains(classify, "pull-requests: write") || strings.Contains(validate, "contents: write") || strings.Contains(validate, "pull-requests: write") {
		t.Fatal("Inspect, Classify, or Validate has publication permission")
	}
	if !strings.Contains(classify, "models: read") || !strings.Contains(publish, "contents: write") || !strings.Contains(publish, "pull-requests: write") {
		t.Fatal("phase permissions do not match the accepted minimum")
	}
}

func TestSynchronizationSchemasAreCanonicalAndForbidSensitivePayloads(t *testing.T) {
	root := filepath.Join(repositoryRoot(t), "workflows", "schemas")
	for _, name := range []string{"pack-source-dispatch.schema.json", "pack-source-operational-artifact.schema.json", "pack-source-publication.schema.json", "pack-source-validation.schema.json", "pack-source-noop.schema.json"} {
		contents := readFile(t, filepath.Join(root, name))
		for _, required := range []string{`"$schema"`, `"additionalProperties": false`, `"schema_version"`} {
			if !strings.Contains(contents, required) {
				t.Fatalf("%s missing %s", name, required)
			}
		}
		for _, forbidden := range []string{`"secret"`, `"token"`, `"upstream_bytes"`, `"upstream_payload"`} {
			if strings.Contains(contents, forbidden) {
				t.Fatalf("%s permits forbidden payload %s", name, forbidden)
			}
		}
	}
}

func TestDispatchSchemaMatchesRuntimeValidation(t *testing.T) {
	sha := strings.Repeat("a", 40)
	cases := []packsyncworkflow.DispatchRequest{
		{SchemaVersion: 1, SourceID: "source", Selector: packsyncworkflow.SelectorLatestStable, ClassificationMode: packsyncworkflow.ClassificationAI, RequestReason: "inspect"},
		{SchemaVersion: 1, SourceID: "source", Selector: packsyncworkflow.SelectorCommit, SelectorRef: sha, ClassificationMode: packsyncworkflow.ClassificationHuman, RequestReason: "inspect"},
		{SchemaVersion: 1, SourceID: "source", Selector: packsyncworkflow.SelectorCommit, SelectorRef: sha, ClassificationMode: packsyncworkflow.ClassificationHuman, RequestReason: "evidence", ExpectedPlanID: "plan", ExpectedBaseSHA: sha, HumanEvidence: json.RawMessage(`{"schema_version":1}`)},
		{SchemaVersion: 1, SourceID: "source", Selector: packsyncworkflow.SelectorLatestStable, SelectorRef: "unexpected", ClassificationMode: packsyncworkflow.ClassificationAI, RequestReason: "invalid"},
		{SchemaVersion: 1, SourceID: "source", Selector: packsyncworkflow.SelectorCommit, SelectorRef: sha, ClassificationMode: packsyncworkflow.ClassificationHuman, RequestReason: "partial", ExpectedPlanID: "plan"},
		{SchemaVersion: 1, SourceID: "source", Selector: packsyncworkflow.SelectorLatestStable, ClassificationMode: packsyncworkflow.ClassificationAI, RequestReason: " \t\n"},
	}
	for _, request := range cases {
		data, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		schemaErr := validateSchemaInstance(t, "pack-source-dispatch.schema.json", data)
		if (request.Validate() == nil) != (schemaErr == nil) {
			t.Fatalf("runtime/schema dispatch disagreement for %s: runtime=%v schema=%v", data, request.Validate(), schemaErr)
		}
	}
}

func TestMaintainerSkillFixturesCoverCanonicalRequestsAndMonitoring(t *testing.T) {
	type requestFixture struct {
		Name    string          `json:"name"`
		Intent  string          `json:"intent"`
		Valid   bool            `json:"valid"`
		Request json.RawMessage `json:"request"`
	}
	type monitoringFixture struct {
		Name       string `json:"name"`
		Operation  string `json:"operation"`
		SameDigest bool   `json:"same_digest"`
		RunStatus  string `json:"run_status"`
		Artifact   string `json:"artifact"`
		Dispatches int    `json:"dispatches"`
		State      string `json:"state"`
	}
	var fixtures struct {
		Requests   []requestFixture    `json:"requests"`
		Monitoring []monitoringFixture `json:"monitoring"`
	}
	path := filepath.Join(repositoryRoot(t), "internal", "ci", "testdata", "sync-pack-source-skill.json")
	if err := json.Unmarshal([]byte(readFile(t, path)), &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures.Requests {
		var request packsyncworkflow.DispatchRequest
		runtimeErr := json.Unmarshal(fixture.Request, &request)
		if runtimeErr == nil {
			runtimeErr = request.Validate()
		}
		schemaErr := validateSchemaInstance(t, "pack-source-dispatch.schema.json", fixture.Request)
		if (runtimeErr == nil) != fixture.Valid || (schemaErr == nil) != fixture.Valid {
			t.Fatalf("request fixture %s valid=%t runtime=%v schema=%v", fixture.Name, fixture.Valid, runtimeErr, schemaErr)
		}
		if fixture.Intent == "" {
			t.Fatalf("request fixture %s has no maintainer intent", fixture.Name)
		}
	}
	testMaintainerDispatchRenderer(t, fixtures.Requests[0].Request)
	var canonical packsyncworkflow.DispatchRequest
	if err := json.Unmarshal(fixtures.Requests[0].Request, &canonical); err != nil {
		t.Fatal(err)
	}
	digest, err := canonical.Digest()
	if err != nil {
		t.Fatal(err)
	}
	root := repositoryRoot(t)
	attachScript := filepath.Join(root, ".agents", "skills", "sync-pack-source", "scripts", "attach.sh")
	resultScript := filepath.Join(root, ".agents", "skills", "sync-pack-source", "scripts", "result-state.sh")
	for _, fixture := range fixtures.Monitoring {
		workspace := t.TempDir()
		requestPath := filepath.Join(workspace, "request.json")
		writeFile(t, requestPath, string(fixtures.Requests[0].Request))
		artifacts := filepath.Join(workspace, "artifacts")
		if err := os.MkdirAll(artifacts, 0o755); err != nil {
			t.Fatal(err)
		}
		if fixture.Artifact == "request" {
			writeFile(t, filepath.Join(artifacts, "42-request.json"), string(fixtures.Requests[0].Request))
		}
		runsPath := filepath.Join(workspace, "runs.json")
		runs := []map[string]any{}
		if fixture.RunStatus != "none" {
			titleDigest := "different"
			if fixture.SameDigest {
				titleDigest = digest
			}
			runs = append(runs, map[string]any{"databaseId": 42, "displayTitle": "sync-pack-source / mattpocock-skills / " + titleDigest, "status": fixture.RunStatus, "url": "https://github.com/yersonargotev/matty/actions/runs/42"})
		}
		runsJSON, _ := json.Marshal(runs)
		writeFile(t, runsPath, string(runsJSON))
		attachErr := exec.Command(attachScript, requestPath, runsPath, artifacts).Run()
		attached := attachErr == nil
		attachBlocked := false
		if exit, ok := attachErr.(*exec.ExitError); ok && exit.ExitCode() == 2 {
			attachBlocked = true
		}
		dispatches := 1
		if attached || attachBlocked || fixture.Operation == "monitor" {
			dispatches = 0
		}
		state := "solicitud aceptada"
		if attachBlocked {
			state = "bloqueada"
		} else if fixture.Name == "interrupted" {
			state = "pendiente"
		} else if dispatches == 0 {
			runPath := filepath.Join(workspace, "run.json")
			writeFile(t, runPath, fmt.Sprintf(`{"status":%q}`, fixture.RunStatus))
			prPath, mainPath := writeMaintainerArtifactFixture(t, artifacts, fixture.Artifact)
			output, err := exec.Command(resultScript, runPath, artifacts, prPath, mainPath).CombinedOutput()
			if err != nil {
				t.Fatalf("monitoring fixture %s: %v: %s", fixture.Name, err, output)
			}
			state = strings.TrimSpace(string(output))
		}
		if dispatches != fixture.Dispatches || state != fixture.State {
			t.Fatalf("monitoring fixture %s = dispatches %d state %q, want %d %q", fixture.Name, dispatches, state, fixture.Dispatches, fixture.State)
		}
	}
}

func writeMaintainerArtifactFixture(t *testing.T, artifacts, kind string) (string, string) {
	t.Helper()
	sha, head, hash := strings.Repeat("a", 40), strings.Repeat("c", 40), strings.Repeat("b", 64)
	title, body := "managed", "managed body\n<!-- matty-pack-sync:fixture -->\n"
	metadataHash := packsyncworkflow.ManagedMetadataHash(title, body)
	var name string
	var instance any
	switch kind {
	case "noop":
		name = "pack-source-noop.schema.json"
		instance = map[string]any{"schema_version": 1, "state": "no-op", "source_id": "mattpocock-skills", "plan_id": "plan", "base_sha": sha, "candidate_sha": sha, "contains_secrets": false, "contains_upstream_bytes": false}
	case "publication", "stale-publication", "stale-base-publication", "edited-publication":
		name = "pack-source-publication.schema.json"
		instance = map[string]any{"schema_version": 1, "source_id": "mattpocock-skills", "plan_id": "plan", "base_sha": sha, "candidate_sha": sha, "result_tree_sha": sha, "head_sha": head, "provenance_sha256": hash, "branch_name": "sync/mattpocock-skills", "pr_number": 7, "pr_state_sha256": metadataHash, "managed_title": title, "managed_metadata_hash": metadataHash, "validation": map[string]bool{"provenance": true, "classification": true, "reacquisition": true, "apply": true, "diff": true, "ownership": true, "matty_suite": true}, "decision_ready": true, "auto_merge": false, "manual_merge_required": true, "upstream_content_executed": false, "invalidation_conditions": packsyncworkflow.DecisionReadyInvalidationConditions()}
	case "operational":
		name = "pack-source-operational-artifact.schema.json"
		instance = packsyncworkflow.FailureArtifact{SchemaVersion: 1, State: "blocked", SourceID: "mattpocock-skills", PlanID: "plan", BaseSHA: sha, CandidateSHA: sha, Blockers: []string{"blocked"}, Recovery: []string{"retry safely"}}
	case "inspection":
		writeFile(t, filepath.Join(artifacts, "inspection.json"), `{"schema_version":1}`)
		return "", ""
	default:
		return "", ""
	}
	data, err := json.Marshal(instance)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSchemaInstance(t, name, data); err != nil {
		t.Fatalf("fixture %s rejected: %v", kind, err)
	}
	artifactName := map[string]string{"noop": "no-op.json", "publication": "publication.json", "stale-publication": "publication.json", "stale-base-publication": "publication.json", "edited-publication": "publication.json", "operational": "operational-artifact.json"}[kind]
	writeFile(t, filepath.Join(artifacts, artifactName), string(data))
	if !strings.Contains(kind, "publication") {
		return "", ""
	}
	prPath := filepath.Join(artifacts, "live-pr.json")
	prHead := head
	if kind == "stale-publication" {
		prHead = strings.Repeat("d", 40)
	}
	prBase := sha
	if kind == "stale-base-publication" {
		prBase = strings.Repeat("d", 40)
	}
	prBody := body
	if kind == "edited-publication" {
		prBody = "reviewer edit\n" + body
	}
	prJSON, _ := json.Marshal(map[string]any{"number": 7, "baseRefOid": prBase, "headRefOid": prHead, "headRefName": "sync/mattpocock-skills", "state": "OPEN", "isDraft": false, "title": title, "body": prBody})
	writeFile(t, prPath, string(prJSON))
	mainPath := filepath.Join(artifacts, "remote-main.json")
	writeFile(t, mainPath, fmt.Sprintf(`{"sha":%q}`, sha))
	return prPath, mainPath
}

func testMaintainerDispatchRenderer(t *testing.T, request json.RawMessage) {
	t.Helper()
	root := repositoryRoot(t)
	workspace := t.TempDir()
	requestPath := filepath.Join(workspace, "request.json")
	writeFile(t, requestPath, string(request))
	bin := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	argsPath, stdinPath := filepath.Join(workspace, "args"), filepath.Join(workspace, "stdin")
	fake := `#!/bin/sh
printf '%s\n' "$*" > "$FAKE_GH_ARGS"
cat > "$FAKE_GH_STDIN"
echo https://github.com/yersonargotev/matty/actions/runs/42
`
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, ".agents", "skills", "sync-pack-source", "scripts", "dispatch.sh")
	cmd := exec.Command(script, requestPath)
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "FAKE_GH_ARGS="+argsPath, "FAKE_GH_STDIN="+stdinPath)
	if output, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(output), "/actions/runs/42") {
		t.Fatalf("fixture dispatch = %v: %s", err, output)
	}
	wantArgs := "workflow run .github/workflows/sync-pack-source.yml --repo yersonargotev/matty --ref main --json"
	if got := strings.TrimSpace(readFile(t, argsPath)); got != wantArgs {
		t.Fatalf("gh args = %q, want %q", got, wantArgs)
	}
	var inputs map[string]string
	if err := json.Unmarshal([]byte(readFile(t, stdinPath)), &inputs); err != nil {
		t.Fatal(err)
	}
	var canonical packsyncworkflow.DispatchRequest
	if err := json.Unmarshal(request, &canonical); err != nil {
		t.Fatal(err)
	}
	digest, _ := canonical.Digest()
	if inputs["request_digest"] != digest || inputs["source_id"] != canonical.SourceID || inputs["selector"] != string(canonical.Selector) || inputs["classification_mode"] != string(canonical.ClassificationMode) || inputs["request_reason"] != canonical.RequestReason {
		t.Fatalf("workflow inputs = %#v", inputs)
	}
	if _, present := inputs["schema_version"]; present {
		t.Fatal("workflow transport contains schema_version")
	}
}

func TestDispatchSchemaMatchesRuntimeFieldPresence(t *testing.T) {
	base := `{"schema_version":1,"source_id":"source","selector":"latest-stable","classification_mode":"ai","request_reason":"inspect"}`
	cases := []string{
		base,
		strings.TrimSuffix(base, "}") + `,"selector_ref":""}`,
		strings.TrimSuffix(base, "}") + `,"retry_of_run":""}`,
		strings.TrimSuffix(base, "}") + `,"expected_plan_id":""}`,
		strings.TrimSuffix(base, "}") + `,"unexpected":true}`,
		fmt.Sprintf(`{"schema_version":1,"source_id":"source","selector":"commit","selector_ref":"%s","classification_mode":"human","request_reason":"evidence","expected_plan_id":"plan","expected_base_sha":"%s","human_evidence":{"schema_version":1}}`, strings.Repeat("a", 40), strings.Repeat("b", 40)),
	}
	for _, document := range cases {
		var request packsyncworkflow.DispatchRequest
		runtimeErr := json.Unmarshal([]byte(document), &request)
		if runtimeErr == nil {
			runtimeErr = request.Validate()
		}
		schemaErr := validateSchemaInstance(t, "pack-source-dispatch.schema.json", []byte(document))
		if (runtimeErr == nil) != (schemaErr == nil) {
			t.Fatalf("runtime/schema presence disagreement for %s: runtime=%v schema=%v", document, runtimeErr, schemaErr)
		}
	}
}

func TestSynchronizationSchemasAcceptCanonicalRuntimeArtifacts(t *testing.T) {
	sha, hash := strings.Repeat("a", 40), strings.Repeat("b", 64)
	instances := map[string]any{
		"pack-source-operational-artifact.schema.json": packsyncworkflow.FailureArtifact{SchemaVersion: 1, State: "blocked", SourceID: "source", PlanID: "plan", BaseSHA: sha, CandidateSHA: sha, Blockers: []string{"blocked"}, Recovery: []string{"retry safely"}},
		"pack-source-validation.schema.json":           packsyncworkflow.ValidationArtifact{SchemaVersion: 1, SourceID: "source", PlanID: "plan", BaseSHA: sha, CandidateSHA: sha, MattySuite: true, Apply: true},
		"pack-source-publication.schema.json":          map[string]any{"schema_version": 1, "source_id": "source", "plan_id": "plan", "base_sha": sha, "candidate_sha": sha, "result_tree_sha": sha, "head_sha": strings.Repeat("c", 40), "provenance_sha256": hash, "branch_name": "sync/source", "pr_number": 7, "pr_state_sha256": hash, "managed_title": "managed", "managed_metadata_hash": hash, "validation": map[string]bool{"provenance": true, "classification": true, "reacquisition": true, "apply": true, "diff": true, "ownership": true, "matty_suite": true}, "decision_ready": true, "auto_merge": false, "manual_merge_required": true, "upstream_content_executed": false, "invalidation_conditions": packsyncworkflow.DecisionReadyInvalidationConditions()},
		"pack-source-noop.schema.json":                 map[string]any{"schema_version": 1, "state": "no-op", "source_id": "source", "plan_id": "plan", "base_sha": sha, "candidate_sha": sha, "contains_secrets": false, "contains_upstream_bytes": false},
	}
	for name, instance := range instances {
		data, err := json.Marshal(instance)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateSchemaInstance(t, name, data); err != nil {
			t.Fatalf("%s rejected canonical runtime artifact: %v", name, err)
		}
	}
}

func TestOperationalArtifactSchemaMatchesRuntimeValidation(t *testing.T) {
	sha := strings.Repeat("a", 40)
	valid := packsyncworkflow.FailureArtifact{SchemaVersion: 1, State: "blocked", SourceID: "source", PlanID: "plan", BaseSHA: sha, CandidateSHA: sha, Blockers: []string{"blocked"}, Recovery: []string{"retry safely"}, RunURL: "https://github.com/owner/repo/actions/runs/1"}
	cases := []packsyncworkflow.FailureArtifact{
		valid,
		func() packsyncworkflow.FailureArtifact { value := valid; value.State = "failed"; return value }(),
		func() packsyncworkflow.FailureArtifact { value := valid; value.SourceID = "../source"; return value }(),
		func() packsyncworkflow.FailureArtifact { value := valid; value.BaseSHA = "bad"; return value }(),
		func() packsyncworkflow.FailureArtifact { value := valid; value.CandidateSHA = "bad"; return value }(),
		func() packsyncworkflow.FailureArtifact {
			value := valid
			value.Blockers = []string{"blocked", "blocked"}
			return value
		}(),
		func() packsyncworkflow.FailureArtifact { value := valid; value.Recovery = []string{""}; return value }(),
		func() packsyncworkflow.FailureArtifact { value := valid; value.RunURL = "://bad"; return value }(),
		func() packsyncworkflow.FailureArtifact { value := valid; value.ContainsSecrets = true; return value }(),
	}
	for _, artifact := range cases {
		data, err := json.Marshal(artifact)
		if err != nil {
			t.Fatal(err)
		}
		schemaErr := validateSchemaInstance(t, "pack-source-operational-artifact.schema.json", data)
		_, runtimeErr := artifact.CanonicalJSON()
		if (runtimeErr == nil) != (schemaErr == nil) {
			t.Fatalf("runtime/schema operational artifact disagreement for %s: runtime=%v schema=%v", data, runtimeErr, schemaErr)
		}
	}
}

func validateSchemaInstance(t *testing.T, name string, instance []byte) error {
	t.Helper()
	root := filepath.Join(repositoryRoot(t), "workflows", "schemas")
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(readFile(t, filepath.Join(root, name)))))
	if err != nil {
		t.Fatalf("parse schema %s: %v", name, err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err := compiler.AddResource(name, document); err != nil {
		t.Fatalf("add schema %s: %v", name, err)
	}
	schema, err := compiler.Compile(name)
	if err != nil {
		t.Fatalf("compile schema %s: %v", name, err)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(instance))
	if err != nil {
		return err
	}
	return schema.Validate(value)
}

func workflowSection(t *testing.T, workflow, start, end string) string {
	t.Helper()
	startIndex := strings.Index(workflow, start)
	endIndex := strings.Index(workflow, end)
	if startIndex < 0 || endIndex <= startIndex {
		t.Fatalf("workflow sections %q..%q not found", start, end)
	}
	return workflow[startIndex:endIndex]
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
