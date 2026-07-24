package capabilitypack

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/addyacceptance"
)

func TestAddyPromotionHistoryIsExactAndRoutesOnlyExistingSurfaces(t *testing.T) {
	catalog, history := addyLifecycleCatalog(t, false)

	detail, err := catalog.ShowDetail("addy")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Pack.Version != "1.1.0" || !detail.Current || detail.Withdrawn ||
		!reflect.DeepEqual(detail.HistoricalVersions, []string{"1.0.0", "1.1.0"}) || len(detail.UpdateRoutes) != 1 {
		t.Fatalf("Addy detail = %#v", detail)
	}
	for _, version := range history.Versions {
		pack, err := catalog.resolveIntentPack("addy", version.Version)
		if err != nil {
			t.Fatalf("resolve Addy %s: %v", version.Version, err)
		}
		if pack.Version != version.Version || pack.manifestVersion != version.SchemaVersion {
			t.Fatalf("resolved Addy %s = %#v", version.Version, pack)
		}
	}
	for _, surface := range []Surface{SurfaceCodex, SurfaceOpenCode} {
		if err := catalog.validateUpdateRoute("addy", "1.0.0", "1.1.0", manifestSchemaV3, surface); err != nil {
			t.Fatalf("existing %s route: %v", surface, err)
		}
	}
	for _, test := range []struct {
		from, to string
		surface  Surface
		message  string
	}{
		{"1.0.0", "1.1.0", SurfaceClaude, "does not add claude intent"},
		{"1.1.0", "1.0.0", SurfaceCodex, "no supported update route"},
		{"0.9.0", "1.1.0", SurfaceCodex, "no supported update route"},
	} {
		if err := catalog.validateUpdateRoute("addy", test.from, test.to, manifestSchemaV3, test.surface); err == nil || !strings.Contains(err.Error(), test.message) {
			t.Fatalf("route %s -> %s on %s error = %v", test.from, test.to, test.surface, err)
		}
	}
}

func TestAddyPinnedHistoryStatusAndUpdateNeverCreateClaudeIntent(t *testing.T) {
	catalog, _ := addyLifecycleCatalog(t, false)
	intent := ActivationIntent{PackID: "addy", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 4}
	store := &fakeActivationStore{state: ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}}
	adapter := &fakeSurfaceAdapter{inspect: func(transition SurfaceTransition) SurfaceInspection {
		return completeAddyObservation(transition.Desired, SurfaceCodex, "missing")
	}}
	facade := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter, SurfaceClaude: adapter}))

	report, err := facade.Status(context.Background(), StatusRequest{PackID: "addy", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Entries) != 1 || report.Entries[0].Intent.Version != "1.0.0" || !report.Entries[0].UpdateAvailable || report.Entries[0].Pack.Version != "1.1.0" {
		t.Fatalf("pinned Addy status = %#v", report)
	}
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "addy", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.OldVersion() != "1.0.0" || plan.Pack().Version != "1.1.0" || plan.Surface() != SurfaceCodex {
		t.Fatalf("Addy update = %#v", plan.JSONReport(true))
	}
	if _, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "addy", Surface: SurfaceClaude}); err == nil || !strings.Contains(err.Error(), "not active") {
		t.Fatalf("implicit Claude update error = %v", err)
	}
	if _, err := facade.Preview(context.Background(), ActivationRequest{PackID: "addy", Surface: SurfaceCodex}); err == nil || !strings.Contains(err.Error(), "explicit pack update") {
		t.Fatalf("implicit activation update error = %v", err)
	}
	if len(store.saves) != 0 || !reflect.DeepEqual(store.state.Intent, intent) {
		t.Fatalf("preview mutated pinned intent: %#v", store.state)
	}
}

