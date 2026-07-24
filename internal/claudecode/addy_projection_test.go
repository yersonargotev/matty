package claudecode

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

func TestAddyCompositeSkillPreservesTreeAndReferences(t *testing.T) {
	root, pack := addyCompositeFixture(t)
	resource := pack.Resources[0]
	got, err := addyCompositeSkill(pack, resource, resource.Bindings[0], root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 10 {
		t.Fatalf("files=%d, want source tree plus seven references", len(got.Files))
	}
	byPath := map[string]compositeFile{}
	for _, file := range got.Files {
		byPath[file.Path] = file
	}
	if !bytes.Equal(byPath["scripts/inert.sh"].Content, []byte("#!/bin/sh\nexit 97\n")) || byPath["scripts/inert.sh"].Mode != 0o755 {
		t.Fatalf("executable source not preserved: %+v", byPath["scripts/inert.sh"])
	}
	for _, id := range addyReferenceIDs {
		path := "references/" + id + ".md"
		if string(byPath[path].Content) != id+"\n" || byPath[path].Mode != 0o644 {
			t.Fatalf("reference %s=%+v", id, byPath[path])
		}
	}
	again, err := addyCompositeSkill(pack, resource, resource.Bindings[0], root)
	if err != nil || got.TreeFingerprint != again.TreeFingerprint || got.DefinitionFingerprint != again.DefinitionFingerprint {
		t.Fatalf("nondeterministic result: first=%+v second=%+v err=%v", got, again, err)
	}
	payload, err := canonicalCompositeSkillPayload(got)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeCompositeSkillPayload(payload)
	if err != nil || decoded.TreeFingerprint != got.TreeFingerprint {
		t.Fatalf("payload round trip: %+v, %v", decoded, err)
	}
}

func TestAddyCommandAliasDependenciesAndLiteralArguments(t *testing.T) {
	root, pack := addyCompositeFixture(t)
	command := capabilitypack.Resource{
		Kind: "command", ID: "review", Source: "commands/review.toml",
		Requires: []string{"skill:using-agent-skills", "agent:code-reviewer", "asset:definition-of-done"},
		Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "skill", Name: "addy-review"}},
	}
	pack.Resources = append(pack.Resources,
		capabilitypack.Resource{Kind: "skill", ID: "using-agent-skills", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "skill", Name: "addy-using-agent-skills"}}},
		capabilitypack.Resource{Kind: "agent", ID: "code-reviewer", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "agent", Name: "addy-code-reviewer"}}},
		command,
	)
	source := []byte("description = \"Review carefully\"\nprompt = '''Keep $ARGUMENTS unchanged.\\nNext line.'''\n")
	writeAddyFile(t, root, command.Source, source, 0o644)
	got, err := addyCompositeSkill(pack, command, command.Bindings[0], root)
	if err != nil {
		t.Fatal(err)
	}
	content := string(got.Files[0].Content)
	for _, want := range []string{"agent:addy-code-reviewer", "skill:addy-using-agent-skills", "reference:definition-of-done.md", "`$ARGUMENTS`", `Keep $ARGUMENTS unchanged.\nNext line.`} {
		if !strings.Contains(content, want) {
			t.Fatalf("SKILL.md missing %q:\n%s", want, content)
		}
	}
	var original []byte
	for _, file := range got.Files {
		if file.Path == "source/review.toml" {
			original = file.Content
		}
	}
	if !bytes.Equal(original, source) {
		t.Fatalf("original command bytes changed: %q", original)
	}
}

func TestAddyCommandStrictTOMLAndDependencies(t *testing.T) {
	for name, input := range map[string][]byte{
		"invalid utf8":   {0xff},
		"unknown":        []byte("description=\"x\"\nprompt=\"y\"\nother=\"z\"\n"),
		"duplicate":      []byte("description=\"x\"\ndescription=\"z\"\nprompt=\"y\"\n"),
		"missing":        []byte("description=\"x\"\n"),
		"non-string":     []byte("description=\"x\"\nprompt=1\n"),
		"invalid syntax": []byte("description=\"x\"\nprompt = [\"y\"]\n"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeAddyCommand(input); err == nil {
				t.Fatalf("accepted %q", input)
			}
		})
	}
	root, pack := addyCompositeFixture(t)
	command := capabilitypack.Resource{Kind: "command", ID: "x", Source: "commands/x.toml", Requires: []string{"agent:absent"}, Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "skill", Name: "x"}}}
	writeAddyFile(t, root, command.Source, []byte("description=\"x\"\nprompt=\"y\"\n"), 0o644)
	if _, err := addyCompositeSkill(pack, command, command.Bindings[0], root); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing dependency err=%v", err)
	}
}

