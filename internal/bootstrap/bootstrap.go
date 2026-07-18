package bootstrap

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/yersonargotev/packy/internal/skillbundle"
)

const DefaultRepositoryURL = "https://github.com/yersonargotev/packy.git"

// BootstrapOptions describes how packy init prepares the package-installed
// Source of Truth checkout. It lives outside command construction so the CLI
// remains an adapter around source bootstrapping behavior.
type BootstrapOptions struct {
	InstalledSource InstalledSource
	RepositoryURL   string
	RepositoryRef   string
	HomeDir         string
	ConfigHome      string
	ReportProgress  func(string) error
}

type BootstrapResult struct {
	Cloned  bool
	Updated bool
}

func EnsureInstalledSource(opts BootstrapOptions) (BootstrapResult, error) {
	if strings.TrimSpace(opts.InstalledSource.Root()) == "" {
		return BootstrapResult{}, errors.New("installed source root is required")
	}
	if strings.TrimSpace(opts.RepositoryURL) == "" {
		opts.RepositoryURL = DefaultRepositoryURL
	}

	result := BootstrapResult{}
	if validateInstalledSource(opts.InstalledSource.Root()) == nil {
		updated, err := ensureInstalledSourceRef(opts)
		if err != nil {
			return BootstrapResult{}, err
		}
		result.Updated = updated
		return result, nil
	}

	info, err := os.Stat(opts.InstalledSource.Root())
	switch {
	case err == nil && !info.IsDir():
		return BootstrapResult{}, fmt.Errorf("Installed Source path exists but is not a directory: %s", opts.InstalledSource.Root())
	case err == nil:
		empty, err := dirEmpty(opts.InstalledSource.Root())
		if err != nil {
			return BootstrapResult{}, err
		}
		if !empty {
			return BootstrapResult{}, fmt.Errorf("Installed Source path exists but is not a valid Packy checkout: %s. Move it aside or pass --source-root", opts.InstalledSource.Root())
		}
		if err := os.Remove(opts.InstalledSource.Root()); err != nil {
			return BootstrapResult{}, fmt.Errorf("remove empty Installed Source directory: %w", err)
		}
	case !os.IsNotExist(err):
		return BootstrapResult{}, fmt.Errorf("inspect Installed Source: %w", err)
	}

	if _, err := exec.LookPath("git"); err != nil {
		return BootstrapResult{}, fmt.Errorf("git is required to clone the Packy Source of Truth into %s", opts.InstalledSource.Root())
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
		return false, fmt.Errorf("Installed Source at %s has local changes; refusing to update to %s. Commit/stash them, move it aside, or pass --source-root", opts.InstalledSource.Root(), ref)
	}
	if err := reportProgress(opts, fmt.Sprintf("updating Installed Source at %s to %s", opts.InstalledSource.Root(), ref)); err != nil {
		return false, err
	}
	if err := fetchInstalledSourceRef(opts, ref); err != nil {
		return false, fmt.Errorf("update Installed Source to %s: %w", ref, err)
	}
	if err := validateFetchedInstalledSource(opts); err != nil {
		return false, err
	}
	if _, err := gitOutput(opts, "checkout", "--detach", "FETCH_HEAD"); err != nil {
		return false, fmt.Errorf("checkout Installed Source ref %s: %w", ref, err)
	}
	return true, nil
}

func validateFetchedInstalledSource(opts BootstrapOptions) (err error) {
	validationRoot, err := os.MkdirTemp(filepath.Dir(opts.InstalledSource.Root()), ".packy-validate.*")
	if err != nil {
		return fmt.Errorf("create Installed Source validation directory: %w", err)
	}
	if err := os.Remove(validationRoot); err != nil {
		return fmt.Errorf("prepare Installed Source validation worktree: %w", err)
	}
	if _, err := gitOutput(opts, "worktree", "add", "--detach", validationRoot, "FETCH_HEAD"); err != nil {
		return fmt.Errorf("prepare fetched Installed Source for validation: %w", err)
	}
	defer func() {
		_, cleanupErr := gitOutput(opts, "worktree", "remove", "--force", validationRoot)
		_ = os.RemoveAll(validationRoot)
		if cleanupErr != nil && err == nil {
			err = fmt.Errorf("clean up Installed Source validation worktree: %w", cleanupErr)
		}
	}()

	if err := validateInstalledSource(validationRoot); err != nil {
		return fmt.Errorf("fetched Installed Source has an invalid skill bundle: %w", err)
	}
	return nil
}

