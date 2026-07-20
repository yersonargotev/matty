package ci_test

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/yersonargotev/packy/internal/packsync"
	"github.com/yersonargotev/packy/internal/packsyncworkflow"
)

var packyOwnedPackages = []string{
	"./cmd/packy",
	"./internal/addyacceptance",
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

type packSourceSchemaSuite struct {
	version string
	major   int
}

var packSourceSchemaSuites = []packSourceSchemaSuite{{version: "v1.0.0", major: 1}, {version: "v2.0.0", major: 2}}

var packSourceSchemaNames = []string{
	"pack-source-dispatch.schema.json",
	"pack-source-noop.schema.json",
	"pack-source-operational-artifact.schema.json",
	"pack-source-publication.schema.json",
	"pack-source-validation.schema.json",
}

func TestValidationEntrypointOwnsTheExactPackageAllowlist(t *testing.T) {
	root := repositoryRoot(t)
	script := readFile(t, filepath.Join(root, "scripts", "validate-packy.sh"))

	packages := shellArray(t, script, "readonly packages=(")
	if !reflect.DeepEqual(packages, packyOwnedPackages) {
		t.Fatalf("validation package allowlist = %#v, want %#v", packages, packyOwnedPackages)
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
	if strings.Count(workflow, "run: ./scripts/validate-packy.sh") != 1 {
		t.Fatal("CI must invoke the repository validation authority exactly once")
	}
	for _, unsafe := range []string{"go test", "go vet", "go build", "gofmt"} {
		if strings.Contains(workflow, "run: "+unsafe) {
			t.Fatalf("CI bypasses validation entrypoint with %q", unsafe)
		}
	}
}

func TestAddyAcceptanceValidationKeepsStableRowsAndBatchesFreshExactTests(t *testing.T) {
	script := readFile(t, filepath.Join(repositoryRoot(t), "scripts", "validate-addy-acceptance.sh"))
	mappings, pairs := addyAcceptanceMappings(t, script)
	wantMappings := []string{
		"map_row 1 ./internal/addyacceptance TestExactUpstreamArchiveInventoryAndSupportRemainInert",
		"map_row 2 ./internal/addyacceptance TestUnsafeArchiveTwinBlocksAndCleansBeforeExecution TestExactUpstreamArchiveInventoryAndSupportRemainInert",
		"map_row 4 ./internal/packsync TestLoadConfigRejectsPathUnsafeSourceIDsAndSharedBindings TestValidatePreconditionsRejectsUnrelatedSourceGenerationWithoutMutation",
		"map_row 6 ./internal/addyacceptance TestCanonicalInventoryAndDeterminism TestOneFactNegativeTwinBlocksCompleteInventory",
		"map_row 7 ./internal/capabilitypack TestDiscoverRejectsInvalidManifestV2Contracts TestCompleteAddyCohortUsesTypedConsentFreshVerificationAndExactNoOp",
		"map_row 8 ./internal/addyacceptance TestExactUpstreamArchiveInventoryAndSupportRemainInert TestCompleteSurfaceCohortsAreDeterministicInertAndIndependent",
		"map_row 9 ./internal/ci TestPackSourceV2SchemasAcceptCanonicalRuntimeContracts TestSynchronizationSchemasAcceptCanonicalRuntimeArtifacts",
		"map_row 10 ./internal/packclassification TestHumanClassificationRequiresInspectionThenBoundEvidenceDispatch",
		"map_row 11 ./internal/addyacceptance TestLifecycleOracleExposesExactCountsAuthoritiesAndSurfaceBindings",
		"map_row 12 ./internal/addyacceptance TestCompleteSurfaceCohortsAreDeterministicInertAndIndependent",
		"map_row 13 ./internal/addyacceptance TestCompleteSurfaceCohortsAreDeterministicInertAndIndependent",
		"map_row 14 ./internal/capabilitypack TestCompleteAddyCollisionBlocksUntilExactSurfaceAliasReplans",
		"map_row 15 ./internal/capabilitypack TestCompleteAddyCohortStalePreflightAndAtomicFailureRequireFreshRecovery",
		"map_row 16 ./internal/capabilitypack TestCompleteAddyDualSurfaceFailurePreservesAuthorizedOtherSurface TestCompleteAddyAliasesRemainSurfaceLocalAndSharedRemovalRetainsContributor",
		"map_row 17 ./internal/capabilitypack TestCompleteAddyCohortUsesTypedConsentFreshVerificationAndExactNoOp",
		"map_row 18 ./internal/capabilitypack TestCompleteAddyAtomicAdapterFailureRecordsAttemptAndRequiresFreshRecoveryPlan",
		"map_row 19 ./internal/capabilitypack TestCompleteAddyReadinessKeepsUnknownPendingOptionalAndExcludedDistinct",
		"map_row 19 ./internal/cli TestPackStatusJSONRequireEmitsDocumentBeforeGateError TestPackStatusRequireUsableIsIndependentNonInteractiveGate",
		"map_row 20 ./internal/capabilitypack TestCompleteAddyReadinessKeepsUnknownPendingOptionalAndExcludedDistinct",
		"map_row 21 ./internal/capabilitypack TestCompleteAddyCohortUsesTypedConsentFreshVerificationAndExactNoOp TestUpdateRejectsStaleCatalogAndExactPlanApproval",
		"map_row 22 ./internal/capabilitypack TestCompleteAddyExactOwnershipRemovalBlocksDriftWithoutEffects TestCompleteAddyAliasesRemainSurfaceLocalAndSharedRemovalRetainsContributor",
		"map_row 23 ./internal/tools/syncpacksource TestAddyRegistrationTracerProvesExactEndToEndAdmission",
		"map_row 23 ./internal/packsync TestCheckSealsAbsentSourceRegistrationWithoutPersistingIt TestApplyCommitsRegistrationConfigurationLockAndContributionAtomically",
		"map_row 24 ./internal/packsync TestCheckRejectsRegistrationWithExistingSourceOrBindingOwner TestApplyCommitsRegistrationConfigurationLockAndContributionAtomically",
		"map_row 24 ./internal/tools/syncpacksource TestAddyRegistrationTracerProvesExactEndToEndAdmission",
		"map_row 24 ./internal/ci TestPackSourceV2RegistrationSemanticAndNullArrayValidation TestSyncWorkflowIsManualPinnedLeastPrivilegeAndPhaseSeparated",
	}
	if !reflect.DeepEqual(mappings, wantMappings) {
		t.Fatalf("Addy acceptance mappings = %#v, want stable matrix %#v", mappings, wantMappings)
	}
	if len(pairs) != 29 {
		t.Fatalf("Addy acceptance unique package/test pairs = %d, want 29", len(pairs))
	}

	result := runAddyAcceptanceValidation(t, script, nil)
	if result.err != nil {
		t.Fatalf("validation failed: %v\n%s", result.err, result.output)
	}
	invocations := strings.Split(strings.TrimSpace(result.log), "\n")
	if len(invocations) != 14 { // seven package listings, then seven executions
		t.Fatalf("go invocations = %d, want 14:\n%s", len(invocations), result.log)
	}
	seenExecution := false
	seenPairs := make(map[string]int)
	for _, invocation := range invocations {
		fields := strings.Split(invocation, "\t")
		if len(fields) < 4 || fields[0] != "test" {
			t.Fatalf("malformed fake-go invocation %q", invocation)
		}
		packagePath := fields[1]
		if fields[2] == "-list" {
			if seenExecution {
				t.Fatalf("listing occurred after execution began: %q", invocation)
			}
			continue
		}
		seenExecution = true
		joined := strings.Join(fields[2:], "\t")
		if !strings.Contains(joined, "\t-count=1") {
			t.Fatalf("execution is not fresh: %q", invocation)
		}
		runIndex := -1
		for i, field := range fields {
			if field == "-run" {
				runIndex = i
				break
			}
		}
		if runIndex < 0 || runIndex+1 >= len(fields) {
			t.Fatalf("execution lacks exact run expression: %q", invocation)
		}
		expression := strings.TrimSuffix(strings.TrimPrefix(fields[runIndex+1], "^("), ")$")
		for _, testName := range strings.Split(expression, "|") {
			seenPairs[packagePath+"/"+testName]++
		}
	}
	if !reflect.DeepEqual(seenPairs, pairs) {
		t.Fatalf("executed package/test pairs = %#v, want each mapped pair once %#v", seenPairs, pairs)
	}
}

func TestAddyAcceptanceValidationFailsClosedBeforeExecution(t *testing.T) {
	script := readFile(t, filepath.Join(repositoryRoot(t), "scripts", "validate-addy-acceptance.sh"))
	tests := []struct {
		name        string
		script      string
		environment map[string]string
		want        string
	}{
		{name: "malformed mapping", script: strings.Replace(script, "map_row 1 ./internal/addyacceptance", "map_row nope ./internal/addyacceptance", 1), want: "malformed Addy acceptance mapping"},
		{name: "unknown package", script: script, environment: map[string]string{"FAIL_LIST_PACKAGE": "./internal/cli"}, want: "package validation failed for ./internal/cli (rows 19)"},
		{name: "missing exact test", script: script, environment: map[string]string{"OMIT_TEST": "TestLifecycleOracleExposesExactCountsAuthoritiesAndSurfaceBindings"}, want: "missing exact test ./internal/addyacceptance/TestLifecycleOracleExposesExactCountsAuthoritiesAndSurfaceBindings (rows 11)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runAddyAcceptanceValidation(t, test.script, test.environment)
			if result.err == nil || !strings.Contains(result.output, test.want) {
				t.Fatalf("result = %v\n%s, want failure containing %q", result.err, result.output, test.want)
			}
			for _, invocation := range strings.Split(strings.TrimSpace(result.log), "\n") {
				if strings.Contains(invocation, "\t-run\t") {
					t.Fatalf("test execution occurred before complete validation: %q", invocation)
				}
			}
		})
	}
}

func TestAddyAcceptanceFailureDiagnosticsRetainEveryAffectedRow(t *testing.T) {
	script := readFile(t, filepath.Join(repositoryRoot(t), "scripts", "validate-addy-acceptance.sh"))
	result := runAddyAcceptanceValidation(t, script, map[string]string{"FAIL_TEST": "TestCompleteSurfaceCohortsAreDeterministicInertAndIndependent"})
	if result.err == nil || !strings.Contains(result.output, "rows 8, 12, 13") {
		t.Fatalf("duplicate test failure lost reverse row trace:\n%s", result.output)
	}

	result = runAddyAcceptanceValidation(t, script, map[string]string{"FAIL_EXEC_PACKAGE": "./internal/cli"})
	if result.err == nil || !strings.Contains(result.output, "package execution failed for ./internal/cli (rows 19)") {
		t.Fatalf("package failure lost affected rows:\n%s", result.output)
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
		"run-name: sync-pack-source / ${{ inputs.source_id }} / ${{ inputs.request_digest }}", "PACKY_REQUEST_DIGEST: ${{ inputs.request_digest }}",
		"operation:", "registration_json:", "registration_sha256:", "PACKY_OPERATION: ${{ inputs.operation }}", "PACKY_REGISTRATION_JSON: ${{ inputs.registration_json }}", "PACKY_REGISTRATION_SHA256: ${{ inputs.registration_sha256 }}",
		"inspect:", "classify:", "validate:", "publish:", "needs: [inspect, classify, validate]", "contents: write", "pull-requests: write",
		"--phase validate", "steps.route.outputs.noop", "packy-sync/inspect/no-op.json", "pack-source-publication-${{ github.run_id }}", "retention-days: 30",
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
	repository := repositoryRoot(t)
	if _, err := os.Stat(filepath.Join(repository, "workflows", "schemas")); !os.IsNotExist(err) {
		t.Fatalf("legacy workflows/schemas path still exists: %v", err)
	}
	if err := filepath.Walk(filepath.Join(repository, "schemas"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) != ".json" {
			t.Fatalf("checked-in Pages tree contains non-JSON file %s", path)
		}
		if strings.Contains(filepath.ToSlash(path), "/latest/") {
			t.Fatalf("checked-in Pages tree contains forbidden latest alias %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	ids := make(map[string]bool, len(packSourceSchemaNames)*len(packSourceSchemaSuites))
	var schemaContents []string
	for _, suite := range packSourceSchemaSuites {
		root := filepath.Join(repository, "schemas", "pack-source", suite.version)
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, entry := range entries {
			if entry.IsDir() {
				t.Fatalf("schema suite %s contains directory %s", suite.version, entry.Name())
			}
			names = append(names, entry.Name())
		}
		if !reflect.DeepEqual(names, packSourceSchemaNames) {
			t.Fatalf("schema suite %s files = %v, want exact complete suite %v", suite.version, names, packSourceSchemaNames)
		}
		baseID := "https://yersonargotev.github.io/packy/schemas/pack-source/" + suite.version + "/"
		for _, name := range packSourceSchemaNames {
			contents := readFile(t, filepath.Join(root, name))
			schemaContents = append(schemaContents, contents)
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
			var schema map[string]any
			if err := json.Unmarshal([]byte(contents), &schema); err != nil {
				t.Fatalf("%s is not valid JSON: %v", name, err)
			}
			wantID := baseID + name
			if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" || schema["$id"] != wantID {
				t.Fatalf("%s identities = $schema %v $id %v, want Draft 2020-12 and %s", name, schema["$schema"], schema["$id"], wantID)
			}
			if ids[wantID] {
				t.Fatalf("duplicate canonical schema ID %s", wantID)
			}
			ids[wantID] = true
			properties, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("%s has no properties object", name)
			}
			schemaVersion, ok := properties["schema_version"].(map[string]any)
			if !ok || schemaVersion["const"] != float64(suite.major) {
				t.Fatalf("%s/%s schema_version does not match suite major v%d", suite.version, name, suite.major)
			}
			document, err := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(contents)))
			if err != nil {
				t.Fatalf("parse schema %s: %v", name, err)
			}
			if err := compiler.AddResource(wantID, document); err != nil {
				t.Fatalf("register schema %s by canonical ID: %v", name, err)
			}
		}
	}
	for _, contents := range schemaContents {
		for _, forbidden := range []string{"github.com/yersonargotev/packy/workflows/schemas", "github.com/yersonargotev/matty/workflows/schemas", "/latest/"} {
			if strings.Contains(contents, forbidden) {
				t.Fatalf("schema contains forbidden identity %q", forbidden)
			}
		}
	}
	for _, suite := range packSourceSchemaSuites {
		baseID := "https://yersonargotev.github.io/packy/schemas/pack-source/" + suite.version + "/"
		for _, name := range packSourceSchemaNames {
			id := baseID + name
			if _, err := compiler.Compile(id); err != nil {
				t.Fatalf("compile and resolve schema offline by canonical ID %s: %v", id, err)
			}
		}
	}
	for _, path := range []string{
		filepath.Join(repository, "workflows", "pack-source-synchronization.md"),
		filepath.Join(repository, ".agents", "skills", "sync-pack-source", "REQUESTS.md"),
	} {
		contents := readFile(t, path)
		for _, forbidden := range []string{"workflows/schemas/", "github.com/yersonargotev/packy/workflows/schemas", "github.com/yersonargotev/matty/workflows/schemas", "/schemas/pack-source/latest/"} {
			if strings.Contains(contents, forbidden) {
				t.Fatalf("normative document %s contains forbidden schema reference %q", path, forbidden)
			}
		}
	}
}

func TestPackSourceV1SchemaBytesRemainImmutable(t *testing.T) {
	want := map[string]string{
		"pack-source-dispatch.schema.json":             "c759176f7cc20bed520104ce7a5d732b2318b29c0442f80caa1b54318f13b571",
		"pack-source-noop.schema.json":                 "596c81c047cea8160190b06e531bda474d748bfc534b01bf44594f701ab26b99",
		"pack-source-operational-artifact.schema.json": "34f3b4e29e69b2f3f0c4e4dd65d2c216a94043ffaa358d25cda659e64ef0224e",
		"pack-source-publication.schema.json":          "e6ec28082e88ad20eb32a5a9ee4142164fd77784278bea9f596e61bf2ae22931",
		"pack-source-validation.schema.json":           "04d2ab6ba1394faab4bfe9d9347c0bcfb5a2ce57622a4ebba8982fbdd36a0da8",
	}
	root := packSourceSchemaRoot(t)
	for name, digest := range want {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != digest {
			t.Fatalf("immutable v1 schema %s digest = %s, want %s", name, got, digest)
		}
	}
}

func TestPackSourceV2SchemasAcceptCanonicalRuntimeContracts(t *testing.T) {
	sha := strings.Repeat("a", 40)
	head := strings.Repeat("c", 40)
	hash := strings.Repeat("b", 64)
	provenance := packsyncworkflow.ArtifactProvenance{SourceLockSHA256: hash, LockSetSHA256: hash, ConfigSHA256: hash, ManifestsSHA256: hash}
	registration := packsync.SourceConfig{
		ID: "addy", Provider: "github", Repository: "addyosmani/agent-skills",
		Selector:  packsync.Selector{Mode: packsync.SelectorStableRelease},
		Resources: []packsync.Binding{{PackID: "addy", Kind: "skill", ResourceID: "idea-refine", UpstreamPath: "skills/idea-refine"}},
	}
	registrationDigest, err := packsyncworkflow.CanonicalRegistrationSHA256(registration)
	if err != nil {
		t.Fatal(err)
	}
	dispatches := []packsyncworkflow.DispatchRequest{
		{SchemaVersion: 2, Operation: packsyncworkflow.OperationSynchronize, SourceID: "addy", Selector: packsyncworkflow.SelectorLatestStable, ClassificationMode: packsyncworkflow.ClassificationAI, RequestReason: "inspect"},
		{SchemaVersion: 2, Operation: packsyncworkflow.OperationRegister, SourceID: "addy", Selector: packsyncworkflow.SelectorLatestStable, ClassificationMode: packsyncworkflow.ClassificationAI, RequestReason: "register", Registration: &registration, RegistrationSHA256: registrationDigest},
	}
	for _, dispatch := range dispatches {
		assertV2RuntimeSchemaParity(t, "pack-source-dispatch.schema.json", dispatch, dispatch.Validate())
	}

	noop := packsyncworkflow.NoopArtifact{SchemaVersion: 2, State: "no-op", SourceID: "addy", PlanID: "plan", BaseSHA: sha, CandidateSHA: sha, ArtifactProvenance: provenance}
	assertV2RuntimeSchemaParity(t, "pack-source-noop.schema.json", noop, noop.Validate())
	failure := packsyncworkflow.FailureArtifact{SchemaVersion: 2, State: "blocked", SourceID: "addy", PlanID: "plan", BaseSHA: sha, CandidateSHA: sha, ArtifactProvenance: provenance, Blockers: []string{"blocked"}, Recovery: []string{"retry safely"}}
	_, failureErr := failure.CanonicalJSON()
	assertV2RuntimeSchemaParity(t, "pack-source-operational-artifact.schema.json", failure, failureErr)
	validation := packsyncworkflow.ValidationArtifact{SchemaVersion: 2, SourceID: "addy", PlanID: "plan", BaseSHA: sha, CandidateSHA: sha, ArtifactProvenance: provenance, ResultTreeSHA: sha, PackySuite: true, Apply: true}
	assertV2RuntimeSchemaParity(t, "pack-source-validation.schema.json", validation, validation.Validate())
	publication := packsyncworkflow.PublicationArtifact{
		SchemaVersion: 2, SourceID: "addy", PlanID: "plan", BaseSHA: sha, CandidateSHA: sha, ArtifactProvenance: provenance,
		ResultTreeSHA: sha, HeadSHA: head, ProvenanceSHA256: hash, BranchName: "sync/addy", PRNumber: 7, PRStateSHA256: hash,
		ManagedTitle: "managed", ManagedMetadataHash: hash,
		Validation:    packsyncworkflow.ValidationGates{Provenance: true, Classification: true, Reacquisition: true, Apply: true, Diff: true, Ownership: true, PackySuite: true},
		DecisionReady: true, ManualMergeRequired: true, InvalidationConditions: packsyncworkflow.DecisionReadyInvalidationConditions(),
	}
	assertV2RuntimeSchemaParity(t, "pack-source-publication.schema.json", publication, publication.Validate())

	invalidNoop := noop
	invalidNoop.LockSetSHA256 = ""
	assertV2RuntimeSchemaParity(t, "pack-source-noop.schema.json", invalidNoop, invalidNoop.Validate())
	invalidFailure := failure
	invalidFailure.PlanID = ""
	_, failureErr = invalidFailure.CanonicalJSON()
	assertV2RuntimeSchemaParity(t, "pack-source-operational-artifact.schema.json", invalidFailure, failureErr)
}

func TestPackSourceV2RegistrationSemanticAndNullArrayValidation(t *testing.T) {
	registration := packsync.SourceConfig{
		ID: "other", Provider: "github", Repository: "addyosmani/agent-skills",
		Selector: packsync.Selector{Mode: packsync.SelectorStableRelease}, Resources: []packsync.Binding{},
	}
	digest, err := packsyncworkflow.CanonicalRegistrationSHA256(registration)
	if err != nil {
		t.Fatal(err)
	}
	mismatch := packsyncworkflow.DispatchRequest{SchemaVersion: 2, Operation: packsyncworkflow.OperationRegister, SourceID: "addy", Selector: packsyncworkflow.SelectorLatestStable, ClassificationMode: packsyncworkflow.ClassificationAI, RequestReason: "register", Registration: &registration, RegistrationSHA256: digest}
	data, err := json.Marshal(mismatch)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSchemaInstanceForSuite(t, "v2.0.0", "pack-source-dispatch.schema.json", data); err != nil {
		t.Fatalf("structural schema rejected representable registration: %v", err)
	}
	if err := mismatch.Validate(); err == nil {
		t.Fatal("runtime accepted registration.id different from source_id")
	}

	nullResources := []byte(`{"schema_version":2,"operation":"register","source_id":"addy","selector":"latest-stable","classification_mode":"ai","request_reason":"register","registration":{"id":"addy","provider":"github","repository":"addyosmani/agent-skills","selector":{"mode":"stable-release"},"resources":null},"registration_sha256":"` + strings.Repeat("a", 64) + `"}`)
	if err := validateSchemaInstanceForSuite(t, "v2.0.0", "pack-source-dispatch.schema.json", nullResources); err == nil {
		t.Fatal("v2 schema accepted null registration resources")
	}
	var request packsyncworkflow.DispatchRequest
	if err := json.Unmarshal(nullResources, &request); err != nil {
		t.Fatal(err)
	}
	if err := request.Validate(); err == nil {
		t.Fatal("runtime accepted null registration resources")
	}
}

func assertV2RuntimeSchemaParity(t *testing.T, name string, value any, runtimeErr error) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	schemaErr := validateSchemaInstanceForSuite(t, "v2.0.0", name, data)
	if (runtimeErr == nil) != (schemaErr == nil) {
		t.Fatalf("v2 runtime/schema disagreement for %s: runtime=%v schema=%v document=%s", name, runtimeErr, schemaErr, data)
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
		{SchemaVersion: 1, SourceID: "source", Selector: packsyncworkflow.SelectorLatestStable, ClassificationMode: packsyncworkflow.ClassificationAI, RequestReason: "\v"},
		{SchemaVersion: 1, SourceID: "source", Selector: packsyncworkflow.SelectorLatestStable, ClassificationMode: packsyncworkflow.ClassificationAI, RequestReason: "\u00a0"},
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
		schemaErr := validateSchemaInstanceForSuite(t, fmt.Sprintf("v%d.0.0", request.SchemaVersion), "pack-source-dispatch.schema.json", fixture.Request)
		if (runtimeErr == nil) != fixture.Valid || (schemaErr == nil) != fixture.Valid {
			t.Fatalf("request fixture %s valid=%t runtime=%v schema=%v", fixture.Name, fixture.Valid, runtimeErr, schemaErr)
		}
		if fixture.Intent == "" {
			t.Fatalf("request fixture %s has no maintainer intent", fixture.Name)
		}
	}
	for _, fixture := range fixtures.Requests {
		if fixture.Valid {
			testMaintainerDispatchRenderer(t, fixture.Request)
		}
	}
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
			runs = append(runs, map[string]any{"databaseId": 42, "displayTitle": "sync-pack-source / mattpocock-skills / " + titleDigest, "status": fixture.RunStatus, "url": "https://github.com/yersonargotev/packy/actions/runs/42"})
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
	title, body := "managed", "managed body\n<!-- packy-pack-sync:fixture -->\n"
	metadataHash := packsyncworkflow.ManagedMetadataHash(title, body)
	var name string
	var instance any
	switch kind {
	case "noop":
		name = "pack-source-noop.schema.json"
		instance = map[string]any{"schema_version": 1, "state": "no-op", "source_id": "mattpocock-skills", "plan_id": "plan", "base_sha": sha, "candidate_sha": sha, "contains_secrets": false, "contains_upstream_bytes": false}
	case "publication", "stale-publication", "stale-base-publication", "edited-publication":
		name = "pack-source-publication.schema.json"
		instance = map[string]any{"schema_version": 1, "source_id": "mattpocock-skills", "plan_id": "plan", "base_sha": sha, "candidate_sha": sha, "result_tree_sha": sha, "head_sha": head, "provenance_sha256": hash, "branch_name": "sync/mattpocock-skills", "pr_number": 7, "pr_state_sha256": metadataHash, "managed_title": title, "managed_metadata_hash": metadataHash, "validation": map[string]bool{"provenance": true, "classification": true, "reacquisition": true, "apply": true, "diff": true, "ownership": true, "packy_suite": true}, "decision_ready": true, "auto_merge": false, "manual_merge_required": true, "upstream_content_executed": false, "invalidation_conditions": packsyncworkflow.DecisionReadyInvalidationConditions()}
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
echo https://github.com/yersonargotev/packy/actions/runs/42
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
	wantArgs := "workflow run .github/workflows/sync-pack-source.yml --repo yersonargotev/packy --ref main --json"
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
	if canonical.SchemaVersion == 2 {
		registration, _ := json.Marshal(canonical.Registration)
		if inputs["operation"] != string(canonical.Operation) || inputs["registration_json"] != string(registration) || inputs["registration_sha256"] != canonical.RegistrationSHA256 {
			t.Fatalf("registration workflow inputs = %#v", inputs)
		}
		if _, present := inputs["registration"]; present {
			t.Fatal("workflow transport contains unmapped registration")
		}
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
		"pack-source-validation.schema.json":           packsyncworkflow.ValidationArtifact{SchemaVersion: 1, SourceID: "source", PlanID: "plan", BaseSHA: sha, CandidateSHA: sha, PackySuite: true, Apply: true},
		"pack-source-publication.schema.json":          map[string]any{"schema_version": 1, "source_id": "source", "plan_id": "plan", "base_sha": sha, "candidate_sha": sha, "result_tree_sha": sha, "head_sha": strings.Repeat("c", 40), "provenance_sha256": hash, "branch_name": "sync/source", "pr_number": 7, "pr_state_sha256": hash, "managed_title": "managed", "managed_metadata_hash": hash, "validation": map[string]bool{"provenance": true, "classification": true, "reacquisition": true, "apply": true, "diff": true, "ownership": true, "packy_suite": true}, "decision_ready": true, "auto_merge": false, "manual_merge_required": true, "upstream_content_executed": false, "invalidation_conditions": packsyncworkflow.DecisionReadyInvalidationConditions()},
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

func TestValidationArtifactSchemaMatchesRuntimeValidation(t *testing.T) {
	sha := strings.Repeat("a", 40)
	valid := packsyncworkflow.ValidationArtifact{SchemaVersion: 1, SourceID: "source", PlanID: "plan", BaseSHA: sha, CandidateSHA: sha, PackySuite: true, Apply: true}
	cases := []packsyncworkflow.ValidationArtifact{
		valid,
		func() packsyncworkflow.ValidationArtifact { value := valid; value.SchemaVersion = 2; return value }(),
		func() packsyncworkflow.ValidationArtifact { value := valid; value.SourceID = "../source"; return value }(),
		func() packsyncworkflow.ValidationArtifact { value := valid; value.PlanID = ""; return value }(),
		func() packsyncworkflow.ValidationArtifact { value := valid; value.BaseSHA = "bad"; return value }(),
		func() packsyncworkflow.ValidationArtifact { value := valid; value.CandidateSHA = "bad"; return value }(),
		func() packsyncworkflow.ValidationArtifact { value := valid; value.PackySuite = false; return value }(),
		func() packsyncworkflow.ValidationArtifact { value := valid; value.Apply = false; return value }(),
		func() packsyncworkflow.ValidationArtifact { value := valid; value.UpstreamBytes = true; return value }(),
	}
	for _, artifact := range cases {
		data, err := json.Marshal(artifact)
		if err != nil {
			t.Fatal(err)
		}
		schemaErr := validateSchemaInstance(t, "pack-source-validation.schema.json", data)
		runtimeErr := artifact.Validate()
		if (runtimeErr == nil) != (schemaErr == nil) {
			t.Fatalf("runtime/schema validation artifact disagreement for %s: runtime=%v schema=%v", data, runtimeErr, schemaErr)
		}
	}
}

func TestPublicationSchemaMatchesRuntimeHashValidation(t *testing.T) {
	sha, hash := strings.Repeat("a", 40), strings.Repeat("b", 64)
	validDocument := map[string]any{"schema_version": 1, "source_id": "source", "plan_id": "plan", "base_sha": sha, "candidate_sha": sha, "result_tree_sha": sha, "head_sha": strings.Repeat("c", 40), "provenance_sha256": hash, "branch_name": "sync/source", "pr_number": 7, "pr_state_sha256": hash, "managed_title": "managed", "managed_metadata_hash": hash, "validation": map[string]bool{"provenance": true, "classification": true, "reacquisition": true, "apply": true, "diff": true, "ownership": true, "packy_suite": true}, "decision_ready": true, "auto_merge": false, "manual_merge_required": true, "upstream_content_executed": false, "invalidation_conditions": packsyncworkflow.DecisionReadyInvalidationConditions()}
	gates := packsyncworkflow.ValidationGates{Provenance: true, Classification: true, Reacquisition: true, Apply: true, Diff: true, Ownership: true, PackySuite: true}
	for _, field := range []string{"provenance_sha256", "managed_metadata_hash", "pr_state_sha256"} {
		for _, invalid := range []string{strings.Repeat("g", 64), strings.Repeat("A", 64)} {
			document := make(map[string]any, len(validDocument))
			for key, value := range validDocument {
				document[key] = value
			}
			document[field] = invalid
			data, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			schemaErr := validateSchemaInstance(t, "pack-source-publication.schema.json", data)
			var runtimeErr error
			switch field {
			case "provenance_sha256", "managed_metadata_hash":
				proposal := packsyncworkflow.Proposal{SourceID: "source", PlanID: "plan", BaseSHA: sha, CandidateSHA: sha, ResultTreeSHA: sha, HeadSHA: strings.Repeat("c", 40), ProvenanceSHA256: hash, ManagedTitle: "managed", ManagedMetadataHash: hash, Validation: gates, InvalidationConditions: packsyncworkflow.DecisionReadyInvalidationConditions()}
				if field == "provenance_sha256" {
					proposal.ProvenanceSHA256 = invalid
				} else {
					proposal.ManagedMetadataHash = invalid
				}
				_, runtimeErr = packsyncworkflow.EvaluatePublication(proposal, packsyncworkflow.PublicationState{})
			case "pr_state_sha256":
				identity := packsyncworkflow.ReadinessIdentity{PlanID: "plan", BaseSHA: sha, HeadSHA: strings.Repeat("c", 40), CandidateSHA: sha, ProvenanceSHA256: hash, PRNumber: 7, PRStateSHA256: invalid}
				_, runtimeErr = packsyncworkflow.MarkDecisionReady(identity, gates, false, false)
			}
			if runtimeErr == nil || schemaErr == nil {
				t.Fatalf("publication %s accepted invalid hash %q: runtime=%v schema=%v", field, invalid, runtimeErr, schemaErr)
			}
		}
	}
}

func validateSchemaInstance(t *testing.T, name string, instance []byte) error {
	t.Helper()
	return validateSchemaInstanceForSuite(t, "v1.0.0", name, instance)
}

func validateSchemaInstanceForSuite(t *testing.T, version, name string, instance []byte) error {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	root := filepath.Join(repositoryRoot(t), "schemas", "pack-source", version)
	baseID := "https://yersonargotev.github.io/packy/schemas/pack-source/" + version + "/"
	for _, suiteName := range packSourceSchemaNames {
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(readFile(t, filepath.Join(root, suiteName)))))
		if err != nil {
			t.Fatalf("parse schema %s: %v", suiteName, err)
		}
		if err := compiler.AddResource(baseID+suiteName, document); err != nil {
			t.Fatalf("register schema %s by canonical ID: %v", suiteName, err)
		}
	}
	schema, err := compiler.Compile(baseID + name)
	if err != nil {
		t.Fatalf("compile schema %s: %v", name, err)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(instance))
	if err != nil {
		return err
	}
	return schema.Validate(value)
}

func packSourceSchemaRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join(repositoryRoot(t), "schemas", "pack-source", "v1.0.0")
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
	validationTemp := filepath.Join(tempRoot, "validation-tmp")
	if err := os.MkdirAll(validationTemp, 0o755); err != nil {
		t.Fatal(err)
	}
	commandLog := filepath.Join(tempRoot, "validation-commands.log")
	shimRoot := filepath.Join(tempRoot, "validation-bin")
	// Exercise the real entrypoint while recording, rather than executing, its
	// expensive validation children. The bash shim makes recursion observable.
	for _, command := range []string{"go", "gofmt", "bash"} {
		contents := "#!/bin/sh\n" +
			"printf '" + command + "\\t%s\\t%s' \"$HOME\" \"$XDG_CONFIG_HOME\" >> \"$PACKY_VALIDATION_COMMAND_LOG\"\n" +
			"for arg in \"$@\"; do printf '\\t%s' \"$arg\" >> \"$PACKY_VALIDATION_COMMAND_LOG\"; done\n" +
			"printf '\\n' >> \"$PACKY_VALIDATION_COMMAND_LOG\"\n"
		if command == "bash" {
			contents += "exec /bin/bash \"$@\"\n"
		}
		writeExecutable(t, filepath.Join(shimRoot, command), contents)
	}
	// Addy acceptance owns its own execution contract; this tracer is scoped to
	// the repository entrypoint's package selection and validation classes.
	writeExecutable(t, filepath.Join(tempRoot, "scripts", "validate-addy-acceptance.sh"), "#!/bin/sh\nexit 0\n")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/bash", filepath.Join(tempRoot, "scripts", "validate-packy.sh"))
	cmd.Dir = tempRoot
	cmd.Env = append(os.Environ(),
		"HOME="+operatorHome,
		"XDG_CONFIG_HOME="+operatorXDG,
		"GOCACHE="+filepath.Join(tempRoot, "go-cache"),
		"GOMODCACHE="+filepath.Join(tempRoot, "go-mod-cache"),
		"GOPATH="+filepath.Join(tempRoot, "go-path"),
		"HOSTILE_SENTINEL="+sentinel,
		"PACKY_VALIDATION_COMMAND_LOG="+commandLog,
		"PATH="+shimRoot+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TMPDIR="+validationTemp,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			t.Fatalf("validation entrypoint recursively invoked itself: %v\n%s", ctx.Err(), output)
		}
		t.Fatalf("validation entrypoint failed with hostile unowned content: %v\n%s", err, output)
	}

	invocations := validationInvocations(t, commandLog)
	wantInvocations := [][]string{
		append([]string{"build"}, validationBuildPackages()...),
		append([]string{"vet"}, packyOwnedPackages...),
		append([]string{"test"}, packyOwnedPackages...),
		append([]string{"test", "-race", "-timeout", "10m"}, packyOwnedPackages...),
	}
	var goInvocations [][]string
	formatInvocations := 0
	for _, invocation := range invocations {
		if !validationRootsAreSandboxed(validationTemp, invocation.home, invocation.xdg) {
			t.Fatalf("validation child escaped the entrypoint sandbox: %#v", invocation)
		}
		switch invocation.command {
		case "go":
			goInvocations = append(goInvocations, invocation.args)
		case "gofmt":
			if len(invocation.args) < 2 || invocation.args[0] != "-l" {
				t.Fatalf("format invocation = %#v, want gofmt -l with allowlisted files", invocation.args)
			}
			for _, path := range invocation.args[1:] {
				if !validationPathIsOwned(tempRoot, path) {
					t.Fatalf("format invocation loaded unowned path %q", path)
				}
			}
			formatInvocations++
		case "bash":
			t.Fatalf("validation entrypoint recursively launched bash: %#v", invocation.args)
		default:
			t.Fatalf("unexpected validation command: %#v", invocation)
		}
	}
	if !reflect.DeepEqual(goInvocations, wantInvocations) {
		t.Fatalf("validation Go invocations = %#v, want %#v", goInvocations, wantInvocations)
	}
	if formatInvocations != 1 {
		t.Fatalf("format invocation count = %d, want 1", formatInvocations)
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

type validationInvocation struct {
	command string
	home    string
	xdg     string
	args    []string
}

type addyValidationResult struct {
	output string
	log    string
	err    error
}

func addyAcceptanceMappings(t *testing.T, script string) ([]string, map[string]int) {
	t.Helper()
	var mappings []string
	pairs := make(map[string]int)
	for _, line := range strings.Split(script, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] != "map_row" {
			continue
		}
		mappings = append(mappings, strings.Join(fields, " "))
		for _, testName := range fields[3:] {
			pairs[fields[2]+"/"+testName] = 1
		}
	}
	return mappings, pairs
}

