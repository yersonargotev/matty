package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/yersonargotev/packy/internal/governanceauth"
)

func main() {
	var authorizationPath string
	var declarationPath string
	flag.StringVar(&authorizationPath, "authorization", "", "path to trusted pull-request and closing-issue metadata JSON")
	flag.StringVar(&declarationPath, "declaration", "", "path to pull-request metadata whose exception declaration should be projected")
	flag.Parse()

	if (authorizationPath == "") == (declarationPath == "") {
		fmt.Fprintln(os.Stderr, "exactly one of --authorization or --declaration is required")
		os.Exit(2)
	}
	if declarationPath != "" {
		var pullRequest struct {
			Body string `json:"body"`
		}
		if err := decode(declarationPath, &pullRequest); err != nil {
			fmt.Fprintf(os.Stderr, "read pull request: %v\n", err)
			os.Exit(1)
		}
		declaration, present, err := governanceauth.ParseExceptionDeclaration(pullRequest.Body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "authorization denied: %v\n", err)
			os.Exit(1)
		}
		if !present {
			fmt.Println("null")
			return
		}
		if err := json.NewEncoder(os.Stdout).Encode(map[string]string{"type": declaration.Type, "url": declaration.URL}); err != nil {
			fmt.Fprintf(os.Stderr, "write declaration: %v\n", err)
			os.Exit(1)
		}
		return
	}

	var authorization struct {
		Event    governanceauth.Event    `json:"event"`
		Metadata governanceauth.Metadata `json:"metadata"`
	}
	if err := decode(authorizationPath, &authorization); err != nil {
		fmt.Fprintf(os.Stderr, "read authorization: %v\n", err)
		os.Exit(1)
	}

	if err := governanceauth.Validate(authorization.Event, authorization.Metadata); err != nil {
		fmt.Fprintf(os.Stderr, "authorization denied: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("authorization approved for pull request #%d\n", authorization.Event.PullRequest.Number)
}

func decode(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	return nil
}