func TestWithdrawnAddyIsHiddenAndRejectsFreshActivationAndUpdateWithoutEffects(t *testing.T) {
	catalog, _ := addyLifecycleCatalog(t, true)
	if details, err := catalog.ListDetails(); err != nil || len(details) != 0 {
		t.Fatalf("withdrawn ListDetails = %#v err=%v", details, err)
	}
	if current, err := catalog.ListCurrent(); err != nil || len(current) != 0 {
		t.Fatalf("withdrawn ListCurrent = %#v err=%v", current, err)
	}
	if detail, err := catalog.ShowDetail("addy"); err != nil || !detail.Withdrawn || detail.Current {
		t.Fatalf("withdrawn ShowDetail = %#v err=%v", detail, err)
	}

	adapter := &fakeSurfaceAdapter{}
	store := &fakeActivationStore{}
	facade := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	if _, err := facade.Preview(context.Background(), ActivationRequest{PackID: "addy", Surface: SurfaceCodex}); err == nil || !strings.Contains(err.Error(), "withdrawn") {
		t.Fatalf("fresh activation error = %v", err)
	}
	store.state = ActivationState{Intent: ActivationIntent{PackID: "addy", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2}}
	if _, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "addy", Surface: SurfaceCodex}); err == nil || !strings.Contains(err.Error(), "withdrawn") {
		t.Fatalf("withdrawn update error = %v", err)
	}
	if adapter.inspectCalls != 0 || len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("rejected lifecycle crossed effect boundary: inspect=%d actions=%d saves=%d", adapter.inspectCalls, len(adapter.actions), len(store.saves))
	}
}

func TestWithdrawnAddyActiveHistoricalIntentRemainsOperable(t *testing.T) {
	catalog, _ := addyLifecycleCatalog(t, true)
	catalog.deferSourceValidation = true
	intent := ActivationIntent{PackID: "addy", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 3}
	store := &fakeActivationStore{state: ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}}
	adapter := &fakeSurfaceAdapter{inspect: func(transition SurfaceTransition) SurfaceInspection {
		return completeAddyObservation(transition.Desired, SurfaceCodex, "missing")
	}}
	facade := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	current, err := catalog.catalogMetadata("addy")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(catalog.bundleRoot, filepath.FromSlash(current.Resources[0].Source))); err != nil {
		t.Fatal(err)
	}

	if report, err := facade.Status(context.Background(), StatusRequest{PackID: "addy", Surface: SurfaceCodex}); err != nil || len(report.Entries) != 1 || report.Entries[0].Intent.Version != "1.0.0" {
		t.Fatalf("withdrawn pinned status = %#v err=%v", report, err)
	}
	if _, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: "addy", Surface: SurfaceCodex}); err != nil {
		t.Fatalf("withdrawn reconcile: %v", err)
	}
	if _, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "addy", Surface: SurfaceCodex}); err != nil {
		t.Fatalf("withdrawn deactivate: %v", err)
	}
	if _, err := facade.Preview(context.Background(), ActivationRequest{PackID: "addy", Surface: SurfaceCodex}); err == nil || !strings.Contains(err.Error(), "explicit pack update") {
		t.Fatalf("withdrawn historical activation should require the unavailable explicit update: %v", err)
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 || !store.state.Intent.Active {
		t.Fatalf("withdrawn previews mutated state: actions=%d saves=%d state=%#v", len(adapter.actions), len(store.saves), store.state)
	}
}

func TestWithdrawnAddyInterruptedUpdateRemainsRecoverable(t *testing.T) {
	catalog, _ := addyLifecycleCatalog(t, true)
	intent := ActivationIntent{PackID: "addy", Surface: SurfaceCodex, Version: "1.1.0", Active: true, Revision: 6}
	journal := ApplyingJournal{
		PlanID: "interrupted-update", PlanDigest: "old-digest", Operation: OperationUpdate,
		Surface: SurfaceCodex, PackID: "addy", Outcome: AttemptRecoveryRequired,
		Actions: []string{"instruction:addy-routing"}, FailedAction: "instruction:addy-routing",
	}
	store := &fakeActivationStore{state: ActivationState{
		Intent: intent, Intents: []ActivationIntent{intent}, Journal: &journal,
	}}
	adapter := &fakeSurfaceAdapter{inspect: func(transition SurfaceTransition) SurfaceInspection {
		return completeAddyObservation(transition.Desired, SurfaceCodex, "missing")
	}}
	facade := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))

	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "addy", Surface: SurfaceCodex})
	if err != nil {
		t.Fatalf("withdrawn interrupted update recovery: %v", err)
	}
	if !plan.Recovery() || plan.HistoricalAttempt() == nil || plan.HistoricalAttempt().PlanID != journal.PlanID {
		t.Fatalf("withdrawn recovery plan = %#v", plan.JSONReport(true))
	}
	if len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatalf("recovery preview crossed effect boundary: actions=%d saves=%d", len(adapter.actions), len(store.saves))
	}
}

