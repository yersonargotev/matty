package localprojection

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

type Executor struct {
	Host         string
	SymlinkKinds map[capabilitypack.ProjectionActionKind]bool
	FileKinds    map[capabilitypack.ProjectionActionKind]bool
}

type stagedAction struct {
	action       capabilitypack.ProjectionAction
	temp, backup string
	hadTarget    bool
}

// TreeFile is one inert regular file in a staged local projection tree.
type TreeFile struct {
	Path    string
	Content []byte
	Mode    fs.FileMode
}

// Apply stages all supported local projections before committing them and
// restores already-committed targets if a later commit fails.
func (e Executor) Apply(actions []capabilitypack.ProjectionAction) error {
	items := make([]stagedAction, 0, len(actions))
	succeeded := false
	cleanupCreatedDirs := true
	var createdDirs []string
	createdSet := map[string]bool{}
	defer func() {
		for _, item := range items {
			_ = os.RemoveAll(item.temp)
			if succeeded {
				_ = os.RemoveAll(item.backup)
			}
		}
		if !succeeded && cleanupCreatedDirs {
			for i := len(createdDirs) - 1; i >= 0; i-- {
				_ = os.Remove(createdDirs[i])
			}
		}
	}()
	for _, action := range actions {
		dirs, err := ensureDir(filepath.Dir(action.Target))
		if err != nil {
			return capabilitypack.ProjectionActionError{ID: action.ID, Err: err}
		}
		for _, dir := range dirs {
			if !createdSet[dir] {
				createdSet[dir] = true
				createdDirs = append(createdDirs, dir)
			}
		}
		temp := filepath.Join(filepath.Dir(action.Target), ".packy-stage-"+FingerprintBytes([]byte(string(action.Kind) + ":" + action.ID))[:12])
		_ = os.RemoveAll(temp)
		items = append(items, stagedAction{action: action, temp: temp, backup: temp + ".backup"})
		if action.Mode == capabilitypack.ProjectionDeleteTarget {
			_, err := os.Lstat(action.Target)
			items[len(items)-1].hadTarget = err == nil
			if err != nil && !os.IsNotExist(err) {
				return capabilitypack.ProjectionActionError{ID: action.ID, Err: err}
			}
			continue
		}
		switch {
		case e.SymlinkKinds[action.Kind]:
			if err := os.Symlink(action.Source, temp); err != nil {
				return capabilitypack.ProjectionActionError{ID: action.ID, Err: fmt.Errorf("stage: %w", err)}
			}
			if _, err := filepath.EvalSymlinks(temp); err != nil {
				return capabilitypack.ProjectionActionError{ID: action.ID, Err: fmt.Errorf("validate staged: %w", err)}
			}
		case e.FileKinds[action.Kind]:
			if err := os.WriteFile(temp, []byte(action.Content), 0o600); err != nil {
				return capabilitypack.ProjectionActionError{ID: action.ID, Err: fmt.Errorf("stage: %w", err)}
			}
			staged, err := os.ReadFile(temp)
			if err != nil {
				return capabilitypack.ProjectionActionError{ID: action.ID, Err: fmt.Errorf("validate staged: %w", err)}
			}
			if string(staged) != action.Content {
				return capabilitypack.ProjectionActionError{ID: action.ID, Err: fmt.Errorf("validate staged: content mismatch")}
			}
		default:
			return capabilitypack.ProjectionActionError{ID: action.ID, Err: fmt.Errorf("unsupported %s projection action %q", e.Host, action.Kind)}
		}
		_, err = os.Lstat(action.Target)
		items[len(items)-1].hadTarget = err == nil
	}
	committed := 0
	for i := range items {
		item := &items[i]
		if item.hadTarget {
			if err := os.Rename(item.action.Target, item.backup); err != nil {
				if rollbackErr := rollback(items[:committed]); rollbackErr != nil {
					cleanupCreatedDirs = false
					return capabilitypack.ProjectionActionError{ID: item.action.ID, Err: fmt.Errorf("commit: %v; rollback failed: %w", err, rollbackErr)}
				}
				return capabilitypack.ProjectionActionError{ID: items[i].action.ID, Err: err}
			}
		}
		if item.action.Mode == capabilitypack.ProjectionDeleteTarget {
			committed++
			continue
		}
		if err := os.Rename(item.temp, item.action.Target); err != nil {
			if item.hadTarget {
				if restoreErr := os.Rename(item.backup, item.action.Target); restoreErr != nil {
					cleanupCreatedDirs = false
					if rollbackErr := rollback(items[:committed]); rollbackErr != nil {
						return capabilitypack.ProjectionActionError{ID: item.action.ID, Err: fmt.Errorf("commit: %v; restore current target failed: %v; rollback failed: %w", err, restoreErr, rollbackErr)}
					}
					return capabilitypack.ProjectionActionError{ID: item.action.ID, Err: fmt.Errorf("commit: %v; restore current target failed: %w", err, restoreErr)}
				}
			}
			if rollbackErr := rollback(items[:committed]); rollbackErr != nil {
				cleanupCreatedDirs = false
				return capabilitypack.ProjectionActionError{ID: item.action.ID, Err: fmt.Errorf("commit: %v; rollback failed: %w", err, rollbackErr)}
			}
			return err
		}
		committed++
	}
	succeeded = true
	return nil
}

