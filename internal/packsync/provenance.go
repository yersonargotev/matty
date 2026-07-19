package packsync

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var canonicalSourceIDPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var canonicalSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// SourceLockDigest identifies the exact canonical bytes of one source lock.
type SourceLockDigest struct {
	SourceID string
	SHA256   string
}

type sourceLockSet struct {
	Locks         map[string]Lock
	Digests       map[string]string
	LockSetSHA256 string
}

func (set sourceLockSet) withTarget(sourceID, digest string) (string, error) {
	digests := make([]SourceLockDigest, 0, len(set.Digests)+1)
	for id, sha := range set.Digests {
		if id != sourceID {
			digests = append(digests, SourceLockDigest{SourceID: id, SHA256: sha})
		}
	}
	digests = append(digests, SourceLockDigest{SourceID: sourceID, SHA256: digest})
	return LockSetSHA256(digests)
}

func loadSourceLockSet(bundleRoot string, config Config) (sourceLockSet, error) {
	return loadSourceLockSetForTarget(bundleRoot, config, "", false)
}

func loadSourceLockSetForTarget(bundleRoot string, config Config, relaxedTarget string, allowMissing bool) (sourceLockSet, error) {
	if _, err := os.Lstat(filepath.Join(bundleRoot, "sources.lock.json")); err == nil {
		return sourceLockSet{}, errors.New("mixed provenance topology: legacy bundle/sources.lock.json is forbidden")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return sourceLockSet{}, err
	}
	wanted := make(map[string]bool, len(config.Sources))
	sources := make(map[string]SourceConfig, len(config.Sources))
	for _, source := range config.Sources {
		wanted[source.ID] = true
		sources[source.ID] = source
	}
	entries, err := os.ReadDir(filepath.Join(bundleRoot, "sources"))
	if errors.Is(err, fs.ErrNotExist) && allowMissing {
		entries, err = nil, nil
	}
	if err != nil {
		return sourceLockSet{}, fmt.Errorf("read source locks: %w", err)
	}
	set := sourceLockSet{Locks: map[string]Lock{}, Digests: map[string]string{}}
	digests := make([]SourceLockDigest, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lock.json") {
			return sourceLockSet{}, fmt.Errorf("unexpected source-lock entry %q", entry.Name())
		}
		sourceID := strings.TrimSuffix(entry.Name(), ".lock.json")
		if !canonicalSourceIDPattern.MatchString(sourceID) || !wanted[sourceID] {
			return sourceLockSet{}, fmt.Errorf("orphaned or path-unsafe source lock %q", entry.Name())
		}
		data, err := os.ReadFile(filepath.Join(bundleRoot, "sources", entry.Name()))
		if err != nil {
			return sourceLockSet{}, err
		}
		var lock Lock
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&lock); err != nil {
			return sourceLockSet{}, fmt.Errorf("decode source lock %s: %w", sourceID, err)
		}
		if err := ensureEOF(decoder); err != nil {
			return sourceLockSet{}, err
		}
		if lock.SourceID != sourceID {
			return sourceLockSet{}, fmt.Errorf("source lock %s contains source id %q", sourceID, lock.SourceID)
		}
		if sourceID != relaxedTarget {
			configured := sources[sourceID].Resources
			if len(lock.Resources) != len(configured) {
				return sourceLockSet{}, fmt.Errorf("source lock %s contribution is incomplete", sourceID)
			}
			for i := range configured {
				if bindingKey(lock.Resources[i].Binding) != bindingKey(configured[i]) || lock.Resources[i].UpstreamPath != configured[i].UpstreamPath {
					return sourceLockSet{}, fmt.Errorf("source lock %s contribution contradicts configuration", sourceID)
				}
			}
		}
		canonical, digest, err := CanonicalSourceLock(lock)
		if err != nil {
			return sourceLockSet{}, err
		}
		if !bytes.Equal(data, canonical) {
			return sourceLockSet{}, fmt.Errorf("source lock %s is not canonical", sourceID)
		}
		set.Locks[sourceID], set.Digests[sourceID] = lock, digest
		digests = append(digests, SourceLockDigest{SourceID: sourceID, SHA256: digest})
	}
	for sourceID := range wanted {
		if _, exists := set.Locks[sourceID]; !exists {
			if allowMissing && sourceID == relaxedTarget {
				continue
			}
			return sourceLockSet{}, fmt.Errorf("configured source %q has no canonical lock", sourceID)
		}
	}
	set.LockSetSHA256, err = LockSetSHA256(digests)
	return set, err
}

// CanonicalSourceLock returns the checked-in source-lock representation and
// its SHA-256. The representation is two-space-indented JSON followed by one
// LF, with resources ordered by portable source-binding identity.
func CanonicalSourceLock(lock Lock) ([]byte, string, error) {
	canonical := lock
	canonical.Resources = append([]ResourceEvidence(nil), lock.Resources...)
	sort.Slice(canonical.Resources, func(i, j int) bool {
		return bindingKey(canonical.Resources[i].Binding) < bindingKey(canonical.Resources[j].Binding)
	})
	for i := range canonical.Resources {
		if i > 0 && bindingKey(canonical.Resources[i-1].Binding) == bindingKey(canonical.Resources[i].Binding) {
			return nil, "", fmt.Errorf("duplicate source-lock resource %q", bindingKey(canonical.Resources[i].Binding))
		}
	}
	data, err := json.MarshalIndent(canonical, "", "  ")
	if err != nil {
		return nil, "", err
	}
	data = append(data, '\n')
	digest := sha256.Sum256(data)
	return data, fmt.Sprintf("%x", digest), nil
}

// LockSetSHA256 hashes the complete ordered provenance generation without
// materializing or persisting an aggregate lock index.
func LockSetSHA256(locks []SourceLockDigest) (string, error) {
	ordered := append([]SourceLockDigest(nil), locks...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].SourceID < ordered[j].SourceID })
	hash := sha256.New()
	for i, lock := range ordered {
		if !canonicalSourceIDPattern.MatchString(lock.SourceID) {
			return "", fmt.Errorf("source id %q is not path-safe", lock.SourceID)
		}
		if !canonicalSHA256Pattern.MatchString(lock.SHA256) {
			return "", fmt.Errorf("source %q lock digest is not lowercase SHA-256", lock.SourceID)
		}
		if i > 0 && ordered[i-1].SourceID == lock.SourceID {
			return "", fmt.Errorf("duplicate source id %q", lock.SourceID)
		}
		fmt.Fprintf(hash, "%s\x00%s\n", lock.SourceID, lock.SHA256)
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
