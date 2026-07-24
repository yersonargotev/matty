package claudesmoke

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"
)

var exactReleaseTagPattern = regexp.MustCompile(`^v0\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// AddyQualification is the package-installed boundary proof consumed by the
// Addy promotion gate. Synthetic qualifications exercise the harness but are
// deliberately inadmissible as production evidence.
type AddyQualification struct {
	SchemaVersion         int                `json:"schema_version"`
	Synthetic             bool               `json:"synthetic"`
	Repository            string             `json:"repository"`
	Workflow              string             `json:"workflow"`
	WorkflowDigest        string             `json:"workflow_digest"`
	RunID                 string             `json:"run_id"`
	Commit                string             `json:"commit,omitempty"`
	Tag                   string             `json:"tag,omitempty"`
	Checkout              string             `json:"checkout"`
	PackyExecutable       string             `json:"packy_executable"`
	PackyExecutableDigest string             `json:"packy_executable_sha256"`
	InstalledSource       string             `json:"installed_source"`
	InstalledSourceCommit string             `json:"installed_source_commit"`
	InstalledSourceClean  bool               `json:"installed_source_clean"`
	Sandbox               string             `json:"sandbox"`
	WritableRoots         AddyWritableRoots  `json:"writable_roots"`
	ProcessLogDigest      string             `json:"process_log_sha256"`
	CollectedAt           string             `json:"collected_at"`
	Safety                AddyObservedSafety `json:"observed_safety"`
	Smoke                 Evidence           `json:"smoke"`
}

// AddyQualificationObservation contains only facts measured by Run while its
// disposable sandbox still exists.
type AddyQualificationObservation struct {
	InstalledSource       string             `json:"installed_source"`
	InstalledSourceCommit string             `json:"installed_source_commit"`
	InstalledSourceClean  bool               `json:"installed_source_clean"`
	WritableRoots         AddyWritableRoots  `json:"writable_roots"`
	ProcessLogDigest      string             `json:"process_log_sha256"`
	CollectedAt           string             `json:"collected_at"`
	Safety                AddyObservedSafety `json:"observed_safety"`
}

type AddyObservedSafety struct {
	NoGoRun           bool `json:"no_go_run"`
	NoDevelopmentPath bool `json:"no_development_path"`
	NoDirectFixture   bool `json:"no_direct_fixture_access"`
	NoUntrackedInput  bool `json:"no_untracked_input"`
	NoAuthentication  bool `json:"no_authentication"`
	NoModelInvocation bool `json:"no_model_invocation"`
	NoPrint           bool `json:"no_print"`
	NoREPL            bool `json:"no_repl"`
	NoUpstreamExecute bool `json:"no_upstream_execution"`
	NoCredentials     bool `json:"no_credentials"`
	NoOutsideWrite    bool `json:"no_outside_write"`
}

type AddyWritableRoots struct {
	Home         string `json:"home"`
	XDGConfig    string `json:"xdg_config_home"`
	ClaudeConfig string `json:"claude_config_dir"`
	State        string `json:"state"`
	Package      string `json:"package"`
	Repository   string `json:"repository"`
	Acquisition  string `json:"acquisition"`
}

func (r AddyWritableRoots) all() []string {
	return []string{r.Home, r.XDGConfig, r.ClaudeConfig, r.State, r.Package, r.Repository, r.Acquisition}
}

// ValidateAddyQualification validates a harness result. It accepts a synthetic
// pre-candidate result so the harness can be qualified without weakening the
// production admission boundary.
func ValidateAddyQualification(q AddyQualification) error {
	if q.SchemaVersion != 1 || q.Repository == "" || q.Workflow == "" || q.RunID == "" {
		return errors.New("missing Addy qualification identity")
	}
	collectedAt, err := time.Parse(time.RFC3339Nano, q.CollectedAt)
	if err != nil || collectedAt.Location() != time.UTC || collectedAt.Format(time.RFC3339Nano) != q.CollectedAt {
		return errors.New("collection timestamp must be canonical UTC RFC3339")
	}
	for _, digest := range []struct{ name, value string }{
		{"workflow", q.WorkflowDigest},
		{"Packy executable", q.PackyExecutableDigest},
		{"process log", q.ProcessLogDigest},
	} {
		if !validSHA256(digest.value) {
			return fmt.Errorf("malformed %s digest", digest.name)
		}
	}
	if !cleanAbsolute(q.Checkout) || !cleanAbsolute(q.PackyExecutable) || !cleanAbsolute(q.InstalledSource) || !cleanAbsolute(q.Sandbox) {
		return errors.New("qualification paths must be clean absolute paths")
	}
	if pathWithin(q.Checkout, q.Sandbox) {
		return errors.New("qualification sandbox must be outside the checkout")
	}
	if pathWithin(q.Checkout, q.PackyExecutable) || pathWithin(q.Checkout, q.InstalledSource) {
		return errors.New("Packy binary and Installed Source must be outside the checkout")
	}
	if !pathWithin(q.Sandbox, q.InstalledSource) {
		return errors.New("Installed Source must be inside the disposable sandbox")
	}
	if !q.InstalledSourceClean || !validCommit(q.InstalledSourceCommit) {
		return errors.New("Installed Source must be an exact clean commit")
	}
	if q.Commit != "" && (!validCommit(q.Commit) || q.InstalledSourceCommit != q.Commit) {
		return errors.New("Installed Source does not match the candidate commit")
	}
	if q.Tag != "" && (q.Commit == "" || !exactReleaseTagPattern.MatchString(q.Tag)) {
		return errors.New("tag qualification requires an exact v0.x.y tag and commit")
	}
	for _, root := range q.WritableRoots.all() {
		if !cleanAbsolute(root) || root == q.Sandbox || !pathWithin(q.Sandbox, root) {
			return errors.New("all writable roots must be beneath one disposable sandbox")
		}
	}
	s := q.Safety
	if !s.NoGoRun || !s.NoDevelopmentPath || !s.NoDirectFixture || !s.NoUntrackedInput ||
		!s.NoAuthentication || !s.NoModelInvocation || !s.NoPrint || !s.NoREPL ||
		!s.NoUpstreamExecute || !s.NoCredentials || !s.NoOutsideWrite {
		return errors.New("required safety fact was not observed")
	}
	if err := ValidateEvidence(q.Smoke); err != nil {
		return fmt.Errorf("invalid bound smoke evidence: %w", err)
	}
	if q.Smoke.PackySHA != q.InstalledSourceCommit || q.Smoke.Sandbox != q.Sandbox {
		return errors.New("smoke evidence is not bound to candidate and sandbox")
	}
	bound := q.Smoke.Qualification
	if q.InstalledSource != bound.InstalledSource ||
		q.InstalledSourceCommit != bound.InstalledSourceCommit ||
		q.InstalledSourceClean != bound.InstalledSourceClean ||
		q.ProcessLogDigest != bound.ProcessLogDigest ||
		q.CollectedAt != bound.CollectedAt ||
		!reflect.DeepEqual(q.WritableRoots, bound.WritableRoots) ||
		!reflect.DeepEqual(q.Safety, bound.Safety) {
		return errors.New("qualification does not match the in-sandbox observation")
	}
	processLog, err := json.Marshal(q.Smoke.Commands)
	if err != nil {
		return fmt.Errorf("encode smoke process log: %w", err)
	}
	sum := sha256.Sum256(processLog)
	if q.ProcessLogDigest != hex.EncodeToString(sum[:]) {
		return errors.New("process log digest does not match the exact smoke commands")
	}
	return nil
}

// BindAddyQualification copies the runner's observations into the admission
// document. Callers cannot manufacture safety claims after Run has removed the
// disposable sandbox.
func BindAddyQualification(q AddyQualification, e Evidence) (AddyQualification, error) {
	o := e.Qualification
	if o.InstalledSource == "" || o.ProcessLogDigest == "" {
		return AddyQualification{}, errors.New("smoke evidence has no in-sandbox qualification observation")
	}
	q.InstalledSource = o.InstalledSource
	q.InstalledSourceCommit = o.InstalledSourceCommit
	q.InstalledSourceClean = o.InstalledSourceClean
	q.Sandbox = e.Sandbox
	q.WritableRoots = o.WritableRoots
	q.ProcessLogDigest = o.ProcessLogDigest
	q.CollectedAt = o.CollectedAt
	q.Safety = o.Safety
	q.Smoke = e
	return q, nil
}

// ValidateProductionAddyQualification rejects synthetic/pre-candidate proofs
// and requires an exact commit or exact tag identity.
func ValidateProductionAddyQualification(q AddyQualification) error {
	if err := ValidateAddyQualification(q); err != nil {
		return err
	}
	if q.Synthetic {
		return errors.New("synthetic Addy qualification is not production-admissible")
	}
	collectedAt, _ := time.Parse(time.RFC3339Nano, q.CollectedAt)
	if age := time.Since(collectedAt); age < -5*time.Minute || age > 24*time.Hour {
		return errors.New("Addy qualification evidence is stale or future-dated")
	}
	if q.Commit == "" || (q.Tag == "" && q.Commit != q.Smoke.PackyRef) {
		return errors.New("production qualification must bind an exact commit or tag")
	}
	return nil
}

// CanonicalAddyQualificationJSON emits deterministic field-ordered evidence.
func CanonicalAddyQualificationJSON(q AddyQualification) ([]byte, error) {
	if err := ValidateAddyQualification(q); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(q, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}
func validCommit(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 20 && value == strings.ToLower(value)
}
func cleanAbsolute(value string) bool {
	return value != "" && filepath.IsAbs(value) && filepath.Clean(value) == value
}
