package addyacceptance

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestAddyPromotionIndependentInputs(t *testing.T) {
	before, err := CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if got := digest(before); got != "4f5e42be89e9c121c48abdbb77b1312d881f1207887119d50c252c316a1ff02f" || len(before) != 54507 {
		t.Fatalf("original Addy 1.0.0 oracle changed: sha256=%s bytes=%d", got, len(before))
	}
	first, second := CanonicalPromotionHistory(), CanonicalPromotionHistory()
	if !reflect.DeepEqual(first, second) {
		t.Fatal("promotion history fixture changed between reconstructions")
	}
	if first.CatalogAdvertised || first.CurrentVersion != PackVersion {
		t.Fatalf("promotion fixture advertised Addy 1.1.0: %#v", first)
	}
	if len(first.Versions) != 2 || first.Versions[0].Version != "1.0.0" || first.Versions[1].Version != "1.1.0" {
		t.Fatalf("immutable versions = %#v", first.Versions)
	}
	if first.Versions[0].SnapshotSHA256 == first.Versions[1].SnapshotSHA256 {
		t.Fatal("synthetic 1.1.0 candidate did not register its strict adapter-ready source bytes")
	}
	if len(first.Versions[0].Files) != len(first.Versions[1].Files) {
		t.Fatal("synthetic 1.1.0 candidate changed the Addy file inventory")
	}
	for _, version := range first.Versions {
		root := materializePromotionVersion(t, version)
		pack, err := capabilitypack.LoadPortableManifest(filepath.Join(root, "pack.json"), root)
		if err != nil {
			t.Fatalf("strict load Addy %s: %v", version.Version, err)
		}
		if pack.ID != "addy" || pack.Version != version.Version || len(pack.Resources) != 44 {
			t.Fatalf("Addy %s decoded as %#v", version.Version, pack)
		}
		if got := historySnapshotDigest(version.Files); got != version.SnapshotSHA256 {
			t.Fatalf("Addy %s snapshot digest = %s, want %s", version.Version, got, version.SnapshotSHA256)
		}
	}
	assertPromotionV3Contract(t, first.Versions[1])
	after, _ := CanonicalJSON()
	if !bytes.Equal(before, after) {
		t.Fatal("promotion reconstruction changed the original Addy 1.0.0 oracle")
	}

	context := applicablePromotionContext()
	evidence := validApplicablePromotionEvidence(context)
	evidence.Proof.BaseSHA256 = evidence.PackageCandidate
	evidence.Proof.HeadSHA256 = evidence.PackageCandidate
	evidence.Proof.HistorySHA256 = evidence.PackageCandidate
	evidence.Proof.DiffSHA256 = evidence.PackageCandidate
	if err := ValidatePromotionEvidence(evidence, context); err == nil || !strings.Contains(err.Error(), "independent reconstruction") {
		t.Fatalf("candidate self-authorization was accepted: %v", err)
	}
	assertSyntheticHarnessRows(t, 1, 2, 3)
}

func TestCanonicalPromotionCurrentIsDetachedAndWritesExactSnapshot(t *testing.T) {
	current := CanonicalPromotionCurrent()
	var registered ImmutableVersionFixture
	for _, version := range CanonicalPromotionHistory().Versions {
		if version.Version == "1.1.0" {
			registered = version
		}
	}
	if !reflect.DeepEqual(current, registered) {
		t.Fatal("current adapter fixture diverges from the exact registered 1.1.0 artifact")
	}
	if current.Version != "1.1.0" || current.SchemaVersion != 3 ||
		!reflect.DeepEqual(current.Surfaces, []string{"claude", "codex", "opencode"}) {
		t.Fatalf("current promotion fixture = %#v", current)
	}
	var manifest Manifest
	if err := json.Unmarshal(current.Manifest, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != current.Version || manifest.SchemaVersion != current.SchemaVersion || len(manifest.Resources) != 44 {
		t.Fatalf("current manifest = %#v", manifest)
	}

	current.Manifest[0] ^= 0xff
	current.Files[0].Content = "mutated"
	again := CanonicalPromotionCurrent()
	if bytes.Equal(current.Manifest, again.Manifest) || again.Files[0].Content == "mutated" {
		t.Fatal("current promotion fixture exposed shared bytes")
	}

	root := filepath.Join(t.TempDir(), "snapshot")
	if err := WriteCanonicalPromotionCurrent(root); err != nil {
		t.Fatal(err)
	}
	for _, file := range again.Files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file.Path)))
		if err != nil || string(data) != file.Content {
			t.Fatalf("snapshot file %s = %q, %v", file.Path, data, err)
		}
	}
	if err := WriteCanonicalPromotionCurrent(root); err == nil || !strings.Contains(err.Error(), "must be empty") {
		t.Fatalf("non-empty snapshot error = %v", err)
	}
}

