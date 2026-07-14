package capabilitypack

import "path/filepath"

// StateLayout is the capability-pack-owned state location beneath Matty Home.
type StateLayout struct {
	file string
}

func NewStateLayout(mattyHome string) StateLayout {
	return StateLayout{file: filepath.Join(mattyHome, "packs.json")}
}

func (l StateLayout) File() string { return l.file }
