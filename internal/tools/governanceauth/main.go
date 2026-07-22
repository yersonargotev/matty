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
	flag.StringVar(&authorizationPath, "authorization", "", "path to trusted pull-request and closing-issue metadata JSON")
	flag.Parse()

	if authorizationPath == "" {
		fmt.Fprintln(os.Stderr, "--authorization is required")
		os.Exit(2)
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
