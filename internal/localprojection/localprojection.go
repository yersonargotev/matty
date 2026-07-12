package localprojection

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yersonargotev/matty/internal/capabilitypack"
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
			return err
		}
		for _, dir := range dirs {
			if !createdSet[dir] {
				createdSet[dir] = true
				createdDirs = append(createdDirs, dir)
			}
		}
		temp := filepath.Join(filepath.Dir(action.Target), ".matty-stage-"+FingerprintBytes([]byte(string(action.Kind) + ":" + action.ID))[:12])
		_ = os.RemoveAll(temp)
		items = append(items, stagedAction{action: action, temp: temp, backup: temp + ".backup"})
		if action.Mode == capabilitypack.ProjectionDeleteTarget {
			_, err := os.Lstat(action.Target)
			items[len(items)-1].hadTarget = err == nil
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		switch {
		case e.SymlinkKinds[action.Kind]:
			if err := os.Symlink(action.Source, temp); err != nil {
				return fmt.Errorf("stage %s: %w", action.ID, err)
			}
			if _, err := filepath.EvalSymlinks(temp); err != nil {
				return fmt.Errorf("validate staged %s: %w", action.ID, err)
			}
		case e.FileKinds[action.Kind]:
			if err := os.WriteFile(temp, []byte(action.Content), 0o600); err != nil {
				return fmt.Errorf("stage %s: %w", action.ID, err)
			}
			staged, err := os.ReadFile(temp)
			if err != nil {
				return fmt.Errorf("validate staged %s: %w", action.ID, err)
			}
			if string(staged) != action.Content {
				return fmt.Errorf("validate staged %s: content mismatch", action.ID)
			}
		default:
			return fmt.Errorf("unsupported %s projection action %q", e.Host, action.Kind)
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
					return fmt.Errorf("commit %s: %v; rollback failed: %w", item.action.ID, err, rollbackErr)
				}
				return err
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
						return fmt.Errorf("commit %s: %v; restore current target failed: %v; rollback failed: %w", item.action.ID, err, restoreErr, rollbackErr)
					}
					return fmt.Errorf("commit %s: %v; restore current target failed: %w", item.action.ID, err, restoreErr)
				}
			}
			if rollbackErr := rollback(items[:committed]); rollbackErr != nil {
				cleanupCreatedDirs = false
				return fmt.Errorf("commit %s: %v; rollback failed: %w", item.action.ID, err, rollbackErr)
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
				return err
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