func TestAddyCommandStrictDecoderAcceptsExactUpstreamCommandBytes(t *testing.T) {
	archive, err := os.Open(filepath.Join("..", "addyacceptance", "testdata", "addy-0.6.4.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	compressed, err := gzip.NewReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	count := 0
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.ToSlash(header.Name)
		if strings.Contains(path, "/.gemini/") || !strings.Contains(path, "/commands/") || filepath.Ext(path) != ".toml" {
			continue
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		command, err := decodeAddyCommand(content)
		if err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		if command.Description == "" || command.Prompt == "" {
			t.Fatalf("empty exact command fields for %s", path)
		}
		count++
	}
	if count != 8 {
		t.Fatalf("decoded %d exact commands, want 8", count)
	}
}

func TestAddyCompositePayloadRejectsTamperAndNoncanonicalData(t *testing.T) {
	root, pack := addyCompositeFixture(t)
	got, err := addyCompositeSkill(pack, pack.Resources[0], pack.Resources[0].Bindings[0], root)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := canonicalCompositeSkillPayload(got)
	tampered := bytes.Replace(payload, []byte("c2tpbGwgYnl0ZXMK"), []byte("b3RoZXIgYnl0ZXMK"), 1)
	if _, err := decodeCompositeSkillPayload(tampered); err == nil {
		t.Fatal("accepted payload with stale tree fingerprint")
	}
	noncanonical := append([]byte(" "), payload...)
	if _, err := decodeCompositeSkillPayload(noncanonical); err == nil {
		t.Fatal("accepted noncanonical JSON")
	}
	traversal := bytes.Replace(payload, []byte(`"path":"SKILL.md"`), []byte(`"path":"../SKILL.md"`), 1)
	if _, err := decodeCompositeSkillPayload(traversal); err == nil {
		t.Fatal("accepted traversal path")
	}
	badMode := bytes.Replace(payload, []byte(`"mode":420`), []byte(`"mode":384`), 1)
	if _, err := decodeCompositeSkillPayload(badMode); err == nil {
		t.Fatal("accepted bad mode")
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	reference := filepath.Join(root, "references", "accessibility-checklist.md")
	if err := os.Remove(reference); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, reference); err != nil {
		t.Fatal(err)
	}
	if _, err := addyCompositeSkill(pack, pack.Resources[0], pack.Resources[0].Bindings[0], root); err == nil || !strings.Contains(err.Error(), "escapes bundle root") {
		t.Fatalf("escaping source symlink was accepted: %v", err)
	}
}

func TestAddyCompositeSurfaceStagesReplacesPreservesAndRemovesExactOwnership(t *testing.T) {
	bundle, pack := addyCompositeFixture(t)
	home := t.TempDir()
	sentinel := filepath.Join(home, "source-executed")
	writeAddyFile(t, bundle, "skills/example/scripts/inert.sh", []byte("#!/bin/sh\n: > "+sentinel+"\n"), 0o755)
	layout := NewCanonicalLayout(home)
	empty := StaticOwnershipSnapshot(NewOwnershipSnapshot())
	adapter := NewSurfaceAdapter(bundle, layout, filepath.Join(home, "state-empty"), "claude", &recordingRunner{result: Result{Stdout: "2.1.203"}}, empty)
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil || len(inspection.Projections) != 1 {
		t.Fatalf("inspection=%+v err=%v", inspection, err)
	}
	projection := inspection.Projections[0]
	if projection.Action.Kind != ActionSkillTree || projection.Exists || projection.ObservedFingerprint != "missing" {
		t.Fatalf("initial projection=%+v", projection)
	}
	if applyErr := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{projection.Action}); applyErr != nil {
		t.Fatal(applyErr)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("source content was executed: %v", err)
	}
	target := filepath.Join(layout.SkillsDir, "example")
	if got, err := localprojection.FingerprintExactTree(target); err != nil || got != projection.DesiredFingerprint {
		t.Fatalf("installed tree=%q want=%q err=%v", got, projection.DesiredFingerprint, err)
	}
	installed, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil || installed.Projections[0].ObservedFingerprint != installed.Projections[0].DesiredFingerprint {
		t.Fatalf("identical inspection=%+v err=%v", installed, err)
	}

	record := compositeOwnershipRecord(t, installed.Projections[0], "addy")
	writeAddyFile(t, bundle, "skills/example/notes.md", []byte("replacement"), 0o644)
	owned := NewSurfaceAdapter(bundle, layout, filepath.Join(home, "state-owned"), "claude", &recordingRunner{result: Result{Stdout: "2.1.203"}}, StaticOwnershipSnapshot(NewOwnershipSnapshot(record)))
	replacement, err := owned.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil || replacement.Projections[0].DesiredFingerprint == record.Fingerprint {
		t.Fatalf("replacement inspection=%+v err=%v", replacement, err)
	}
	if applyErr := owned.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{replacement.Projections[0].Action}); applyErr != nil {
		t.Fatalf("exact-owned replacement failed: %v", applyErr)
	}
	replacementRecord := compositeOwnershipRecord(t, replacement.Projections[0], "addy")

	foreign := NewSurfaceAdapter(bundle, layout, filepath.Join(home, "state-foreign"), "claude", &recordingRunner{}, empty)
	if applyErr := foreign.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{replacement.Projections[0].Action}); applyErr == nil || !strings.Contains(applyErr.Error(), "foreign") {
		t.Fatalf("same-byte foreign target was accepted: %v", applyErr)
	}
	tamperedPath := filepath.Join(target, "notes.md")
	if err := os.WriteFile(tamperedPath, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	tamperedBefore, _ := os.ReadFile(tamperedPath)
	tampered := NewSurfaceAdapter(bundle, layout, filepath.Join(home, "state-tampered"), "claude", &recordingRunner{}, StaticOwnershipSnapshot(NewOwnershipSnapshot(replacementRecord)))
	if applyErr := tampered.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{replacement.Projections[0].Action}); applyErr == nil || !strings.Contains(applyErr.Error(), "changed") {
		t.Fatalf("modified owned tree was accepted: %v", applyErr)
	}
	if after, _ := os.ReadFile(tamperedPath); !bytes.Equal(after, tamperedBefore) {
		t.Fatal("blocked modified tree was mutated")
	}

	if err := os.RemoveAll(target); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("unexpected file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if applyErr := tampered.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{replacement.Projections[0].Action}); applyErr == nil || !strings.Contains(applyErr.Error(), "unexpected path type") {
		t.Fatalf("unexpected path type was accepted: %v", applyErr)
	}
	if data, _ := os.ReadFile(target); string(data) != "unexpected file" {
		t.Fatal("unexpected target was mutated")
	}

	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if applyErr := tampered.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{replacement.Projections[0].Action}); applyErr != nil {
		t.Fatalf("owned missing target did not recover through fresh action: %v", applyErr)
	}
	ambiguous := NewSurfaceAdapter(bundle, layout, filepath.Join(home, "state-ambiguous"), "claude", &recordingRunner{}, StaticOwnershipSnapshot(NewOwnershipSnapshot(replacementRecord, replacementRecord)))
	if applyErr := ambiguous.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{replacement.Projections[0].Action}); applyErr == nil || !strings.Contains(applyErr.Error(), "ambiguous") {
		t.Fatalf("ambiguous ownership was accepted: %v", applyErr)
	}
	staleSnapshot := NewOwnershipSnapshot(replacementRecord)
	staleSnapshot.Revision = "stale"
	stale := NewSurfaceAdapter(bundle, layout, filepath.Join(home, "state-stale"), "claude", &recordingRunner{}, StaticOwnershipSnapshot(staleSnapshot))
	if applyErr := stale.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{replacement.Projections[0].Action}); applyErr == nil || !strings.Contains(applyErr.Error(), "stale") {
		t.Fatalf("stale ownership was accepted: %v", applyErr)
	}

	removalInspection, err := owned.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Prior: pack, Desired: capabilitypack.Pack{}})
	if err != nil || len(removalInspection.Projections) != 1 || removalInspection.Projections[0].Action.Mode != capabilitypack.ProjectionDeleteTarget {
		t.Fatalf("removal inspection=%+v err=%v", removalInspection, err)
	}
	sharedRecord := replacementRecord
	sharedRecord.Contributors = []string{"addy", "other"}
	sharedRecord.DeletionAuthorized = false
	shared := NewSurfaceAdapter(bundle, layout, filepath.Join(home, "state-shared"), "claude", &recordingRunner{}, StaticOwnershipSnapshot(NewOwnershipSnapshot(sharedRecord)))
	if applyErr := shared.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{removalInspection.Projections[0].Action}); applyErr == nil || !strings.Contains(applyErr.Error(), "not authorized") {
		t.Fatalf("shared composite deletion was accepted: %v", applyErr)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("shared target was removed: %v", err)
	}
	last := NewSurfaceAdapter(bundle, layout, filepath.Join(home, "state-last"), "claude", &recordingRunner{}, StaticOwnershipSnapshot(NewOwnershipSnapshot(replacementRecord)))
	if applyErr := last.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{removalInspection.Projections[0].Action}); applyErr != nil {
		t.Fatalf("last contributor removal failed: %v", applyErr)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("exact-owned target remains: %v", err)
	}
}

