// THROWAWAY PROTOTYPE: this models a discussion; it is not capability-pack implementation.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

type plan struct {
	operation   string
	desiredRev  string
	observedRev string
	actions     []string
	digest      string
}

func buildPlan(observedRev string) plan {
	p := plan{
		operation:   "activate engram on codex",
		desiredRev:  "desired-v1",
		observedRev: observedRev,
		actions:     []string{"write instruction projection", "run engram setup codex"},
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%q", p.operation, p.desiredRev, p.observedRev, p.actions)))
	p.digest = hex.EncodeToString(sum[:8])
	return p
}

func apply(approved plan, currentObservedRev string) error {
	fmt.Printf("apply input: approved=%s current-observation=%s\n", approved.digest, currentObservedRev)
	if currentObservedRev != approved.observedRev {
		return fmt.Errorf("STALE PLAN: approved observation=%s; current observation=%s; applied=0 actions", approved.observedRev, currentObservedRev)
	}
	for i, action := range approved.actions {
		fmt.Printf("apply action %d/%d from approved plan %s: %s\n", i+1, len(approved.actions), approved.digest, action)
	}
	return nil
}

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "happy" && os.Args[1] != "stale" && os.Args[1] != "partial") {
		fmt.Fprintln(os.Stderr, "usage: go run ./.scratch/capability-packs/reconciliation-prototype <happy|stale|partial>")
		os.Exit(2)
	}

	approved := buildPlan("host-v1")
	fmt.Printf("inspect: observed=%s\n", approved.observedRev)
	fmt.Printf("plan: digest=%s desired=%s actions=%q\n", approved.digest, approved.desiredRev, approved.actions)
	fmt.Printf("human approval: digest=%s\n", approved.digest)

	current := "host-v1"
	if os.Args[1] == "stale" {
		current = "host-v2"
		fmt.Printf("concurrent change: observed=%s\n", current)
	}
	if os.Args[1] == "partial" {
		fmt.Printf("local transaction: staged, validated, committed; backup retained until verification\n")
		fmt.Printf("external barrier: run engram setup codex -> FAILED\n")
		fmt.Printf("journal: local=completed external=failed attempt=recovery-required\n")
		fmt.Printf("readiness: configured=false authorized=false usable=false pending-human-action=false\n")
		fmt.Fprintln(os.Stderr, "PARTIAL FAILURE: inspect reality and preview a new recovery plan")
		os.Exit(1)
	}
	if err := apply(approved, current); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("result: applied exact approved plan %s\n", approved.digest)
}