func TestAddyCurrentUpdateIsAnExactSurfaceLocalNoOp(t *testing.T) {
	catalog, _ := addyLifecycleCatalog(t, false)
	pack, err := catalog.Show("addy")
	if err != nil {
		t.Fatal(err)
	}
	intent := ActivationIntent{PackID: "addy", Surface: SurfaceCodex, Version: "1.1.0", Active: true, Revision: 8, Aliases: []SurfaceAlias{}}
	observation := completeAddyObservation(pack, SurfaceCodex, "desired")
	state := ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}
	for _, projection := range observation.Projections {
		state.Ownership = append(state.Ownership, ProjectionOwnership{ID: projection.ID, Contributors: []string{"addy"}, Fingerprint: projection.DesiredFingerprint})
	}
	store := &fakeActivationStore{state: state}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{observation}}
	facade := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))

	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "addy", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.NoOp() || len(plan.Phases()) != 0 || plan.OldVersion() != "1.1.0" || len(plan.Aliases()) != 0 {
		t.Fatalf("current Addy update = %#v", plan.JSONReport(true))
	}
	if len(store.saves) != 0 || len(adapter.actions) != 0 || !reflect.DeepEqual(store.state.Intent, intent) {
		t.Fatalf("no-op update changed state: actions=%d saves=%d intent=%#v", len(adapter.actions), len(store.saves), store.state.Intent)
	}
}

func TestAddyHistoricalArtifactRejectsOneFactTamperAndMissingHistory(t *testing.T) {
	catalog, _ := addyLifecycleCatalog(t, false)
	root := filepath.Join(catalog.bundleRoot, "history", "addy", "1.0.0")
	artifact := readHistoricalArtifact(t, root)
	path := filepath.Join(root, filepath.FromSlash(artifact.Resources[0].Files[0].Path))
	original := mustRead(t, path)
	mustWrite(t, path, append(append([]byte(nil), original...), '\n'), 0o644)
	if _, err := catalog.resolveIntentPack("addy", "1.0.0"); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("one-fact tamper error = %v", err)
	}
	mustWrite(t, path, original, 0o644)
	if err := os.Remove(filepath.Join(root, "artifact.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.resolveIntentPack("addy", "1.0.0"); err == nil || !strings.Contains(err.Error(), "read artifact.json") {
		t.Fatalf("missing history error = %v", err)
	}
}

func addyLifecycleCatalog(t *testing.T, withdrawn bool) (Catalog, addyacceptance.PromotionHistoryFixture) {
	t.Helper()
	history := addyacceptance.CanonicalPromotionHistory()
	bundle := filepath.Join(t.TempDir(), "bundle")
	for _, version := range history.Versions {
		root := filepath.Join(bundle, "history", "addy", version.Version)
		for _, file := range version.Files {
			mustWrite(t, filepath.Join(root, filepath.FromSlash(file.Path)), []byte(file.Content), os.FileMode(file.Mode))
		}
		mustWrite(t, filepath.Join(root, "pack.json"), version.Manifest, 0o644)
		trustHistoricalFixture(t, root, "addy@"+version.Version)
		if version.Version == "1.1.0" {
			for _, file := range version.Files {
				mustWrite(t, filepath.Join(bundle, filepath.FromSlash(file.Path)), []byte(file.Content), os.FileMode(file.Mode))
			}
			mustWrite(t, filepath.Join(bundle, "packs", "addy", "pack.json"), version.Manifest, 0o644)
		}
	}
	entry := catalogEntry{
		ID: "addy", Description: "Addy acceptance cohort", Surfaces: []Surface{SurfaceClaude, SurfaceCodex, SurfaceOpenCode}, Withdrawn: withdrawn,
		HistoricalVersions: []string{"1.0.0", "1.1.0"},
		UpdateRoutes:       []UpdateRoute{{FromVersion: "1.0.0", ToVersion: "1.1.0", ExistingSurfaces: []Surface{SurfaceCodex, SurfaceOpenCode}}},
	}
	catalog, err := discoverCatalog(bundle, []catalogEntry{entry})
	if err != nil {
		t.Fatal(err)
	}
	catalog.enforceUpdateRoutes = true
	return catalog, history
}
