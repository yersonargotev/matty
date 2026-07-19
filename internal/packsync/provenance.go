package packsync

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
)

var canonicalSourceIDPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var canonicalSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// SourceLockDigest identifies the exact canonical bytes of one source lock.
type SourceLockDigest struct {
	SourceID string
	SHA256   string
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
