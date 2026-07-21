package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/yersonargotev/packy/internal/claudesmoke"
)

func main() {
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
