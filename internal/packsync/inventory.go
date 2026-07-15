package packsync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

func inventory(root string) ([]FileEvidence, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("resource root %s is not a real directory", root)
	}
	var files []FileEvidence
	err = filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		mode := entryInfo.Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe symlink: %s", name)
		}
		if mode.Perm()&0o022 != 0 || mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
			return fmt.Errorf("unsafe permissions %04o: %s", mode.Perm(), name)
		}
		if entry.IsDir() {
			return nil
		}
		if !mode.IsRegular() {
			return fmt.Errorf("unsafe non-regular file: %s", name)
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if !safeSlashPath(relative) {
			return fmt.Errorf("unsafe inventoried path %q", relative)
		}
		files = append(files, FileEvidence{Path: relative, Size: int64(len(data)), Mode: uint32(mode.Perm()), SHA256: hashBytes(data)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("selected resource %s contains no files", root)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func resourceHash(files []FileEvidence) string {
	hash := sha256.New()
	ordered := append([]FileEvidence(nil), files...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	for _, file := range ordered {
		fmt.Fprintf(hash, "%s\x00%d\x00%04o\x00%s\n", file.Path, file.Size, file.Mode, file.SHA256)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func snapshotHash(resources []ResourceEvidence) string {
	hash := sha256.New()
	ordered := append([]ResourceEvidence(nil), resources...)
	sort.Slice(ordered, func(i, j int) bool { return bindingKey(ordered[i].Binding) < bindingKey(ordered[j].Binding) })
	for _, resource := range ordered {
		fmt.Fprintf(hash, "%s\x00%s\x00%s\x00%s\n", bindingKey(resource.Binding), resource.UpstreamPath, resource.VendoredPath, resource.SHA256)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func treeHash(root string) (string, error) {
	files, err := inventory(root)
	if err != nil {
		return "", err
	}
	return resourceHash(files), nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func mapFiles(files []FileEvidence) map[string]FileEvidence {
	result := make(map[string]FileEvidence, len(files))
	for _, file := range files {
		result[file.Path] = file
	}
	return result
}