func TestReconstructIndependentPromotionInputsRejectsUntrustedShapes(t *testing.T) {
	valid := ReconstructedFile{Path: "bundle/packs/addy/pack.json", Mode: 0o644, SHA256: digest([]byte("manifest"))}
	material := IndependentPromotionMaterial{
		Base: []ReconstructedFile{valid}, Head: []ReconstructedFile{valid},
		BaseHistory: []ReconstructedFile{}, HeadHistory: []ReconstructedFile{},
		DiffSHA256: digest([]byte("diff")),
	}
	tests := []struct {
		name string
		edit func(*IndependentPromotionMaterial)
	}{
		{"nil files", func(m *IndependentPromotionMaterial) { m.Base = nil }},
		{"unsafe path", func(m *IndependentPromotionMaterial) { m.Base[0].Path = "../pack.json" }},
		{"unsafe mode", func(m *IndependentPromotionMaterial) { m.Base[0].Mode = 0o666 }},
		{"invalid file digest", func(m *IndependentPromotionMaterial) { m.Base[0].SHA256 = "invalid" }},
		{"duplicate path", func(m *IndependentPromotionMaterial) { m.Base = append(m.Base, m.Base[0]) }},
		{"invalid diff digest", func(m *IndependentPromotionMaterial) { m.DiffSHA256 = "invalid" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := material
			candidate.Base = append([]ReconstructedFile(nil), material.Base...)
			test.edit(&candidate)
			if _, err := ReconstructIndependentPromotionInputs(candidate); err == nil {
				t.Fatal("invalid independently reconstructed material was accepted")
			}
		})
	}
}

func TestAddyPromotionAuthorityFoundations(t *testing.T) {
	rows := PromotionRows()
	for _, number := range []int{4, 5, 6} {
		row := rows[number-1]
		if row.Gate != 2 || row.OwningTest != "TestAddyPromotionAuthorityFoundations" {
			t.Fatalf("authority row %d = %#v", number, row)
		}
	}
	assertSyntheticHarnessRows(t, 4, 5, 6)
}

func TestAddyPromotionLifecycleFoundations(t *testing.T) {
	rows := PromotionRows()
	for _, number := range []int{7, 8, 9, 10} {
		row := rows[number-1]
		if row.Gate < 3 || row.Gate > 4 || row.OwningTest != "TestAddyPromotionLifecycleFoundations" {
			t.Fatalf("lifecycle row %d = %#v", number, row)
		}
	}
	assertSyntheticHarnessRows(t, 7, 8, 9, 10)
}

func TestAddyPromotionRealHostFoundations(t *testing.T) {
	rows := PromotionRows()
	for _, number := range []int{11, 12} {
		row := rows[number-1]
		if row.Gate != 5 || row.OwningTest != "TestAddyPromotionRealHostFoundations" {
			t.Fatalf("real-host row %d = %#v", number, row)
		}
	}
	assertSyntheticHarnessRows(t, 11, 12)
}

