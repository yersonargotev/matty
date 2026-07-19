package addyacceptance

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// exactArchive is the immutable GitHub tarball for Commit. It is inspected as
// bytes only; no content from it is ever executed.
//
//go:embed testdata/addy-0.6.4.tar.gz
var exactArchive []byte

type UpstreamFile struct {
	Path string `json:"path"`
	Mode int64  `json:"mode"`
	Size int64  `json:"size"`
	Type byte   `json:"type"`
}

type AcquisitionReport struct {
	Written  []string       `json:"written"`
	Rejected []UpstreamFile `json:"rejected"`
}

func ExactArchive() []byte { return append([]byte(nil), exactArchive...) }

// InspectExactArchive returns a deterministic static inventory and rejects
// unsafe archive paths. Links remain inert inventory facts and are not files.
func InspectExactArchive() ([]UpstreamFile, error) {
	reader, err := gzip.NewReader(bytes.NewReader(exactArchive))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	archive := tar.NewReader(reader)
	var files []UpstreamFile
	var root string
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(filepath.ToSlash(header.Name), "/") {
			return nil, fmt.Errorf("unsafe archive path %q", header.Name)
		}
		parts := strings.Split(filepath.ToSlash(header.Name), "/")
		if len(parts) < 2 || parts[0] == "" || strings.Contains(header.Name, "\\") {
			continue
		}
		if root == "" {
			root = parts[0]
		}
		if parts[0] != root {
			return nil, fmt.Errorf("archive has multiple roots")
		}
		relative := strings.Join(parts[1:], "/")
		if relative == "" {
			continue
		}
		if filepath.IsAbs(relative) || strings.Contains("/"+relative+"/", "/../") {
			return nil, fmt.Errorf("unsafe archive path %q", header.Name)
		}
		if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA || header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			files = append(files, UpstreamFile{Path: relative, Mode: header.Mode & 0o7777, Size: header.Size, Type: header.Typeflag})
		}
	}
	return files, nil
}

// WriteExactAcquisition writes only regular upstream bytes to an empty
// disposable root. Links and all executable content remain unexecuted.
func WriteExactAcquisition(root string) error {
	_, err := AcquireExact(root)
	return err
}

// AcquireExact reports every rejected non-regular archive member instead of
// silently normalizing it into the acquired tree.
func AcquireExact(root string) (AcquisitionReport, error) {
	return acquireSafely(exactArchive, root)
}

func acquireSafely(data []byte, root string) (AcquisitionReport, error) {
	var emptyRootExisted bool
	if entries, err := os.ReadDir(root); err == nil {
		if len(entries) != 0 {
			return AcquisitionReport{}, fmt.Errorf("acquisition root must be empty: %s", root)
		}
		emptyRootExisted = true
	} else if !os.IsNotExist(err) {
		return AcquisitionReport{}, err
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o700); err != nil {
		return AcquisitionReport{}, err
	}
	staging, err := os.MkdirTemp(filepath.Dir(root), ".addy-acquisition-")
	if err != nil {
		return AcquisitionReport{}, err
	}
	report, err := acquireArchive(data, staging)
	if err != nil {
		_ = os.RemoveAll(staging)
		return report, err
	}
	if emptyRootExisted {
		if err := os.Remove(root); err != nil {
			_ = os.RemoveAll(staging)
			return report, err
		}
	}
	if err := os.Rename(staging, root); err != nil {
		_ = os.RemoveAll(staging)
		return report, err
	}
	return report, nil
}

func acquireArchive(data []byte, root string) (AcquisitionReport, error) {
	var report AcquisitionReport
	entries, err := os.ReadDir(root)
	if err != nil {
		if !os.IsNotExist(err) {
			return report, err
		}
		if err := os.MkdirAll(root, 0o700); err != nil {
			return report, err
		}
	} else if len(entries) != 0 {
		return report, fmt.Errorf("acquisition root must be empty: %s", root)
	}
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return report, err
	}
	defer reader.Close()
	archive := tar.NewReader(reader)
	var prefix string
	for {
		header, err := archive.Next()
		if err == io.EOF {
			return report, nil
		}
		if err != nil {
			return report, err
		}
		if strings.HasPrefix(filepath.ToSlash(header.Name), "/") {
			return report, fmt.Errorf("unsafe archive path %q", header.Name)
		}
		parts := strings.Split(filepath.ToSlash(header.Name), "/")
		if len(parts) < 2 {
			continue
		}
		if prefix == "" {
			prefix = parts[0]
		}
		relative := strings.Join(parts[1:], "/")
		if relative == "" {
			continue
		}
		if parts[0] != prefix || filepath.IsAbs(relative) || strings.Contains("/"+relative+"/", "/../") {
			return report, fmt.Errorf("unsafe archive path %q", header.Name)
		}
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			report.Rejected = append(report.Rejected, UpstreamFile{Path: relative, Mode: header.Mode & 0o7777, Size: header.Size, Type: header.Typeflag})
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			continue
		}
		if header.Mode&0o6000 != 0 || header.Mode&0o002 != 0 {
			return report, fmt.Errorf("unsafe archive mode %04o for %q", header.Mode&0o7777, relative)
		}
		target := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return report, err
		}
		content, err := io.ReadAll(archive)
		if err != nil {
			return report, err
		}
		if err := os.WriteFile(target, content, os.FileMode(header.Mode&0o777)); err != nil {
			return report, err
		}
		report.Written = append(report.Written, relative)
	}
}