func runAddyAcceptanceValidation(t *testing.T, script string, environment map[string]string) addyValidationResult {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "scripts", "validate-addy-acceptance.sh")
	writeExecutable(t, path, script)
	_, pairs := addyAcceptanceMappings(t, script)
	available := filepath.Join(root, "available-tests")
	var names []string
	for pair := range pairs {
		names = append(names, pair[strings.LastIndex(pair, "/")+1:])
	}
	writeFile(t, available, strings.Join(names, "\n")+"\n")
	bin := filepath.Join(root, "bin")
	logPath := filepath.Join(root, "go.log")
	writeExecutable(t, filepath.Join(bin, "go"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\t' "$@" >>"$GO_LOG"
printf '\n' >>"$GO_LOG"
package="${2-}"
if [[ "${3-}" == "-list" ]]; then
  if [[ "$package" == "${FAIL_LIST_PACKAGE-}" ]]; then
    echo "unknown package $package" >&2
    exit 1
  fi
  while IFS= read -r test; do
    [[ "$test" == "${OMIT_TEST-}" ]] || printf '%s\n' "$test"
  done <"$AVAILABLE_TESTS"
  echo "ok status is not a test name"
  exit 0
fi
if [[ "$package" == "${FAIL_EXEC_PACKAGE-}" ]]; then
  echo "package compile failed" >&2
  exit 1
fi
if [[ -n "${FAIL_TEST-}" && " $* " == *"${FAIL_TEST}"* ]]; then
  echo "--- FAIL: ${FAIL_TEST} (0.00s)" >&2
  exit 1
fi
echo "ok $package"
`)
	cmd := exec.Command("/bin/bash", path)
	cmd.Env = append(os.Environ(),
		"PATH="+bin+":"+os.Getenv("PATH"),
		"GO_LOG="+logPath,
		"AVAILABLE_TESTS="+available,
		"HOME="+filepath.Join(root, "home"),
		"XDG_CONFIG_HOME="+filepath.Join(root, "xdg"),
	)
	for key, value := range environment {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	output, err := cmd.CombinedOutput()
	log, readErr := os.ReadFile(logPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	return addyValidationResult{output: string(output), log: string(log), err: err}
}

func validationInvocations(t *testing.T, path string) []validationInvocation {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var invocations []validationInvocation
	for _, line := range strings.Split(strings.TrimSpace(string(contents)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			t.Fatalf("malformed validation command log line %q", line)
		}
		invocations = append(invocations, validationInvocation{
			command: fields[0],
			home:    fields[1],
			xdg:     fields[2],
			args:    fields[3:],
		})
	}
	return invocations
}

func validationBuildPackages() []string {
	var packages []string
	for _, packagePath := range packyOwnedPackages {
		if packagePath != "./internal/ci" && packagePath != "./internal/release" {
			packages = append(packages, packagePath)
		}
	}
	return packages
}

func validationPathIsOwned(root, path string) bool {
	for _, packagePath := range packyOwnedPackages {
		packageRoot := filepath.Join(root, strings.TrimPrefix(packagePath, "./"))
		if filepath.Dir(path) == packageRoot && filepath.Ext(path) == ".go" {
			return true
		}
	}
	return false
}

func validationRootsAreSandboxed(tempRoot, home, xdg string) bool {
	sandbox := filepath.Dir(home)
	return filepath.Base(home) == "home" &&
		xdg == filepath.Join(sandbox, "xdg") &&
		filepath.Dir(sandbox) == tempRoot &&
		strings.HasPrefix(filepath.Base(sandbox), "packy-validation.")
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

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
}