func TestAddyPromotionEvidenceFoundations(t *testing.T) {
	rows := PromotionRows()
	if len(rows) != 14 {
		t.Fatalf("promotion rows = %d, want 14", len(rows))
	}
	for i, row := range rows {
		wantID := "ADDY-CLAUDE-PROMOTION-ROW-" + twoDigits(i+1)
		if row.ID != wantID || row.Number != i+1 || row.BlockedDiagnostic != wantID+"-BLOCKED" {
			t.Fatalf("row %d = %#v", i+1, row)
		}
	}
	assertSyntheticHarnessRows(t, 13, 14)
	rows[0].ID = "mutated"
	if PromotionRows()[0].ID == "mutated" {
		t.Fatal("PromotionRows returned shared storage")
	}

	context := applicablePromotionContext()
	if err := ValidatePromotionEvidence(validApplicablePromotionEvidence(context), context); err != nil {
		t.Fatalf("valid promotion evidence rejected: %v", err)
	}

	tests := []struct {
		name string
		edit func(*PromotionEvidence)
		want string
	}{
		{name: "missing row", edit: func(e *PromotionEvidence) { e.Rows = e.Rows[:13] }, want: "13 rows"},
		{name: "duplicate row", edit: func(e *PromotionEvidence) { e.Rows[13].ID = e.Rows[12].ID }, want: "duplicate row"},
		{name: "unknown row", edit: func(e *PromotionEvidence) { e.Rows[13].ID = "ADDY-CLAUDE-PROMOTION-ROW-99" }, want: "unknown row"},
		{name: "stale aggregate", edit: func(e *PromotionEvidence) { e.CollectedAt = e.CollectedAt.Add(-2 * time.Hour) }, want: "stale"},
		{name: "stale row", edit: func(e *PromotionEvidence) { e.Rows[0].CollectedAt = e.Rows[0].CollectedAt.Add(-2 * time.Hour) }, want: "stale"},
		{name: "cross commit", edit: func(e *PromotionEvidence) { e.Rows[0].CommitSHA = strings.Repeat("f", 40) }, want: "cross-commit"},
		{name: "cross workflow", edit: func(e *PromotionEvidence) { e.Rows[0].WorkflowDigest = strings.Repeat("e", 64) }, want: "cross-commit"},
		{name: "cross run", edit: func(e *PromotionEvidence) { e.Rows[0].RunID = "other-run" }, want: "cross-commit"},
		{name: "ambiguous result", edit: func(e *PromotionEvidence) { e.Rows[0].Result = "blocked" }, want: "ambiguous result"},
		{name: "ambiguous identity", edit: func(e *PromotionEvidence) { e.Tag = "v1.1.0" }, want: "identity does not match"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence := validApplicablePromotionEvidence(context)
			test.edit(&evidence)
			if err := ValidatePromotionEvidence(evidence, context); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func assertSyntheticHarnessRows(t *testing.T, numbers ...int) {
	t.Helper()
	context := applicablePromotionContext()
	run := func() PromotionHarnessReport {
		report, err := (PromotionHarness{
			Root: t.TempDir(), Context: context, Mode: PromotionHarnessSynthetic,
			Evaluate: SyntheticPromotionRowEvaluator(nil),
		}).Run()
		if err != nil {
			t.Fatal(err)
		}
		return report
	}
	first, second := run(), run()
	firstJSON, err := first.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := second.CanonicalJSON()
	if err != nil || !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("synthetic harness rerun changed: %v", err)
	}
	if !first.Qualified {
		t.Fatal("synthetic promotion harness did not qualify")
	}
	for _, number := range numbers {
		row := first.Rows[number-1]
		if row.ID != PromotionRows()[number-1].ID || row.Result != PromotionPassed || row.EvidenceSHA256 == "" {
			t.Fatalf("promotion row %d harness result = %#v", number, row)
		}
	}
	if _, err := first.BuildAggregate(context, PromotionAggregateCandidate{}); err == nil {
		t.Fatal("synthetic harness report crossed the production aggregate boundary")
	}
}

func TestAddyPromotionNotApplicableIsCanonicalAndFailsClosed(t *testing.T) {
	context := notApplicablePromotionContext()
	evidence := NewNotApplicablePromotionEvidence(context)
	first, err := evidence.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	second, _ := evidence.CanonicalJSON()
	if !bytes.Equal(first, second) {
		t.Fatal("not_applicable encoding changed between reruns")
	}
	decoded, err := DecodePromotionEvidence(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePromotionEvidence(decoded, context); err != nil {
		t.Fatalf("canonical not_applicable rejected: %v", err)
	}
	if _, err := ValidateCanonicalPromotionEvidence(first, context); err != nil {
		t.Fatalf("canonical not_applicable bytes rejected: %v", err)
	}
	compact, _ := json.Marshal(decoded)
	if _, err := ValidateCanonicalPromotionEvidence(compact, context); err == nil || !strings.Contains(err.Error(), "not canonical") {
		t.Fatalf("noncanonical aggregate bytes were accepted: %v", err)
	}
	if !bytes.Contains(first, []byte(`"disposition": "not_applicable"`)) {
		t.Fatalf("canonical output = %s", first)
	}

	var raw map[string]any
	if err := json.Unmarshal(first, &raw); err != nil {
		t.Fatal(err)
	}
	raw["unknown"] = true
	unknown, _ := json.Marshal(raw)
	if _, err := DecodePromotionEvidence(unknown); err == nil {
		t.Fatal("unknown aggregate field was accepted")
	}

	context.PromotionChange = true
	if err := ValidatePromotionEvidence(decoded, context); err == nil || !strings.Contains(err.Error(), "cannot be not_applicable") {
		t.Fatalf("promotion change used not_applicable: %v", err)
	}

	context.PromotionChange = false
	context.FoundationChange = true
	foundation := NewFoundationPromotionEvidence(context)
	foundationJSON, _ := foundation.CanonicalJSON()
	if !bytes.Contains(foundationJSON, []byte(`"disposition": "foundation_validated"`)) {
		t.Fatalf("foundation output = %s", foundationJSON)
	}
	if _, err := ValidateCanonicalPromotionEvidence(foundationJSON, context); err != nil {
		t.Fatalf("foundation evidence rejected: %v", err)
	}
	if err := ValidatePromotionEvidence(decoded, context); err == nil || !strings.Contains(err.Error(), "cannot be not_applicable") {
		t.Fatalf("foundation change used not_applicable: %v", err)
	}
}

func applicablePromotionContext() PromotionValidationContext {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	return PromotionValidationContext{
		PromotionChange:   true,
		Repository:        "yersonargotev/packy",
		PullRequest:       201,
		BaseSHA:           strings.Repeat("a", 40),
		HeadSHA:           strings.Repeat("b", 40),
		EvaluatedMergeSHA: strings.Repeat("c", 40),
		Workflow:          PromotionCheckName,
		WorkflowDigest:    strings.Repeat("d", 64),
		MatrixVersion:     PromotionMatrixVersion,
		RunID:             "12345",
		Now:               now,
		MaxAge:            time.Hour,
		Inputs:            deterministicIndependentPromotionInputs(),
	}
}

func deterministicIndependentPromotionInputs() IndependentPromotionInputs {
	file := func(path string, content string) ReconstructedFile {
		return ReconstructedFile{Path: path, Mode: 0o644, SHA256: digest([]byte(content))}
	}
	inputs, err := ReconstructIndependentPromotionInputs(IndependentPromotionMaterial{
		Base:        []ReconstructedFile{file("bundle/packs/addy/pack.json", "base")},
		Head:        []ReconstructedFile{file("bundle/packs/addy/pack.json", "head")},
		BaseHistory: []ReconstructedFile{file("bundle/history/addy/1.0.0/artifact.json", "base-history")},
		HeadHistory: []ReconstructedFile{file("bundle/history/addy/1.0.0/artifact.json", "base-history"), file("bundle/history/addy/1.1.0/artifact.json", "head-history")},
		DiffSHA256:  digest([]byte("independently reconstructed exact diff")),
	})
	if err != nil {
		panic(err)
	}
	return inputs
}

func materializePromotionVersion(t *testing.T, version ImmutableVersionFixture) string {
	t.Helper()
	root := t.TempDir()
	for _, file := range version.Files {
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if digest([]byte(file.Content)) != file.SHA256 {
			t.Fatalf("%s content digest does not match inventory", file.Path)
		}
		if err := os.WriteFile(target, []byte(file.Content), os.FileMode(file.Mode)); err != nil {
			t.Fatal(err)
		}
	}
	if digest(version.Manifest) != version.ManifestSHA256 {
		t.Fatalf("%s manifest digest does not match bytes", version.Version)
	}
	if err := os.WriteFile(filepath.Join(root, "pack.json"), version.Manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func historySnapshotDigest(files []SyntheticHistoryFile) string {
	rows := make([]string, len(files))
	for i, file := range files {
		rows[i] = fmt.Sprintf("%s\x00%04o\x00%s\n", file.Path, file.Mode, file.SHA256)
	}
	sort.Strings(rows)
	sum := sha256.Sum256([]byte(strings.Join(rows, "")))
	return fmt.Sprintf("%x", sum)
}

func assertPromotionV3Contract(t *testing.T, version ImmutableVersionFixture) {
	t.Helper()
	var manifest struct {
		Resources []struct {
			Kind     string `json:"kind"`
			ID       string `json:"id"`
			Bindings []struct {
				Surface        string                         `json:"surface"`
				AgentAuthority *capabilitypack.AgentAuthority `json:"agent_authority"`
			} `json:"bindings"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(version.Manifest, &manifest); err != nil {
		t.Fatal(err)
	}
	counts, projecting := map[string]int{}, 0
	approvedTools := map[string][]string{
		"code-reviewer":           {"Bash", "Glob", "Grep", "Read"},
		"security-auditor":        {"Bash", "Glob", "Grep", "Read", "WebFetch", "WebSearch"},
		"test-engineer":           {"Bash", "Edit", "Glob", "Grep", "Read", "Write"},
		"web-performance-auditor": {"Bash", "Glob", "Grep", "Read", "WebFetch", "WebSearch"},
	}
	for _, resource := range manifest.Resources {
		counts[resource.Kind]++
		if resource.Kind == "asset" || resource.Kind == "notice" {
			if len(resource.Bindings) != 0 {
				t.Fatalf("%s:%s unexpectedly projects", resource.Kind, resource.ID)
			}
			continue
		}
		projecting++
		if len(resource.Bindings) != 3 || resource.Bindings[0].Surface != "claude" || resource.Bindings[1].Surface != "codex" || resource.Bindings[2].Surface != "opencode" {
			t.Fatalf("%s:%s bindings = %#v", resource.Kind, resource.ID, resource.Bindings)
		}
		if resource.Kind == "agent" && (resource.Bindings[0].AgentAuthority == nil || resource.Bindings[0].AgentAuthority.PermissionMode != "default") {
			t.Fatalf("agent %s authority = %#v", resource.ID, resource.Bindings[0].AgentAuthority)
		}
		if resource.Kind == "agent" {
			seen := map[string]bool{}
			for _, authority := range resource.Bindings[0].AgentAuthority.Authorities {
				for _, tool := range authority.ClaudeTools {
					seen[tool] = true
				}
			}
			var got []string
			for tool := range seen {
				got = append(got, tool)
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, approvedTools[resource.ID]) {
				t.Fatalf("agent %s effective Claude tools = %v, want %v", resource.ID, got, approvedTools[resource.ID])
			}
		}
	}
	if !reflect.DeepEqual(counts, map[string]int{"skill": 24, "agent": 4, "command": 8, "asset": 7, "notice": 1}) || projecting != 36 {
		t.Fatalf("v3 counts = %v, projecting=%d", counts, projecting)
	}
}

func notApplicablePromotionContext() PromotionValidationContext {
	context := applicablePromotionContext()
	context.PromotionChange = false
	context.EvaluatedMergeSHA = ""
	return context
}

func validApplicablePromotionEvidence(context PromotionValidationContext) PromotionEvidence {
	rows := make([]PromotionRowEvidence, 0, len(promotionRows))
	for i, definition := range promotionRows {
		rows = append(rows, PromotionRowEvidence{
			ID:             definition.ID,
			Result:         PromotionPassed,
			EvidenceSHA256: strings.Repeat(string("0123456789abcdef"[i%16]), 64),
			CommitSHA:      contextCommit(context),
			WorkflowDigest: context.WorkflowDigest,
			RunID:          context.RunID,
			CollectedAt:    context.Now.Add(-time.Minute),
		})
	}
	return PromotionEvidence{
		Schema:            PromotionEvidenceSchema,
		Disposition:       PromotionApplicable,
		Repository:        context.Repository,
		PullRequest:       context.PullRequest,
		BaseSHA:           context.BaseSHA,
		HeadSHA:           context.HeadSHA,
		EvaluatedMergeSHA: context.EvaluatedMergeSHA,
		Workflow:          context.Workflow,
		WorkflowDigest:    context.WorkflowDigest,
		MatrixVersion:     context.MatrixVersion,
		RunID:             context.RunID,
		CollectedAt:       context.Now.Add(-time.Minute),
		Rows:              rows,
		Proof:             PromotionProof{IndependentPromotionInputs: context.Inputs},
		PackageCandidate:  strings.Repeat("1", 64),
		ClaudeIdentities:  []string{"claude-code@2.1.203"},
		AtomicitySHA256:   strings.Repeat("2", 64),
	}
}

func twoDigits(value int) string {
	if value < 10 {
		return "0" + string(rune('0'+value))
	}
	return string(rune('0'+value/10)) + string(rune('0'+value%10))
}
