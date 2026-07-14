package engrambin

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SetupLayout owns the Engram paths inspected by setup diagnosis.
type SetupLayout struct {
	topology Topology
	localBin string
}

func NewSetupLayout(home, homebrewPrefix string) SetupLayout {
	return SetupLayout{
		topology: NewTopology(homebrewPrefix),
		localBin: filepath.Join(home, ".local", "bin", "engram"),
	}
}

func (l SetupLayout) LocalBin() string { return l.localBin }

// SetupObservation is a detached read-only view of Engram executable,
// compatibility-link, version, and runtime facts.
type SetupObservation struct {
	expectedPath      string
	canonical         *Canonical
	lookupErr         error
	executables       []Executable
	localBinDiagnoses []LocalBinDiagnosis
	pathExecutable    *Executable
	processes         []Process
	processErr        error
}

func ObserveSetup(layout SetupLayout, pathEnv string, lookPath func(string) (string, error), facts Facts) SetupObservation {
	facts = facts.WithDefaults()
	canonical := DiscoverHomebrewFromPrefixes(layout.topology.prefixes)
	observation := SetupObservation{
		expectedPath: layout.topology.ExpectedPath(),
		canonical:    canonical,
	}

	resolved, err := lookPath("engram")
	observation.lookupErr = err
	if err == nil {
		paths := UniquePaths(resolved, pathEnv, layout.topology.prefixes)
		observation.executables = make([]Executable, 0, len(paths))
		for _, path := range paths {
			version, versionErr := facts.Version(path)
			observation.executables = append(observation.executables, NewExecutable(path, canonical, version, versionErr))
		}
	}
	observation.localBinDiagnoses = DiagnoseLocalBin(layout.localBin, canonical)

	resolved, err = lookPath("engram")
	if err == nil {
		executable := NewExecutable(resolved, canonical, "", nil)
		observation.pathExecutable = &executable
	}
	observation.processes, observation.processErr = facts.ServeProcesses()
	return observation
}

func (o SetupObservation) ExpectedPath() string { return o.expectedPath }
func (o SetupObservation) Canonical() *Canonical {
	if o.canonical == nil {
		return nil
	}
	copy := *o.canonical
	return &copy
}
func (o SetupObservation) LookupErr() error { return o.lookupErr }
func (o SetupObservation) Executables() []Executable {
	return append([]Executable(nil), o.executables...)
}
func (o SetupObservation) LocalBinDiagnoses() []LocalBinDiagnosis {
	return append([]LocalBinDiagnosis(nil), o.localBinDiagnoses...)
}
func (o SetupObservation) PathExecutable() *Executable {
	if o.pathExecutable == nil {
		return nil
	}
	copy := *o.pathExecutable
	return &copy
}
func (o SetupObservation) Processes() []Process { return append([]Process(nil), o.processes...) }
func (o SetupObservation) ProcessErr() error    { return o.processErr }

func DiagnoseVersionMismatch(executables []Executable) *VersionDiagnosis {
	versionByPath := []string{}
	versions := map[string]bool{}
	for _, executable := range executables {
		if executable.Version == "" {
			continue
		}
		versions[executable.Version] = true
		versionByPath = append(versionByPath, fmt.Sprintf("%s version %s", executable.Path, executable.Version))
	}
	if len(versions) <= 1 {
		return nil
	}
	return &VersionDiagnosis{Detail: "multiple engram executables report different versions: " + strings.Join(versionByPath, ", ")}
}

func DiagnoseHomebrewShadowing(executables []Executable) *VersionDiagnosis {
	if len(executables) < 2 || executables[0].Canonical {
		return nil
	}
	resolved := executables[0]
	for _, executable := range executables[1:] {
		if !executable.Canonical {
			continue
		}
		detail := fmt.Sprintf("%s appears before Homebrew Engram at %s", resolved.Path, executable.Path)
		if resolved.Version != "" {
			detail += " and reports version " + resolved.Version
		}
		if executable.Version != "" {
			detail += "; Homebrew reports version " + executable.Version
		}
		return &VersionDiagnosis{Detail: detail}
	}
	return nil
}
