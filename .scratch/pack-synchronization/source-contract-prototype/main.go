// PROTOTYPE — throw this TUI away after the contract decision is captured.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const fixtureDir = ".scratch/pack-synchronization/source-contract-prototype"

type prototype struct {
	config contract
	lock   contract
	base   observed
	caseID int
}

var cases = []string{
	"current Matty bundle",
	"byte-identical vendoring",
	"moved tag",
	"repository replacement",
	"selected file missing",
}

func main() {
	check := flag.Bool("check", false, "print the current validation result without starting the TUI")
	flag.Parse()
	p, err := loadPrototype()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *check {
		r := evaluate(p.config, p.lock, p.scenario())
		printResult(cases[p.caseID], r)
		if status(r) != "BLOCKED" {
			os.Exit(1)
		}
		return
	}

	in := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\033[2J\033[H")
		printResult(cases[p.caseID], evaluate(p.config, p.lock, p.scenario()))
		fmt.Println("\n\033[1m[n]\033[0m next scenario  \033[1m[p]\033[0m previous scenario  \033[1m[q]\033[0m quit")
		if !in.Scan() || strings.TrimSpace(in.Text()) == "q" {
			return
		}
		switch strings.TrimSpace(in.Text()) {
		case "n":
			p.caseID = (p.caseID + 1) % len(cases)
		case "p":
			p.caseID = (p.caseID + len(cases) - 1) % len(cases)
		}
	}
}

func loadPrototype() (prototype, error) {
	var p prototype
	if err := readJSON(filepath.Join(fixtureDir, "example.sources.json"), &p.config); err != nil {
		return p, err
	}
	if err := readJSON(filepath.Join(fixtureDir, "example.sources.lock.json"), &p.lock); err != nil {
		return p, err
	}
	lockedRepo := p.lock.Sources[0].Repository.(map[string]any)
	p.base = observed{
		RepositoryID: int64(lockedRepo["id"].(float64)),
		TagObjectSHA: p.lock.Sources[0].Git.TagRef.ObjectSHA,
		Files:        map[string]map[string]file{},
	}
	for _, r := range p.lock.Sources[0].Resources {
		files, err := readVendoredFiles(r.VendoredPath)
		if err != nil {
			return p, err
		}
		p.base.Files[bindingKey(r)] = files
	}
	return p, nil
}

func (p prototype) scenario() observed {
	o := cloneObserved(p.base)
	switch p.caseID {
	case 1:
		for _, r := range p.lock.Sources[0].Resources {
			o.Files[bindingKey(r)] = map[string]file{}
			for _, f := range r.Files {
				o.Files[bindingKey(r)][f.Path] = f
			}
		}
	case 2:
		o.TagObjectSHA = strings.Repeat("f", 40)
	case 3:
		o.RepositoryID++
	case 4:
		first := p.lock.Sources[0].Resources[0]
		delete(o.Files[bindingKey(first)], first.Files[0].Path)
	}
	return o
}

func cloneObserved(in observed) observed {
	out := observed{RepositoryID: in.RepositoryID, TagObjectSHA: in.TagObjectSHA, Files: map[string]map[string]file{}}
	for resource, files := range in.Files {
		out.Files[resource] = map[string]file{}
		for path, f := range files {
			out.Files[resource][path] = f
		}
	}
	return out
}

func readVendoredFiles(root string) (map[string]file, error) {
	files := map[string]file{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		files[filepath.ToSlash(rel)] = file{Path: filepath.ToSlash(rel), Size: len(data), SHA256: hex.EncodeToString(sum[:])}
		return nil
	})
	return files, err
}

func readJSON(path string, into any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, into)
}

func printResult(name string, r result) {
	fmt.Printf("\033[1mSource configuration + provenance lock contract\033[0m\n")
	fmt.Printf("\033[2mscenario:\033[0m %s\n\n", name)
	fields := [][2]any{
		{"decision", status(r)}, {"configured sources", r.ConfigSources}, {"selected resources", r.Bindings},
		{"locked files", r.LockedFiles}, {"byte-identical", r.ByteIdentical}, {"drifted", r.Drifted},
		{"missing", r.Missing}, {"unexpected", r.Unexpected},
	}
	for _, field := range fields {
		fmt.Printf("\033[1m%-22s\033[0m %v\n", field[0], field[1])
	}
	failures := append([]string(nil), r.Failures...)
	sort.Strings(failures)
	if len(failures) == 0 {
		fmt.Println("\n\033[1mfailures\033[0m none")
		return
	}
	fmt.Printf("\n\033[1mfailures\033[0m\n  - %s\n", strings.Join(failures, "\n  - "))
}
