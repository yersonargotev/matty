package main

import (
	"fmt"
	"os"

	"github.com/yersonargotev/packy/internal/cli"
)

func main() {
	cmd := cli.NewRootCommand(cli.Options{})
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
