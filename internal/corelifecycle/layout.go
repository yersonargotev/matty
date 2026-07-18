package corelifecycle

import "path/filepath"

// Layout is the classic lifecycle-owned state beneath Packy Home.
type Layout struct {
	packyHome string
	stateFile string
}

func NewLayout(packyHome string) Layout {
	return Layout{packyHome: packyHome, stateFile: filepath.Join(packyHome, "config.json")}
}

func (l Layout) PackyHome() string { return l.packyHome }
func (l Layout) StateFile() string { return l.stateFile }