func ensureDir(dir string) ([]string, error) {
	var missing []string
	for current := dir; ; current = filepath.Dir(current) {
		if _, err := os.Stat(current); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	for i, j := 0, len(missing)-1; i < j; i, j = i+1, j-1 {
		missing[i], missing[j] = missing[j], missing[i]
	}
	return missing, nil
}

func rollback(items []stagedAction) error {
	for i := len(items) - 1; i >= 0; i-- {
		if err := os.RemoveAll(items[i].action.Target); err != nil {
			return err
		}
		if items[i].hadTarget {
			if err := os.Rename(items[i].backup, items[i].action.Target); err != nil {
				return capabilitypack.ProjectionActionError{ID: items[i].action.ID, Err: err}
			}
		}
	}
	return nil
}

func FingerprintBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func FingerprintPath(path string) (string, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "missing", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "broken", true, nil
		}
		value, err := FingerprintTree(target)
		return value, true, err
	}
	if info.IsDir() {
		value, err := FingerprintTree(path)
		return value, true, err
	}
	data, err := os.ReadFile(path)
	return FingerprintBytes(data), true, err
}

func FingerprintTree(root string) (string, error) {
	var parts []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parts = append(parts, filepath.ToSlash(rel)+"="+FingerprintBytes(data))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(parts)
	return FingerprintBytes([]byte(strings.Join(parts, "\n"))), nil
}

// FingerprintTreeFiles binds every relative path, file mode, and byte sequence
// before a composite tree reaches a host-visible path.
func FingerprintTreeFiles(files []TreeFile) (string, error) {
	normalized, err := normalizedTreeFiles(files)
	if err != nil {
		return "", err
	}
	parts := make([]string, len(normalized))
	for i, file := range normalized {
		parts[i] = fmt.Sprintf("%s\x00%04o\x00%s", file.Path, file.Mode.Perm(), FingerprintBytes(file.Content))
	}
	return FingerprintBytes([]byte(strings.Join(parts, "\n"))), nil
}

// FingerprintExactTree rejects links and special files and binds file modes in
// addition to the path/content facts used by legacy skill links.
func FingerprintExactTree(root string) (string, error) {
	var files []TreeFile
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("composite tree contains non-regular path %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, TreeFile{Path: filepath.ToSlash(rel), Content: data, Mode: info.Mode().Perm()})
		return nil
	})
	if err != nil {
		return "", err
	}
	return FingerprintTreeFiles(files)
}

// ReplaceTree stages and verifies one complete inert directory tree before a
// single rename publishes it. A failed stage or verification leaves an
// existing target byte-for-byte untouched.
func ReplaceTree(target string, files []TreeFile, expectedFingerprint string) (err error) {
	normalized, err := normalizedTreeFiles(files)
	if err != nil {
		return err
	}
	fingerprint, err := FingerprintTreeFiles(normalized)
	if err != nil {
		return err
	}
	if expectedFingerprint == "" || fingerprint != expectedFingerprint {
		return errors.New("composite tree fingerprint does not match sealed projection")
	}
	created, err := ensureDir(filepath.Dir(target))
	if err != nil {
		return err
	}
	suffix := fingerprint[:12]
	stage := filepath.Join(filepath.Dir(target), ".packy-tree-stage-"+suffix)
	backup := stage + ".backup"
	_ = os.RemoveAll(stage)
	_ = os.RemoveAll(backup)
	succeeded := false
	defer func() {
		_ = os.RemoveAll(stage)
		if succeeded {
			_ = os.RemoveAll(backup)
			return
		}
		for i := len(created) - 1; i >= 0; i-- {
			_ = os.Remove(created[i])
		}
	}()
	if err := os.Mkdir(stage, 0o700); err != nil {
		return err
	}
	for _, file := range normalized {
		path := filepath.Join(stage, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("stage composite tree: %w", err)
		}
		if err := os.WriteFile(path, file.Content, file.Mode.Perm()); err != nil {
			return fmt.Errorf("stage composite tree: %w", err)
		}
		if err := os.Chmod(path, file.Mode.Perm()); err != nil {
			return fmt.Errorf("stage composite tree mode: %w", err)
		}
	}
	stagedFingerprint, err := FingerprintExactTree(stage)
	if err != nil {
		return fmt.Errorf("verify staged composite tree: %w", err)
	}
	if stagedFingerprint != expectedFingerprint {
		return errors.New("staged composite tree fingerprint mismatch")
	}
	_, statErr := os.Lstat(target)
	hadTarget := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return statErr
	}
	if hadTarget {
		if err := os.Rename(target, backup); err != nil {
			return fmt.Errorf("backup composite tree: %w", err)
		}
	}
	if err := os.Rename(stage, target); err != nil {
		if hadTarget {
			if restoreErr := os.Rename(backup, target); restoreErr != nil {
				return fmt.Errorf("publish composite tree: %v; restore failed: %w", err, restoreErr)
			}
		}
		return fmt.Errorf("publish composite tree: %w", err)
	}
	succeeded = true
	return nil
}

func normalizedTreeFiles(files []TreeFile) ([]TreeFile, error) {
	if len(files) == 0 {
		return nil, errors.New("composite tree requires at least one file")
	}
	result := make([]TreeFile, len(files))
	seen := map[string]bool{}
	for i, file := range files {
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(file.Path)))
		if file.Path == "" || file.Path != clean || clean == "." || strings.HasPrefix(clean, "../") || filepath.IsAbs(filepath.FromSlash(file.Path)) || strings.Contains(file.Path, "\\") {
			return nil, fmt.Errorf("invalid composite tree path %q", file.Path)
		}
		if seen[file.Path] {
			return nil, fmt.Errorf("duplicate composite tree path %q", file.Path)
		}
		seen[file.Path] = true
		mode := file.Mode.Perm()
		if mode != 0o644 && mode != 0o755 {
			return nil, fmt.Errorf("invalid composite tree mode %04o for %q", mode, file.Path)
		}
		result[i] = TreeFile{Path: file.Path, Content: append([]byte(nil), file.Content...), Mode: mode}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}
