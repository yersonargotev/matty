package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/governancedrift"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("governancedrift", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	mode := flags.String("mode", "", "evaluate, gate, or issue-decision")
	contractPath := flags.String("contract", "", "expected-state contract JSON")
	observationPath := flags.String("observation", "", "sanitized observation JSON")
	evaluationPath := flags.String("evaluation", "", "evaluation JSON")
	existingPath := flags.String("existing-issues", "", "canonical issue projections JSON")
	outputPath := flags.String("output", "", "output JSON path; stdout when omitted")
	boundary := flags.String("boundary", "", "promotion or publication")
	repository := flags.String("repository", "", "expected owner/repository")
	ref := flags.String("ref", "", "expected protected ref")
	commit := flags.String("commit", "", "expected commit SHA")
	workflowSHA := flags.String("workflow-sha", "", "expected workflow definition SHA")
	nowText := flags.String("now", "", "gate time in RFC3339")
	maxAgeText := flags.String("max-age", "192h", "maximum evidence age")
	canonicalKey := flags.String("canonical-key", "", "canonical drift issue key")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("governancedrift accepts flags only")
	}

	var result any
	switch *mode {
	case "evaluate":
		var contract governancedrift.Contract
		var observation governancedrift.Observation
		if err := readStrict(*contractPath, &contract); err != nil {
			return fmt.Errorf("read contract: %w", err)
		}
		if err := readStrict(*observationPath, &observation); err != nil {
			return fmt.Errorf("read observation: %w", err)
		}
		evaluation, err := governancedrift.Evaluate(contract, observation)
		if err != nil {
			return err
		}
		result = evaluation
	case "gate":
		var evaluation governancedrift.Evaluation
		if err := readStrict(*evaluationPath, &evaluation); err != nil {
			return fmt.Errorf("read evaluation: %w", err)
		}
		now, err := time.Parse(time.RFC3339, *nowText)
		if err != nil {
			return fmt.Errorf("parse --now: %w", err)
		}
		maxAge, err := time.ParseDuration(*maxAgeText)
		if err != nil {
			return fmt.Errorf("parse --max-age: %w", err)
		}
		decision := governancedrift.Gate(governancedrift.GateRequest{
			Boundary:    governancedrift.Boundary(*boundary),
			Repository:  *repository,
			Ref:         *ref,
			CommitSHA:   *commit,
			WorkflowSHA: *workflowSHA,
			Now:         now,
			MaxAge:      maxAge,
			Evaluations: []governancedrift.Evaluation{evaluation},
		})
		result = decision
		if err := writeJSON(*outputPath, stdout, result); err != nil {
			return err
		}
		if !decision.Allowed {
			return fmt.Errorf("governance drift blocks %s: %s", *boundary, strings.Join(decision.Reasons, "; "))
		}
		return nil
	case "issue-decision":
		var evaluation governancedrift.Evaluation
		var existing []governancedrift.ExistingIssue
		if err := readStrict(*evaluationPath, &evaluation); err != nil {
			return fmt.Errorf("read evaluation: %w", err)
		}
		if err := readStrict(*existingPath, &existing); err != nil {
			return fmt.Errorf("read existing issues: %w", err)
		}
		decision, err := governancedrift.DecideIssue(governancedrift.IssueRequest{
			CanonicalKey: *canonicalKey,
			Evaluation:   evaluation,
			Existing:     existing,
		})
		if err != nil {
			return err
		}
		result = decision
	default:
		return errors.New("--mode must be evaluate, gate, or issue-decision")
	}
	return writeJSON(*outputPath, stdout, result)
}

func readStrict(path string, target any) error {
	if path == "" {
		return errors.New("required input path is empty")
	}
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
		return errors.New("input contains more than one JSON value")
	}
	return nil
}

func writeJSON(path string, stdout io.Writer, value any) error {
	writer := stdout
	var file *os.File
	if path != "" {
		var err error
		file, err = os.Create(path)
		if err != nil {
			return err
		}
		defer file.Close()
		writer = file
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
