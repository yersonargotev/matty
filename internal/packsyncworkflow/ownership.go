package packsyncworkflow

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const publicationMarker = "<!-- matty-pack-sync:"

func ManagedBody(prefix string, record PublicationRecord) (string, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	return prefix + "\n" + publicationMarker + base64.RawURLEncoding.EncodeToString(data) + " -->\n", nil
}

func ParsePublicationRecord(body string) (PublicationRecord, bool) {
	start := strings.LastIndex(body, publicationMarker)
	if start < 0 {
		return PublicationRecord{}, false
	}
	encoded := body[start+len(publicationMarker):]
	encoded, _, ok := strings.Cut(encoded, " -->")
	if !ok {
		return PublicationRecord{}, false
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return PublicationRecord{}, false
	}
	var record PublicationRecord
	if json.Unmarshal(data, &record) != nil {
		return PublicationRecord{}, false
	}
	return record, true
}

func ManagedMetadataHash(title, body string) string {
	prefix := body
	if index := strings.LastIndex(body, publicationMarker); index >= 0 {
		prefix = strings.TrimSuffix(body[:index], "\n")
	}
	return HashText(title + "\x00" + prefix)
}

func HashText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func NewPublicationRecord(proposal Proposal, head, metadataHash string) PublicationRecord {
	return PublicationRecord{PlanID: proposal.PlanID, BaseSHA: proposal.BaseSHA, CandidateSHA: proposal.CandidateSHA, HeadSHA: head, ProvenanceSHA256: proposal.ProvenanceSHA256, MetadataHash: metadataHash}
}
