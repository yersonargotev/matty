package capabilitypack

import "path/filepath"

// StateLayout is the capability-pack-owned state location beneath Packy Home.
type StateLayout struct {
	file string
}

func NewStateLayout(packyHome string) StateLayout {
	return StateLayout{file: filepath.Join(packyHome, "packs.json")}
}

func (l StateLayout) File() string { return l.file }
