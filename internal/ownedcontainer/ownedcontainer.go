package ownedcontainer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Kind string

const (
	File      Kind = "file"
	Directory Kind = "directory"
)

type Record struct {
	Path string `json:"path"`
	Kind Kind   `json:"kind"`
}

func Provision(candidates []Record) ([]Record, error) {
	candidates = append([]Record(nil), candidates...)
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Kind != candidates[j].Kind {
			return candidates[i].Kind == Directory
		}
		iDepth := strings.Count(filepath.Clean(candidates[i].Path), string(os.PathSeparator))
		jDepth := strings.Count(filepath.Clean(candidates[j].Path), string(os.PathSeparator))
		if iDepth == jDepth {
			return candidates[i].Path < candidates[j].Path
		}
		return iDepth < jDepth
	})
	created := make([]Record, 0, len(candidates))
	for _, candidate := range candidates {
		var err error
		if candidate.Kind == Directory {
			err = os.Mkdir(candidate.Path, 0o700)
		} else {
			var file *os.File
			file, err = os.OpenFile(candidate.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err == nil {
				err = file.Close()
			}
		}
		if err == nil {
			created = append(created, candidate)
			continue
		}
		if os.IsExist(err) {
			continue
		}
		return created, fmt.Errorf("provision Matty container %s: %w", candidate.Path, err)
	}
	sortRecords(created)
	return created, nil
}

type Expectation struct {
	Record Record
	digest string
}

type Plan struct {
	expectations []Expectation
	seal         string
}

var ErrStalePlan = errors.New("owned-container cleanup plan is stale")

func Preview(records []Record) (Plan, error) {
	expectations := make([]Expectation, 0, len(records))
	for _, record := range records {
		digest, err := fingerprint(record)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Plan{}, err
		}
		expectations = append(expectations, Expectation{Record: record, digest: digest})
	}
	sort.Slice(expectations, func(i, j int) bool { return expectations[i].Record.Path < expectations[j].Record.Path })
	plan := Plan{expectations: expectations}
	plan.seal = plan.digest()
	return plan, nil
}

func (p Plan) Records() []Record {
	out := make([]Record, 0, len(p.expectations))
	for _, expectation := range p.expectations {
		out = append(out, expectation.Record)
	}
	return out
}

func (p Plan) Verify() error {
	if p.seal == "" || p.seal != p.digest() {
		return fmt.Errorf("%w: seal mismatch", ErrStalePlan)
	}
	for _, expectation := range p.expectations {
		got, err := fingerprint(expectation.Record)
		if err != nil || got != expectation.digest {
			return fmt.Errorf("%w: %s changed after preview", ErrStalePlan, expectation.Record.Path)
		}
	}
	return nil
}

func (p Plan) Cleanup() ([]Record, error) {
	records := p.Records()
	sort.Slice(records, func(i, j int) bool {
		iDepth, jDepth := strings.Count(filepath.Clean(records[i].Path), string(os.PathSeparator)), strings.Count(filepath.Clean(records[j].Path), string(os.PathSeparator))
		if iDepth == jDepth {
			return records[i].Path > records[j].Path
		}
		return iDepth > jDepth
	})
	removed := make([]Record, 0, len(records))
	for _, record := range records {
		before, err := os.Lstat(record.Path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return removed, err
		}
		removable, err := removable(record)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return removed, err
		}
		if !removable {
			continue
		}
		after, err := os.Lstat(record.Path)
		if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || before.ModTime() != after.ModTime() {
			continue
		}
		if err := os.Remove(record.Path); err != nil && !os.IsNotExist(err) {
			return removed, fmt.Errorf("remove Matty-created container %s: %w", record.Path, err)
		}
		removed = append(removed, record)
	}
	return removed, nil
}

func Merge(existing, created []Record) []Record {
	byPath := make(map[string]Record, len(existing)+len(created))
	for _, record := range existing {
		byPath[record.Path] = record
	}
	for _, record := range created {
		byPath[record.Path] = record
	}
	out := make([]Record, 0, len(byPath))
	for _, record := range byPath {
		out = append(out, record)
	}
	sortRecords(out)
	return out
}

func fingerprint(record Record) (string, error) {
	info, err := os.Lstat(record.Path)
	if err != nil {
		return "", err
	}
	if (record.Kind == Directory) != info.IsDir() || (!info.IsDir() && !info.Mode().IsRegular()) {
		return "", fmt.Errorf("%w: %s changed type", ErrStalePlan, record.Path)
	}
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s:%o:%d:", record.Kind, info.Mode().Perm(), info.Size())
	if info.IsDir() {
		entries, err := os.ReadDir(record.Path)
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			_, _ = fmt.Fprintf(h, "%s:%s;", entry.Name(), entry.Type())
		}
	} else {
		data, err := os.ReadFile(record.Path)
		if err != nil {
			return "", err
		}
		_, _ = h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func removable(record Record) (bool, error) {
	if record.Kind == Directory {
		entries, err := os.ReadDir(record.Path)
		return len(entries) == 0, err
	}
	data, err := os.ReadFile(record.Path)
	if err != nil {
		return false, err
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return true, nil
	}
	var value any
	if json.Unmarshal(data, &value) == nil && emptyJSON(value) {
		return true, nil
	}
	return false, nil
}

func emptyJSON(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			if !emptyJSON(child) {
				return false
			}
		}
		return true
	case []any:
		return len(typed) == 0
	case nil:
		return true
	default:
		return false
	}
}

func (p Plan) digest() string {
	type sealedExpectation struct {
		Record Record `json:"record"`
		Digest string `json:"digest"`
	}
	sealed := make([]sealedExpectation, 0, len(p.expectations))
	for _, expectation := range p.expectations {
		sealed = append(sealed, sealedExpectation{Record: expectation.Record, Digest: expectation.digest})
	}
	payload, _ := json.Marshal(sealed)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func sortRecords(records []Record) {
	sort.Slice(records, func(i, j int) bool { return records[i].Path < records[j].Path })
}
