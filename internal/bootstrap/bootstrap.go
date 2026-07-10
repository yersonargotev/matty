package bootstrap

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const DefaultRepositoryURL = "https://github.com/yersonargotev/matty.git"

// BootstrapOptions describes how matty init prepares the package-installed
// Source of Truth checkout. It lives outside command construction so the CLI
// remains an adapter around source bootstrapping behavior.
type BootstrapOptions struct {
	SourceRoot     string
	RepositoryURL  string
	RepositoryRef  string
	HomeDir        string
	ConfigHome     string
	ReportProgress func(string) error
}

type BootstrapResult struct {
	SourceRoot string
	Cloned     bool
	Updated    bool
}

func EnsureInstalledSource(opts BootstrapOptions) (BootstrapResult, error) {
	if strings.TrimSpace(opts.SourceRoot) == "" {
		return BootstrapResult{}, errors.New("installed source root is required")
	}
	if strings.TrimSpace(opts.RepositoryURL) == "" {
		opts.RepositoryURL = DefaultRepositoryURL
	}

	result := BootstrapResult{SourceRoot: opts.SourceRoot}
	if validInstalledSource(opts.SourceRoot) {
		updated, err := ensureInstalledSourceRef(opts)
		if err != nil {
			return BootstrapResult{}, err
		}
		result.Updated = updated
		return result, nil
	}

	info, err := os.Stat(opts.SourceRoot)
	switch {
	case err == nil && !info.IsDir():
		return BootstrapResult{}, fmt.Errorf("Installed Source path exists but is not a directory: %s", opts.SourceRoot)
	case err == nil:
		empty, err := dirEmpty(opts.SourceRoot)
		if err != nil {
			return BootstrapResult{}, err
		}
		if !empty {
			return BootstrapResult{}, fmt.Errorf("Installed Source path exists but is not a valid Matty checkout: %s. Move it aside or pass --source-root", opts.SourceRoot)
		}
		if err := os.Remove(opts.SourceRoot); err != nil {
			return BootstrapResult{}, fmt.Errorf("remove empty Installed Source directory: %w", err)
		}
	case !os.IsNotExist(err):
		return BootstrapResult{}, fmt.Errorf("inspect Installed Source: %w", err)
	}

	if _, err := exec.LookPath("git"); err != nil {
		return BootstrapResult{}, fmt.Errorf("git is required to clone the Matty Source of Truth into %s", opts.SourceRoot)
	}
	if err := cloneInstalledSource(opts); err != nil {
		return BootstrapResult{}, err
	}
	result.Cloned = true
	return result, nil
}

func ensureInstalledSourceRef(opts BootstrapOptions) (bool, error) {
	ref := strings.TrimSpace(opts.RepositoryRef)
	if ref == "" {
		return false, nil
	}
	matches, err := repositoryRefMatches(opts, fmt.Sprintf("cannot update it to %s", ref))
	if err != nil {
		return false, err
	}
	if matches {
		return false, nil
	}
	if dirty, err := repositoryDirty(opts); err != nil {
		return false, err
	} else if dirty {
		return false, fmt.Errorf("Installed Source at %s has local changes; refusing to update to %s. Commit/stash them, move it aside, or pass --source-root", opts.SourceRoot, ref)
	}
	if err := reportProgress(opts, fmt.Sprintf("updating Installed Source at %s to %s", opts.SourceRoot, ref)); err != nil {
		return false, err
	}
	if _, err := gitOutput(opts, "fetch", "--depth", "1", "origin", ref); err != nil {
		return false, fmt.Errorf("update Installed Source to %s: %w", ref, err)
	}
	if _, err := gitOutput(opts, "checkout", "--detach", "FETCH_HEAD"); err != nil {
		return false, fmt.Errorf("checkout Installed Source ref %s: %w", ref, err)
	}
	if !validInstalledSource(opts.SourceRoot) {
		return false, errors.New("updated Installed Source does not contain bundle/skills")
	}
	return true, nil
}