func TestAddyCompositeOwnershipProviderReconstructsAliasAndRecovery(t *testing.T) {
	bundle, pack := addyCompositeFixture(t)
	home := t.TempDir()
	layout := NewCanonicalLayout(home)
	alias := capabilitypack.SurfaceAlias{Kind: "skill", ID: "example", Name: "addy-example"}
	aliased := resourceWithAliases(pack.Resources[0], []capabilitypack.SurfaceAlias{alias})
	composite, err := addyCompositeSkill(pack, aliased, aliased.Bindings[0], bundle)
	if err != nil {
		t.Fatal(err)
	}
	provenance, err := canonicalCompositeOwnership(composite.Ownership)
	if err != nil {
		t.Fatal(err)
	}
	state := capabilitypack.ActivationState{
		Intent: capabilitypack.ActivationIntent{PackID: "addy", Version: "1.1.0", Surface: capabilitypack.SurfaceClaude, Active: true, Aliases: []capabilitypack.SurfaceAlias{alias}},
		Ownership: []capabilitypack.ProjectionOwnership{{
			ID: "skill:addy-example", Contributors: []string{"addy"}, Fingerprint: composite.TreeFingerprint, AdapterProvenance: provenance,
		}},
	}
	provider := NewCapabilityPackOwnershipProvider(ownershipStore{state}, map[string]capabilitypack.Pack{"addy": pack}, layout, bundle)
	snapshot, err := provider.ObserveOwnership(context.Background())
	if err != nil || len(snapshot.Records) != 1 {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	record := snapshot.Records[0]
	if record.Kind != string(ActionSkillTree) || record.Target != filepath.Join(layout.SkillsDir, "addy-example") || record.Composite != composite.Ownership || record.Fingerprint != composite.TreeFingerprint {
		t.Fatalf("record=%+v", record)
	}

	recovery := state
	recovery.Ownership = nil
	recovery.Intent.Active = false
	recovery.Journal = &capabilitypack.ApplyingJournal{PackID: "addy", Surface: capabilitypack.SurfaceClaude, Actions: []string{"skill:addy-example"}}
	recoveryProvider := NewCapabilityPackOwnershipProvider(ownershipStore{recovery}, map[string]capabilitypack.Pack{"addy": pack}, layout, bundle)
	recovered, err := recoveryProvider.ObserveOwnership(context.Background())
	if err != nil || len(recovered.Records) != 1 || recovered.Records[0].Composite != composite.Ownership {
		t.Fatalf("recovery snapshot=%+v err=%v", recovered, err)
	}

	stale := state
	stale.Ownership = append([]capabilitypack.ProjectionOwnership(nil), state.Ownership...)
	stale.Ownership[0].AdapterProvenance = "{}"
	staleProvider := NewCapabilityPackOwnershipProvider(ownershipStore{stale}, map[string]capabilitypack.Pack{"addy": pack}, layout, bundle)
	if _, err := staleProvider.ObserveOwnership(context.Background()); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("stale provenance was accepted: %v", err)
	}
}