func fetchInstalledSourceRef(opts BootstrapOptions, ref string) error {
	if strings.HasPrefix(ref, "v") {
		tagRef := "refs/tags/" + ref
		if _, err := gitOutput(opts, "fetch", "--depth", "1", "origin", tagRef+":"+tagRef); err == nil {
			return nil
		}
	}
	_, err := gitOutput(opts, "fetch", "--depth", "1", "origin", ref)
	return err
}

func ValidateInstalledSourceRef(opts BootstrapOptions) error {
	ref := strings.TrimSpace(opts.RepositoryRef)
	if ref == "" {
		return nil
	}
	if strings.TrimSpace(opts.InstalledSource.Root()) == "" {
		return errors.New("installed source root is required")
	}
	if validateInstalledSource(opts.InstalledSource.Root()) != nil {
		return fmt.Errorf("default Installed Source is missing or invalid at %s; run packy init to initialize it", skillbundle.InstalledSourceRoot(opts.InstalledSource))
	}
	matches, err := repositoryRefMatchesReadOnly(opts, fmt.Sprintf("run packy init to align it with %s", ref))
	if err != nil {
		return err
	}
	if !matches {
		return fmt.Errorf("default Installed Source at %s is stale for Packy %s; run packy init to align it before packy update", opts.InstalledSource.Root(), ref)
	}
	return nil
}

// repositoryRefMatchesReadOnly validates the already-installed checkout
// without invoking Git. Lifecycle Preview uses this path so dry-runs never
// execute commands; mutating bootstrap operations continue to use Git itself.
func repositoryRefMatchesReadOnly(opts BootstrapOptions, missingGitReason string) (bool, error) {
	repository, err := git.PlainOpenWithOptions(opts.InstalledSource.Root(), &git.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		if errors.Is(err, git.ErrRepositoryNotExists) {
			return false, fmt.Errorf("Installed Source at %s is not a git checkout; %s. Move it aside or pass --source-root", opts.InstalledSource.Root(), missingGitReason)
		}
		return false, fmt.Errorf("inspect Installed Source git metadata: %w", err)
	}
	head, err := repository.Head()
	if err != nil {
		return false, fmt.Errorf("inspect Installed Source HEAD: %w", err)
	}
	target, err := repository.ResolveRevision(plumbing.Revision(opts.RepositoryRef + "^{commit}"))
	if err != nil {
		return false, nil
	}
	return head.Hash() == *target, nil
}

func repositoryRefMatches(opts BootstrapOptions, missingGitReason string) (bool, error) {
	if _, err := os.Stat(filepath.Join(opts.InstalledSource.Root(), ".git")); err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("Installed Source at %s is not a git checkout; %s. Move it aside or pass --source-root", opts.InstalledSource.Root(), missingGitReason)
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
	parent := filepath.Dir(opts.InstalledSource.Root())
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create Installed Source parent: %w", err)
	}
	tmp, err := os.MkdirTemp(parent, ".packy-clone.*")
	if err != nil {
		return fmt.Errorf("create temporary clone directory: %w", err)
	}
	defer os.RemoveAll(tmp)

	args := []string{"clone", "--depth", "1"}
	if strings.TrimSpace(opts.RepositoryRef) != "" {
		args = append(args, "--branch", opts.RepositoryRef)
	}
	args = append(args, opts.RepositoryURL, tmp)
	if err := reportProgress(opts, fmt.Sprintf("cloning Installed Source into %s", opts.InstalledSource.Root())); err != nil {
		return err
	}
	if _, err := runGit(opts, args...); err != nil {
		return fmt.Errorf("clone Packy Source of Truth: %w", err)
	}
	if err := validateInstalledSource(tmp); err != nil {
		return fmt.Errorf("cloned Packy Source of Truth has an invalid skill bundle: %w", err)
	}
	if err := os.Rename(tmp, opts.InstalledSource.Root()); err != nil {
		return fmt.Errorf("install cloned Packy Source of Truth: %w", err)
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
	return runGit(opts, append([]string{"-C", opts.InstalledSource.Root()}, args...)...)
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

func validateInstalledSource(dir string) error {
	return skillbundle.ValidateSource(skillbundle.SourceRoot(dir), "")
}

func dirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("read Installed Source directory: %w", err)
	}
	return len(entries) == 0, nil
}
