package claudesmoke

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var exactReleaseTagPattern = regexp.MustCompile(`^v0\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// AddyQualification is the package-installed boundary proof consumed by the
// Addy promotion gate. Synthetic qualifications exercise the harness but are
// deliberately inadmissible as production evidence.
type AddyQualification struct {
	SchemaVersion         int               `json:"schema_version"`
	Synthetic             bool              `json:"synthetic"`
	Repository            string            `json:"repository"`
	Workflow              string            `json:"workflow"`
	WorkflowDigest        string            `json:"workflow_digest"`
	RunID                 string            `json:"run_id"`
	Commit                string            `json:"commit,omitempty"`
	Tag                   string            `json:"tag,omitempty"`
	Checkout              string            `json:"checkout"`
	PackyExecutable       string            `json:"packy_executable"`
	PackyExecutableDigest string            `json:"packy_executable_sha256"`
	InstalledSource       string            `json:"installed_source"`
	InstalledSourceCommit string            `json:"installed_source_commit"`
	InstalledSourceClean  bool              `json:"installed_source_clean"`
	Sandbox               string            `json:"sandbox"`
	WritableRoots         AddyWritableRoots `json:"writable_roots"`
	ProcessLogDigest      string            `json:"process_log_sha256"`
	UsedGoRun             bool              `json:"used_go_run"`
	UsedDevelopmentPath   bool              `json:"used_development_path"`
	UsedDirectFixture     bool              `json:"used_direct_fixture"`
	UsedUntrackedInput    bool              `json:"used_untracked_input"`
	Authenticated         bool              `json:"authenticated"`
	ModelInvoked          bool              `json:"model_invoked"`
	PrintInvoked          bool              `json:"print_invoked"`
	REPLInvoked           bool              `json:"repl_invoked"`
	UpstreamExecuted      bool              `json:"upstream_executed"`
	CredentialsObserved   bool              `json:"credentials_observed"`
	OutsideWriteObserved  bool              `json:"outside_write_observed"`
	Smoke                 Evidence          `json:"smoke"`
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
	seenRoots := map[string]bool{}
	for _, root := range q.WritableRoots.all() {
		if !cleanAbsolute(root) || root == q.Sandbox || !pathWithin(q.Sandbox, root) {
			return errors.New("all writable roots must be beneath one disposable sandbox")
		}
		if seenRoots[root] {
			return errors.New("writable roots must be distinct")
		}
		seenRoots[root] = true
	}
	if q.UsedGoRun || q.UsedDevelopmentPath || q.UsedDirectFixture || q.UsedUntrackedInput {
		return errors.New("development or untracked inputs are not qualification evidence")
	}
	if q.Authenticated || q.ModelInvoked || q.PrintInvoked || q.REPLInvoked || q.UpstreamExecuted || q.CredentialsObserved || q.OutsideWriteObserved {
		return errors.New("unsafe Claude smoke activity")
	}
	if err := ValidateEvidence(q.Smoke); err != nil {
		return fmt.Errorf("invalid bound smoke evidence: %w", err)
	}
	if q.Smoke.PackySHA != q.InstalledSourceCommit || q.Smoke.Sandbox != q.Sandbox {
		return errors.New("smoke evidence is not bound to candidate and sandbox")
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

// ValidateProductionAddyQualification rejects synthetic/pre-candidate proofs
// and requires an exact commit or exact tag identity.
func ValidateProductionAddyQualification(q AddyQualification) error {
	if err := ValidateAddyQualification(q); err != nil {
		return err
	}
	if q.Synthetic {
		return errors.New("synthetic Addy qualification is not production-admissible")
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
