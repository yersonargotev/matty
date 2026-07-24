package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/yersonargotev/packy/internal/addyacceptance"
)

type manifestInventory struct {
	Resources []struct {
		Source string `json:"source"`
	} `json:"resources"`
}

func reconstructPromotionInputs(baseSHA, headSHA string) (addyacceptance.IndependentPromotionInputs, error) {
	base, err := reconstructAddyFiles(baseSHA)
	if err != nil {
		return addyacceptance.IndependentPromotionInputs{}, fmt.Errorf("reconstruct base: %w", err)
	}
	head, err := reconstructAddyFiles(headSHA)
	if err != nil {
		return addyacceptance.IndependentPromotionInputs{}, fmt.Errorf("reconstruct head: %w", err)
	}
	baseHistory, err := gitFiles(baseSHA, []string{"bundle/history/addy"})
	if err != nil {
		return addyacceptance.IndependentPromotionInputs{}, fmt.Errorf("reconstruct base history: %w", err)
	}
	headHistory, err := gitFiles(headSHA, []string{"bundle/history/addy"})
	if err != nil {
		return addyacceptance.IndependentPromotionInputs{}, fmt.Errorf("reconstruct head history: %w", err)
	}
	diff, err := gitOutputBytes("diff", "--binary", "--full-index", "--no-ext-diff", baseSHA, headSHA, "--")
	if err != nil {
		return addyacceptance.IndependentPromotionInputs{}, fmt.Errorf("reconstruct diff: %w", err)
	}
	diffSum := sha256.Sum256(diff)
	return addyacceptance.ReconstructIndependentPromotionInputs(addyacceptance.IndependentPromotionMaterial{
		Base:        base,
		Head:        head,
		BaseHistory: baseHistory,
		HeadHistory: headHistory,
		DiffSHA256:  hex.EncodeToString(diffSum[:]),
	})
}

func reconstructAddyFiles(ref string) ([]addyacceptance.ReconstructedFile, error) {
	manifestBytes, err := gitOutputBytes("show", ref+":bundle/packs/addy/pack.json")
	if err != nil {
		return nil, fmt.Errorf("read Addy manifest: %w", err)
	}
	var manifest manifestInventory
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("decode Addy manifest inventory: %w", err)
	}
	paths := []string{
		"bundle/packs/addy/pack.json",
		"bundle/sources/addy.lock.json",
		"bundle/sources.json",
	}
	for _, resource := range manifest.Resources {
		if resource.Source == "" || strings.HasPrefix(resource.Source, "/") || strings.Contains(resource.Source, "..") {
			return nil, fmt.Errorf("unsafe Addy source path %q", resource.Source)
		}
		paths = append(paths, "bundle/"+resource.Source)
	}
	sort.Strings(paths)
	paths = compactStrings(paths)
	files, err := gitFiles(ref, paths)
	if err != nil {
		return nil, err
	}
	for _, selected := range paths {
		found := false
		for _, file := range files {
			if file.Path == selected || strings.HasPrefix(file.Path, selected+"/") {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("selected Addy path %q is absent", selected)
		}
	}
	return files, nil
}

func gitFiles(ref string, paths []string) ([]addyacceptance.ReconstructedFile, error) {
	args := []string{"ls-tree", "-r", "-z", ref, "--"}
	args = append(args, paths...)
	output, err := gitOutputBytes(args...)
	if err != nil {
		return nil, err
	}
	records := bytes.Split(output, []byte{0})
	files := make([]addyacceptance.ReconstructedFile, 0, len(records))
	for _, encoded := range records {
		if len(encoded) == 0 {
			continue
		}
		header, path, found := bytes.Cut(encoded, []byte{'\t'})
		fields := strings.Fields(string(header))
		if !found || len(fields) != 3 || fields[1] != "blob" || path == nil {
			return nil, fmt.Errorf("invalid Git tree entry %q", encoded)
		}
		mode, err := strconv.ParseUint(fields[0], 8, 32)
		if err != nil {
			return nil, fmt.Errorf("parse mode for %s: %w", path, err)
		}
		content, err := gitOutputBytes("cat-file", "blob", fields[2])
		if err != nil {
			return nil, fmt.Errorf("read blob %s: %w", path, err)
		}
		sum := sha256.Sum256(content)
		files = append(files, addyacceptance.ReconstructedFile{
			Path:   string(path),
			Mode:   uint32(mode) & 0o777,
			SHA256: hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func gitOutputBytes(args ...string) ([]byte, error) {
	command := exec.Command("git", args...)
	output, err := command.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, err
	}
	return output, nil
}

func compactStrings(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}
