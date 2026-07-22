// Command releasesbom generates Packy's deterministic file-level SPDX SBOM.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/release"
)

var versionPattern = regexp.MustCompile(`^v0\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("releasesbom", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var version, createdText, distPath, outputPath string
	flags.StringVar(&version, "version", "", "release version")
	flags.StringVar(&createdText, "created", "", "RFC3339 source timestamp")
	flags.StringVar(&distPath, "dist", "", "retained binary directory")
	flags.StringVar(&outputPath, "out", "", "sbom.spdx.json output")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if !versionPattern.MatchString(version) {
		return errors.New("version must have form v0.x.y")
	}
	created, err := time.Parse(time.RFC3339, createdText)
	if err != nil {
		return fmt.Errorf("created must be RFC3339: %w", err)
	}
	if distPath == "" || outputPath == "" {
		return errors.New("dist and out are required")
	}
	if filepath.Base(outputPath) != release.SBOMName {
		return fmt.Errorf("out must be named %s", release.SBOMName)
	}
	dist, output, err := validatePaths(distPath, outputPath)
	if err != nil {
		return err
	}
	subjects, err := observeBinaries(dist)
	if err != nil {
		return err
	}
	document := makeDocument(version, created.UTC().Format(time.RFC3339), subjects)
	data, err := canonicalJSON(document)
	if err != nil {
		return err
	}
	if err := release.VerifySPDXSBOM(data, version, subjects); err != nil {
		return fmt.Errorf("verify generated SPDX SBOM: %w", err)
	}
	if err := writeFileNoReplace(output, data); err != nil {
		return err
	}
	_, err = stdout.Write(data)
	return err
}

type document struct {
	SPDXVersion       string       `json:"spdxVersion"`
	SPDXID            string       `json:"SPDXID"`
	DataLicense       string       `json:"dataLicense"`
	Name              string       `json:"name"`
	DocumentNamespace string       `json:"documentNamespace"`
	CreationInfo      creationInfo `json:"creationInfo"`
	DocumentDescribes []string     `json:"documentDescribes"`
	Files             []file       `json:"files"`
}
type creationInfo struct {
	Created  string   `json:"created"`
	Creators []string `json:"creators"`
}
type file struct {
	FileName         string     `json:"fileName"`
	SPDXID           string     `json:"SPDXID"`
	Checksums        []checksum `json:"checksums"`
	LicenseConcluded string     `json:"licenseConcluded"`
	CopyrightText    string     `json:"copyrightText"`
}
type checksum struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"checksumValue"`
}

func makeDocument(version, created string, subjects []release.Subject) document {
	result := document{SPDXVersion: "SPDX-2.3", SPDXID: "SPDXRef-DOCUMENT", DataLicense: "CC0-1.0", Name: "packy-" + version, DocumentNamespace: "https://github.com/yersonargotev/packy/releases/download/" + version + "/sbom.spdx.json", CreationInfo: creationInfo{Created: created, Creators: []string{"Tool: packy-release"}}, DocumentDescribes: make([]string, 0, len(subjects)), Files: make([]file, 0, len(subjects))}
	for _, subject := range subjects {
		id := "SPDXRef-File-" + hex.EncodeToString([]byte(subject.Name))
		result.DocumentDescribes = append(result.DocumentDescribes, id)
		result.Files = append(result.Files, file{FileName: subject.Name, SPDXID: id, Checksums: []checksum{{Algorithm: "SHA256", Value: subject.SHA256}}, LicenseConcluded: "NOASSERTION", CopyrightText: "NOASSERTION"})
	}
	return result
}

func observeBinaries(dir string) ([]release.Subject, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dist: %w", err)
	}
	subjects := make([]release.Subject, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			return nil, fmt.Errorf("dist contains hidden entry %q", entry.Name())
		}
		if entry.Name() == release.ChecksumsName || entry.Name() == release.SBOMName {
			return nil, fmt.Errorf("dist already contains release metadata %q", entry.Name())
		}
		path := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("dist entry %q is not a regular file", entry.Name())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		subjects = append(subjects, release.Subject{Name: entry.Name(), SHA256: hex.EncodeToString(sum[:])})
	}
	if len(subjects) == 0 {
		return nil, errors.New("dist must contain at least one binary")
	}
	sort.Slice(subjects, func(i, j int) bool { return subjects[i].Name < subjects[j].Name })
	return subjects, nil
}

func validatePaths(distPath, outputPath string) (string, string, error) {
	info, err := os.Lstat(distPath)
	if err != nil {
		return "", "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", "", errors.New("dist must be a non-symlink directory")
	}
	dist, err := filepath.EvalSymlinks(distPath)
	if err != nil {
		return "", "", err
	}
	dist, err = filepath.Abs(dist)
	if err != nil {
		return "", "", err
	}
	if _, err := os.Lstat(outputPath); err == nil {
		return "", "", errors.New("output already exists")
	} else if !os.IsNotExist(err) {
		return "", "", err
	}
	parentPath := filepath.Dir(outputPath)
	parentInfo, err := os.Lstat(parentPath)
	if err != nil {
		return "", "", err
	}
	if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
		return "", "", errors.New("output parent must be a non-symlink directory")
	}
	parent, err := filepath.EvalSymlinks(parentPath)
	if err != nil {
		return "", "", err
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return "", "", err
	}
	output := filepath.Join(parent, filepath.Base(outputPath))
	rel, err := filepath.Rel(dist, output)
	if err != nil {
		return "", "", err
	}
	if rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", errors.New("output must not overlap dist")
	}
	return dist, output, nil
}

func canonicalJSON(value any) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
func writeFileNoReplace(path string, data []byte) error {
	parent := filepath.Dir(path)
	temporary, err := os.CreateTemp(parent, ".releasesbom-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err = temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(data)
	}
	closeErr := temporary.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Link(temporaryPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("refusing to overwrite existing output")
		}
		return fmt.Errorf("publish SBOM: %w", err)
	}
	return nil
}
