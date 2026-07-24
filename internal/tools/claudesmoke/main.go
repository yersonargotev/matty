package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/claudesmoke"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "verify-release" {
		verifyRelease(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "verify-addy-release" {
		verifyAddyRelease(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "qualify-addy" {
		qualifyAddy(os.Args[2:])
		return
	}
	var cfg claudesmoke.Config
	flag.StringVar(&cfg.Packy, "packy", "", "prebuilt Packy executable")
	flag.StringVar(&cfg.SourceRepo, "source-repo", "", "local Packy source repository")
	flag.StringVar(&cfg.SourceRef, "source-ref", "", "source ref used by package install")
	flag.StringVar(&cfg.ClaudeSelector, "claude-version", claudesmoke.ExactFloor, "Claude version: 2.1.203 or stable")
	flag.StringVar(&cfg.EvidencePath, "evidence", "", "canonical JSON evidence output")
	flag.StringVar(&cfg.NPM, "npm", "npm", "npm executable")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if _, err := claudesmoke.Run(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "claudesmoke:", err)
		os.Exit(1)
	}
}

func qualifyAddy(args []string) {
	flags := flag.NewFlagSet("qualify-addy", flag.ExitOnError)
	evidencePath := flags.String("evidence", "", "canonical Claude smoke evidence")
	outputPath := flags.String("output", "", "canonical Addy qualification output")
	repository := flags.String("repository", "", "trusted repository identity")
	workflow := flags.String("workflow", "", "trusted workflow path")
	workflowDigest := flags.String("workflow-digest", "", "trusted workflow SHA-256")
	runID := flags.String("run-id", "", "trusted workflow run ID")
	tag := flags.String("tag", "", "trusted exact release tag")
	checkout := flags.String("checkout", "", "source checkout used only for exact acquisition")
	packy := flags.String("packy", "", "prebuilt Packy executable")
	synthetic := flags.Bool("synthetic", false, "qualify the pre-candidate harness without production admission")
	if err := flags.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, "claudesmoke qualify-addy:", err)
		os.Exit(2)
	}
	var evidence claudesmoke.Evidence
	if err := strictDecode(*evidencePath, &evidence); err != nil {
		fmt.Fprintln(os.Stderr, "claudesmoke qualify-addy:", err)
		os.Exit(1)
	}
	packyPath, err := filepath.Abs(*packy)
	if err != nil {
		fmt.Fprintln(os.Stderr, "claudesmoke qualify-addy:", err)
		os.Exit(1)
	}
	executableDigest, err := fileDigest(packyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "claudesmoke qualify-addy:", err)
		os.Exit(1)
	}
	commandBytes, err := json.Marshal(evidence.Commands)
	if err != nil {
		fmt.Fprintln(os.Stderr, "claudesmoke qualify-addy:", err)
		os.Exit(1)
	}
	commandDigest := sha256.Sum256(commandBytes)
	qualification := claudesmoke.AddyQualification{
		SchemaVersion: 1, Synthetic: *synthetic, Repository: *repository, Workflow: *workflow,
		WorkflowDigest: *workflowDigest, RunID: *runID, Commit: evidence.PackySHA,
		Checkout: *checkout, PackyExecutable: packyPath, PackyExecutableDigest: executableDigest,
		InstalledSource: filepath.Join(evidence.Sandbox, "installed-source"), InstalledSourceCommit: evidence.InstalledSourceSHA, InstalledSourceClean: true,
		Sandbox: evidence.Sandbox,
		WritableRoots: claudesmoke.AddyWritableRoots{
			Home: filepath.Join(evidence.Sandbox, "home"), XDGConfig: filepath.Join(evidence.Sandbox, "config"),
			ClaudeConfig: filepath.Join(evidence.Sandbox, "home", ".claude"), State: filepath.Join(evidence.Sandbox, "data"),
			Package: filepath.Join(evidence.Sandbox, "npm"), Repository: filepath.Join(evidence.Sandbox, "work"),
			Acquisition: filepath.Join(evidence.Sandbox, "acquisition"),
		},
		ProcessLogDigest: hex.EncodeToString(commandDigest[:]), Smoke: evidence,
	}
	if *tag != "" {
		qualification.Tag = *tag
	} else if strings.HasPrefix(evidence.PackyRef, "v") {
		qualification.Tag = evidence.PackyRef
	}
	if *synthetic {
		err = claudesmoke.ValidateAddyQualification(qualification)
	} else {
		err = claudesmoke.ValidateProductionAddyQualification(qualification)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "claudesmoke qualify-addy:", err)
		os.Exit(1)
	}
	data, err := claudesmoke.CanonicalAddyQualificationJSON(qualification)
	if err != nil {
		fmt.Fprintln(os.Stderr, "claudesmoke qualify-addy:", err)
		os.Exit(1)
	}
	if *outputPath == "" {
		_, err = os.Stdout.Write(data)
	} else {
		err = os.WriteFile(*outputPath, data, 0o600)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "claudesmoke qualify-addy:", err)
		os.Exit(1)
	}
}

func strictDecode(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("evidence contains trailing JSON")
	}
	return nil
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func verifyRelease(args []string) {
	flags := flag.NewFlagSet("verify-release", flag.ExitOnError)
	root := flags.String("evidence-root", "", "directory containing the four release evidence documents")
	version := flags.String("packy-version", "", "expected release tag reported by Packy")
	sha := flags.String("packy-sha", "", "expected release commit and Installed Source SHA")
	if err := flags.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, "claudesmoke verify-release:", err)
		os.Exit(2)
	}
	if err := claudesmoke.ValidateReleaseEvidenceMatrix(*root, *version, *sha); err != nil {
		fmt.Fprintln(os.Stderr, "claudesmoke verify-release:", err)
		os.Exit(1)
	}
}

func verifyAddyRelease(args []string) {
	flags := flag.NewFlagSet("verify-addy-release", flag.ExitOnError)
	root := flags.String("evidence-root", "", "directory containing the four Addy qualification documents")
	version := flags.String("packy-version", "", "expected exact release tag")
	sha := flags.String("packy-sha", "", "expected release and Installed Source SHA")
	production := flags.Bool("production", false, "require production-admissible exact candidate qualifications")
	if err := flags.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, "claudesmoke verify-addy-release:", err)
		os.Exit(2)
	}
	if err := claudesmoke.ValidateReleaseAddyQualificationMatrix(*root, *version, *sha, *production); err != nil {
		fmt.Fprintln(os.Stderr, "claudesmoke verify-addy-release:", err)
		os.Exit(1)
	}
}