func compositeOwnershipRecord(t *testing.T, projection capabilitypack.ObservedProjection, contributor string) OwnershipRecord {
	t.Helper()
	ownership, err := decodeCompositeOwnership(projection.Action.AdapterProvenance)
	if err != nil {
		t.Fatal(err)
	}
	return OwnershipRecord{
		StateOwner: "capabilitypack", ContributorID: contributor, ID: projection.ID, Kind: string(ActionSkillTree),
		Target: projection.Action.Target, Fingerprint: projection.DesiredFingerprint, Contributors: []string{contributor},
		DeletionAuthorized: true, Composite: ownership,
	}
}

func addyCompositeFixture(t *testing.T) (string, capabilitypack.Pack) {
	t.Helper()
	root := t.TempDir()
	skill := capabilitypack.Resource{Kind: "skill", ID: "example", Source: "skills/example", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "skill", Name: "example"}}}
	resources := []capabilitypack.Resource{skill}
	writeAddyFile(t, root, "skills/example/SKILL.md", []byte("skill bytes\n"), 0o644)
	writeAddyFile(t, root, "skills/example/notes.md", []byte{0, 1, 2}, 0o644)
	writeAddyFile(t, root, "skills/example/scripts/inert.sh", []byte("#!/bin/sh\nexit 97\n"), 0o755)
	for _, id := range addyReferenceIDs {
		resource := capabilitypack.Resource{Kind: "asset", ID: id, Source: "references/" + id + ".md"}
		resources = append(resources, resource)
		writeAddyFile(t, root, resource.Source, []byte(id+"\n"), 0o644)
	}
	return root, capabilitypack.Pack{ID: "addy", Version: "1.1.0", Resources: resources}
}

func writeAddyFile(t *testing.T, root, relative string, content []byte, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatal(err)
	}
}
