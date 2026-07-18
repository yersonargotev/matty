package bootstrap

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/packy/internal/workstation"
)

// InstalledSource is the immutable location of Packy's package-installed
// checkout and its bundle.
type InstalledSource struct {
	root       string
	bundleRoot string
}

func (s InstalledSource) Root() string       { return s.root }
func (s InstalledSource) BundleRoot() string { return s.bundleRoot }

func DefaultInstalledSourceRoot(home string) string {
	return filepath.Join(home, ".local", "share", "packy")
}

func InstalledSourceAt(root string) InstalledSource {
	root = filepath.Clean(root)
	return InstalledSource{root: root, bundleRoot: filepath.Join(root, "bundle")}
}

// ResolveInstalledSource applies bootstrap's Installed Source layout and
// normalizes an explicit root relative to the captured invocation directory.
func ResolveInstalledSource(snapshot workstation.Snapshot, explicitRoot string) (InstalledSource, error) {
	root := strings.TrimSpace(explicitRoot)
	if root == "" {
		root = DefaultInstalledSourceRoot(snapshot.Home())
	}
	if !filepath.IsAbs(root) {
		currentDirectory, err := snapshot.CurrentDirectory()
		if err != nil {
			return InstalledSource{}, fmt.Errorf("resolve installed source root: %w", err)
		}
		root = filepath.Join(currentDirectory, root)
	}
	root = filepath.Clean(root)
	return InstalledSourceAt(root), nil
}
