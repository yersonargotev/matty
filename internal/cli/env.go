package cli

import "os"

// Env provides environment lookup behind a small interface so tests can run
// Packy against sandboxed HOME/config paths without mutating the real machine.
type Env interface {
	Getenv(key string) string
}

type osEnv struct{}

func (osEnv) Getenv(key string) string { return os.Getenv(key) }

// MapEnv is a test-friendly environment implementation.
type MapEnv map[string]string

func (m MapEnv) Getenv(key string) string { return m[key] }