func ValidateInstalledSourceRef(opts BootstrapOptions) error {
	ref := strings.TrimSpace(opts.RepositoryRef)
	if ref == "" {
		return nil
	}
	if strings.TrimSpace(opts.SourceRoot) == "" {
		return errors.New("installed source root is required")
	}
	if !validInstalledSource(opts.SourceRoot) {
		return fmt.Errorf("default Installed Source is missing or invalid at %s; run matty init to initialize it", filepath.Join(opts.SourceRoot, "bundle", "skills"))
	}
	matches, err := repositoryRefMatches(opts, fmt.Sprintf("run matty init to align it with %s", ref))
	if err != nil {
		return err
	}
	if !matches {
		return fmt.Errorf("default Installed Source at %s is stale for Matty %s; run matty init to align it before matty update", opts.SourceRoot, ref)
	}
	return nil
}

func repositoryRefMatches(opts BootstrapOptions, missingGitReason string) (bool, error) {
	if _, err := os.Stat(filepath.Join(opts.SourceRoot, ".git")); err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("Installed Source at %s is not a git checkout; %s. Move it aside or pass --source-root", opts.SourceRoot, missingGitReason)
		}
		return false, fmt.Errorf("inspect Installed Source git metadata: %w", err)
	}
	return repositoryAtRef(opts)
}

func repositoryAtRef(opts BootstrapOptions) (bool, error) {
	head, err := gitOutput(opts, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return false, fmt.Errorf("inspect Installed Source HEAD: %w", err)
	}
	target, err := gitOutput(opts, "rev-parse", "--verify", opts.RepositoryRef+"^{commit}")
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(head) == strings.TrimSpace(target), nil
}

func repositoryDirty(opts BootstrapOptions) (bool, error) {
	status, err := gitOutput(opts, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("inspect Installed Source status: %w", err)
	}
	return strings.TrimSpace(status) != "", nil
}

func cloneInstalledSource(opts BootstrapOptions) error {
	parent := filepath.Dir(opts.SourceRoot)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create Installed Source parent: %w", err)
	}
	tmp, err := os.MkdirTemp(parent, ".matty-clone.*")
	if err != nil {
		return fmt.Errorf("create temporary clone directory: %w", err)
	}
	defer os.RemoveAll(tmp)

	args := []string{"clone", "--depth", "1"}
	if strings.TrimSpace(opts.RepositoryRef) != "" {
		args = append(args, "--branch", opts.RepositoryRef)
	}
	args = append(args, opts.RepositoryURL, tmp)
	if err := reportProgress(opts, fmt.Sprintf("cloning Installed Source into %s", opts.SourceRoot)); err != nil {
		return err
	}
	if _, err := runGit(opts, args...); err != nil {
		return fmt.Errorf("clone Matty Source of Truth: %w", err)
	}
	if !validInstalledSource(tmp) {
		return errors.New("cloned Matty Source of Truth does not contain bundle/skills")
	}
	if err := os.Rename(tmp, opts.SourceRoot); err != nil {
		return fmt.Errorf("install cloned Matty Source of Truth: %w", err)
	}
	return nil
}

func reportProgress(opts BootstrapOptions, message string) error {
	if opts.ReportProgress == nil {
		return nil
	}
	if err := opts.ReportProgress(message); err != nil {
		return fmt.Errorf("report initialization progress: %w", err)
	}
	return nil
}

func gitOutput(opts BootstrapOptions, args ...string) (string, error) {
	return runGit(opts, append([]string{"-C", opts.SourceRoot}, args...)...)
}

func runGit(opts BootstrapOptions, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Env = gitEnv(opts)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func gitEnv(opts BootstrapOptions) []string {
	env := append([]string{}, os.Environ()...)
	env = append(env, "GIT_CONFIG_NOSYSTEM=1")
	if opts.HomeDir != "" {
		env = append(env, "HOME="+opts.HomeDir)
	}
	if opts.ConfigHome != "" {
		env = append(env, "XDG_CONFIG_HOME="+opts.ConfigHome)
	}
	return env
}

func validInstalledSource(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "bundle", "skills"))
	return err == nil && info.IsDir()
}

func dirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("read Installed Source directory: %w", err)
	}
	return len(entries) == 0, nil
}
