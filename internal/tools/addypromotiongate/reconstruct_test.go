package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReconstructPromotionInputsUsesNamedGitObjects(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "bundle/packs/addy/pack.json", `{"resources":[{"source":"skills/example"}]}`+"\n")
	writeFixtureFile(t, root, "bundle/sources/addy.lock.json", "{}\n")
	writeFixtureFile(t, root, "bundle/sources.json", "{}\n")
	writeFixtureFile(t, root, "bundle/skills/example/SKILL.md", "base\n")
	writeFixtureFile(t, root, "README.md", "base\n")
	runFixtureGit(t, root, "init", "-q")
	runFixtureGit(t, root, "config", "user.email", "fixture@example.test")
	runFixtureGit(t, root, "config", "user.name", "Fixture")
	runFixtureGit(t, root, "add", ".")
	runFixtureGit(t, root, "commit", "-qm", "base")
	base := fixtureGitOutput(t, root, "rev-parse", "HEAD")

	writeFixtureFile(t, root, "README.md", "candidate helper changed\n")
	runFixtureGit(t, root, "add", "README.md")
	runFixtureGit(t, root, "commit", "-qm", "unselected")
	unselected := fixtureGitOutput(t, root, "rev-parse", "HEAD")

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	inputs, err := reconstructPromotionInputs(base, unselected)
	if err != nil {
		t.Fatal(err)
	}
	if inputs.BaseSHA256 != inputs.HeadSHA256 {
		t.Fatal("candidate helper change altered independently selected Addy inputs")
	}

	writeFixtureFile(t, root, "bundle/skills/example/SKILL.md", "head\n")
	runFixtureGit(t, root, "add", "bundle/skills/example/SKILL.md")
	runFixtureGit(t, root, "commit", "-qm", "selected")
	selected := fixtureGitOutput(t, root, "rev-parse", "HEAD")
	inputs, err = reconstructPromotionInputs(unselected, selected)
	if err != nil {
		t.Fatal(err)
	}
	if inputs.BaseSHA256 == inputs.HeadSHA256 {
		t.Fatal("selected Addy byte change was not reconstructed from named Git objects")
	}
}

func writeFixtureFile(t *testing.T, root, relative, content string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runFixtureGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func fixtureGitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
