// Command syncpacksource is a private repository renderer for packsync Check.
// It is intentionally outside cmd/ and is not included in release artifacts.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/yersonargotev/matty/internal/packsync"
	"github.com/yersonargotev/matty/internal/packsync/githubsource"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("syncpacksource", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	repositoryRoot := flags.String("repository-root", ".", "Matty repository root")
	sourceID := flags.String("source", "", "configured source id")
	format := flags.String("format", "human", "human or json")
	selectorMode := flags.String("selector", "", "optional selector override")
	selectorRef := flags.String("ref", "", "exact prerelease tag or full commit SHA")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	acquisition, err := os.MkdirTemp("", "matty-pack-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(acquisition)
	request := packsync.CheckRequest{RepositoryRoot: *repositoryRoot, SourceID: *sourceID, AcquisitionDir: acquisition}
	if *selectorMode != "" {
		request.Selector = &packsync.Selector{Mode: *selectorMode, Ref: *selectorRef}
	}
	plan, err := (packsync.Engine{Source: githubsource.New(nil)}).Check(ctx, request)
	if err != nil {
		return err
	}
	return render(output, plan, *format)
}

func render(output io.Writer, plan packsync.Plan, format string) error {
	switch format {
	case "human":
		_, err := io.WriteString(output, plan.Human())
		return err
	case "json":
		data, err := plan.CanonicalJSON()
		if err != nil {
			return err
		}
		_, err = output.Write(data)
		return err
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}
