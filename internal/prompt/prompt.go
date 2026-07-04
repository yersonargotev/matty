package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const codexMattySectionID = "skills-router"

type WriteResult struct {
	Warnings []string
}

func CodexContent() string {
	return strings.TrimSpace(`## Matty global workflow

- Global skills live in ~/.agents/skills. When a task matches a skill, read that skill's SKILL.md before acting.
- Use ask-matt at ~/.agents/skills/ask-matt as the router when you are unsure which skill or workflow applies.
- Use Engram memory tools when available: search before past-work or project-sensitive tasks; save decisions, discoveries, bug fixes, and conventions; summarize sessions before finishing.
- Apply host delegation rules when this Codex session exposes subagent/delegation tools. If unavailable, proceed inline and mention that delegation was unavailable.`) + "\n"
}

func WriteCodex(path string) (WriteResult, error) {
	existing, err := readOptionalFile(path)
	if err != nil {
		return WriteResult{}, err
	}
	result := WriteResult{Warnings: DetectExternalManagedBlocks(existing)}
	updated := upsertSection(existing, codexMattySectionID, CodexContent())
	if updated == existing {
		return result, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return WriteResult{}, fmt.Errorf("create Codex config directory %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		return WriteResult{}, fmt.Errorf("write Codex Matty prompt %s: %w", path, err)
	}
	return result, nil
}

func RemoveCodex(path string) error {
	existing, err := readOptionalFile(path)
	if err != nil {
		return err
	}
	updated := removeSection(existing, codexMattySectionID)
	if updated == existing {
		return nil
	}
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		return fmt.Errorf("remove Codex Matty prompt %s: %w", path, err)
	}
	return nil
}

func DetectExternalManagedBlocks(content string) []string {
	var warnings []string
	if strings.Contains(content, "<!-- gentle-ai:") || strings.Contains(content, "<!-- /gentle-ai:") {
		warnings = append(warnings, "Codex prompt contains gentle-ai managed blocks; Matty preserved them and only updated Matty markers")
	}
	if containsEngramMarker(content) {
		warnings = append(warnings, "Codex prompt contains Engram managed instructions; Matty preserved them and only updated Matty markers")
	}
	return warnings
}

func containsEngramMarker(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<!--") && strings.HasSuffix(trimmed, "-->") && strings.Contains(strings.ToLower(trimmed), "engram") {
			return true
		}
	}
	return false
}

func readOptionalFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}

func upsertSection(existing, sectionID, content string) string {
	return mergeSection(existing, sectionID, content)
}

func removeSection(existing, sectionID string) string {
	return mergeSection(existing, sectionID, "")
}

func mergeSection(existing, sectionID, content string) string {
	open := openMarker(sectionID)
	close := closeMarker(sectionID)
	block := sectionBlock(open, close, content)
	var out strings.Builder
	inserted := false
	for {
		openIdx := strings.Index(existing, open)
		if openIdx < 0 {
			out.WriteString(existing)
			break
		}
		closeRelIdx := strings.Index(existing[openIdx+len(open):], close)
		if closeRelIdx < 0 {
			out.WriteString(existing[:openIdx])
			existing = existing[openIdx+len(open):]
			continue
		}

		closeEnd := openIdx + len(open) + closeRelIdx + len(close)
		out.WriteString(existing[:openIdx])
		if content != "" && !inserted {
			out.WriteString(block)
			inserted = true
		}
		existing = existing[closeEnd:]
	}
	if content != "" && !inserted {
		out.WriteString(block)
	}
	return out.String()
}

func openMarker(sectionID string) string  { return "<!-- matty:" + sectionID + " -->" }
func closeMarker(sectionID string) string { return "<!-- /matty:" + sectionID + " -->" }

func sectionBlock(open, close, content string) string {
	if content == "" {
		return ""
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return open + "\n" + content + close
}
