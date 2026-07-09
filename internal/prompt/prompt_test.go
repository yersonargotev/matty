package prompt

import (
	"os"
	"strings"
	"testing"
)

func TestSectionInsertUpdateRemove(t *testing.T) {
	existing := "# User notes\n\nKeep this.\n"
	inserted := upsertSection(existing, codexMattySectionID, CodexContent())
	for _, want := range []string{
		"# User notes\n\nKeep this.",
		"<!-- matty:skills-router -->",
		"~/.agents/skills",
		"ask-matt",
		"Engram memory tools",
		"host delegation rules",
		"<!-- /matty:skills-router -->",
	} {
		if !strings.Contains(inserted, want) {
			t.Fatalf("inserted prompt missing %q:\n%s", want, inserted)
		}
	}

	updated := upsertSection(inserted, codexMattySectionID, "replacement\n")
	if strings.Count(updated, "<!-- matty:skills-router -->") != 1 {
		t.Fatalf("updated prompt should have one Matty marker:\n%s", updated)
	}
	if !strings.Contains(updated, "replacement") || strings.Contains(updated, "ask-matt") {
		t.Fatalf("Matty block was not replaced surgically:\n%s", updated)
	}
	if !strings.Contains(updated, "# User notes\n\nKeep this.") {
		t.Fatalf("user content was not preserved:\n%s", updated)
	}

	removed := removeSection(updated, codexMattySectionID)
	if strings.Contains(removed, "matty:skills-router") || strings.Contains(removed, "replacement") {
		t.Fatalf("Matty block was not removed:\n%s", removed)
	}
	if removed != existing {
		t.Fatalf("remove should preserve original content exactly:\ngot:  %q\nwant: %q", removed, existing)
	}
}

func TestWriteCodexAddsAndRemovesRulesSection(t *testing.T) {
	path := t.TempDir() + "/AGENTS.md"
	original := "# User notes\n\nKeep this.\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("write original prompt: %v", err)
	}

	if _, err := WriteCodex(path); err != nil {
		t.Fatalf("WriteCodex failed: %v", err)
	}
	updatedBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read updated prompt: %v", err)
	}
	updated := string(updatedBytes)
	for _, want := range []string{
		"<!-- matty:skills-router -->",
		RulesSectionContent(),
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("updated prompt missing %q:\n%s", want, updated)
		}
	}

	if err := RemoveCodex(path); err != nil {
		t.Fatalf("RemoveCodex failed: %v", err)
	}
	removedBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read removed prompt: %v", err)
	}
	if removed := string(removedBytes); removed != original {
		t.Fatalf("RemoveCodex should remove all Matty sections:\ngot:  %q\nwant: %q", removed, original)
	}
}

func TestSectionPreservesGentleAIAndEngramBlocks(t *testing.T) {
	existing := strings.Join([]string{
		"# User intro",
		"",
		"<!-- gentle-ai:persona -->",
		"Gentle persona.",
		"<!-- /gentle-ai:persona -->",
		"",
		"<!-- gentle-ai:engram-protocol -->",
		"Engram protocol.",
		"<!-- /gentle-ai:engram-protocol -->",
		"",
		"User footer.",
		"",
	}, "\n")

	updated := upsertSection(existing, codexMattySectionID, CodexContent())
	withoutMatty := removeSection(updated, codexMattySectionID)
	if withoutMatty != existing {
		t.Fatalf("non-Matty content changed after insert/remove:\ngot:\n%s\nwant:\n%s", withoutMatty, existing)
	}
	for _, want := range []string{"<!-- gentle-ai:persona -->", "Gentle persona.", "<!-- gentle-ai:engram-protocol -->", "Engram protocol."} {
		if !strings.Contains(updated, want) {
			t.Fatalf("updated prompt lost %q:\n%s", want, updated)
		}
	}
}

func TestSectionUpdateAndRemoveAllMattyBlocks(t *testing.T) {
	existing := "before\n" +
		"<!-- matty:skills-router -->\none\n<!-- /matty:skills-router -->" +
		"\nbetween\n" +
		"<!-- matty:skills-router -->\ntwo\n<!-- /matty:skills-router -->" +
		"\nafter"

	updated := upsertSection(existing, codexMattySectionID, "replacement\n")
	if got := strings.Count(updated, "<!-- matty:skills-router -->"); got != 1 {
		t.Fatalf("updated prompt should collapse to one Matty block, got %d:\n%s", got, updated)
	}
	if strings.Contains(updated, "one") || strings.Contains(updated, "two") {
		t.Fatalf("old Matty block content remained:\n%s", updated)
	}
	for _, want := range []string{"before\n", "\nbetween\n", "\nafter"} {
		if !strings.Contains(updated, want) {
			t.Fatalf("outside content %q not preserved in update:\n%s", want, updated)
		}
	}

	removed := removeSection(existing, codexMattySectionID)
	want := "before\n\nbetween\n\nafter"
	if removed != want {
		t.Fatalf("remove should delete all Matty blocks and preserve intervening bytes:\ngot:  %q\nwant: %q", removed, want)
	}
}

func TestDetectExternalManagedBlocks(t *testing.T) {
	warnings := DetectExternalManagedBlocks("<!-- gentle-ai:persona -->\n<!-- /gentle-ai:persona -->\n<!-- engram:memory -->\n")
	if len(warnings) != 2 {
		t.Fatalf("warnings = %#v, want gentle-ai and Engram warnings", warnings)
	}
}
