// Command addypromotiongate validates Addy promotion evidence against trusted CI context.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/yersonargotev/packy/internal/addyacceptance"
)

func main() {
	var context addyacceptance.PromotionValidationContext
	var evidencePath string
	flag.BoolVar(&context.PromotionChange, "promotion-change", false, "whether the evaluated diff changes Addy promotion state")
	flag.BoolVar(&context.FoundationChange, "foundation-change", false, "whether the evaluated diff changes established Addy promotion authority")
	flag.StringVar(&context.Repository, "repository", "", "trusted repository identity")
	flag.IntVar(&context.PullRequest, "pull-request", 0, "trusted pull request number")
	flag.StringVar(&context.BaseSHA, "base-sha", "", "trusted base commit SHA")
	flag.StringVar(&context.HeadSHA, "head-sha", "", "trusted head commit SHA")
	flag.StringVar(&context.EvaluatedMergeSHA, "evaluated-merge-sha", "", "trusted evaluated merge commit SHA")
	flag.StringVar(&context.Tag, "tag", "", "trusted exact release tag")
	flag.StringVar(&context.Workflow, "workflow", "", "trusted workflow path")
	flag.StringVar(&context.WorkflowDigest, "workflow-digest", "", "trusted workflow SHA-256")
	flag.StringVar(&context.RunID, "run-id", "", "trusted workflow run ID")
	flag.StringVar(&evidencePath, "evidence", "", "candidate promotion evidence JSON")
	flag.Parse()

	context.MatrixVersion = addyacceptance.PromotionMatrixVersion
	context.Now = time.Now().UTC()
	context.MaxAge = 24 * time.Hour
	inputs, err := reconstructPromotionInputs(context.BaseSHA, context.HeadSHA)
	if err != nil {
		fatalf("reconstruct trusted promotion inputs: %v", err)
	}
	context.Inputs = inputs

	var evidence addyacceptance.PromotionEvidence
	if context.PromotionChange {
		if evidencePath == "" {
			fatalf("promotion change requires evidence")
		}
		data, err := os.ReadFile(evidencePath)
		if err != nil {
			fatalf("read promotion evidence: %v", err)
		}
		evidence, err = addyacceptance.ValidateCanonicalPromotionEvidence(data, context)
		if err != nil {
			fatalf("validate canonical promotion evidence: %v", err)
		}
	} else if context.FoundationChange {
		if evidencePath != "" {
			fatalf("candidate evidence is not accepted for a foundation-only change")
		}
		evidence = addyacceptance.NewFoundationPromotionEvidence(context)
	} else {
		if evidencePath != "" {
			fatalf("evidence is not accepted for a non-promotion change")
		}
		evidence = addyacceptance.NewNotApplicablePromotionEvidence(context)
	}

	if !context.PromotionChange {
		if err := addyacceptance.ValidatePromotionEvidence(evidence, context); err != nil {
			fatalf("validate promotion evidence: %v", err)
		}
	}
	data, err := evidence.CanonicalJSON()
	if err != nil {
		fatalf("encode promotion evidence: %v", err)
	}
	if _, err := os.Stdout.Write(data); err != nil {
		fatalf("write promotion evidence: %v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "addy promotion gate: "+format+"\n", args...)
	os.Exit(1)
}
