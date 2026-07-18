// Package workstation normalizes the ambient workstation facts shared by one
// Packy command invocation.
package workstation

import (
	"fmt"
	"path/filepath"
	"sync"
)

// Inputs are the ambient process facts captured at the CLI boundary.
type Inputs struct {
	Home                 string
	ConfigurationHome    string
	ExecutableSearchPath string
	HomebrewPrefix       string
	CurrentDirectory     string
	CurrentDirectoryErr  error
}

// Options contains explicit command overrides for snapshot construction.
type Options struct {
	Home string
}

// Snapshot is an immutable, normalized view of the workstation facts shared
// by one command invocation. Domain artifact paths deliberately do not belong
// here.
type Snapshot struct {
	home                 string
	configurationHome    string
	executableSearchPath string
	homebrewPrefix       string
	currentDirectory     string
	currentDirectoryErr  error
	packyHome            string
}

func (s Snapshot) Home() string                 { return s.home }
func (s Snapshot) ConfigurationHome() string    { return s.configurationHome }
func (s Snapshot) ExecutableSearchPath() string { return s.executableSearchPath }
func (s Snapshot) HomebrewPrefix() string       { return s.homebrewPrefix }
func (s Snapshot) CurrentDirectory() (string, error) {
	return s.currentDirectory, s.currentDirectoryErr
}
func (s Snapshot) PackyHome() string { return s.packyHome }

// Resolve normalizes captured ambient inputs into a narrow workstation
// snapshot. An explicit Home intentionally isolates configuration lookup from
// the ambient XDG configuration home.
func Resolve(inputs Inputs, opts Options) (Snapshot, error) {
	home := inputs.Home
	configurationHome := inputs.ConfigurationHome
	if opts.Home != "" {
		home = opts.Home
		configurationHome = ""
	}
	if home == "" {
		return Snapshot{}, fmt.Errorf("HOME is required")
	}
	if configurationHome == "" || !filepath.IsAbs(configurationHome) {
		configurationHome = filepath.Join(home, ".config")
	}

	return Snapshot{
		home:                 home,
		configurationHome:    configurationHome,
		executableSearchPath: inputs.ExecutableSearchPath,
		homebrewPrefix:       inputs.HomebrewPrefix,
		currentDirectory:     inputs.CurrentDirectory,
		currentDirectoryErr:  inputs.CurrentDirectoryErr,
		packyHome:            filepath.Join(home, ".packy"),
	}, nil
}

// Resolver captures and resolves workstation facts lazily at most once. The
// first Resolve call fixes both the ambient inputs and explicit overrides for
// the lifetime of the resolver.
type Resolver struct {
	capture func() (Inputs, error)
	once    sync.Once
	value   Snapshot
	err     error
}

func NewResolver(capture func() (Inputs, error)) *Resolver {
	return &Resolver{capture: capture}
}

func (r *Resolver) Resolve(opts Options) (Snapshot, error) {
	r.once.Do(func() {
		inputs, err := r.capture()
		if err != nil {
			r.err = err
			return
		}
		r.value, r.err = Resolve(inputs, opts)
	})
	return r.value, r.err
}
