package claudecode

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const MinimumSupportedVersion = "2.1.203"

type Command struct {
	Executable  string
	Args        []string
	Env         []string
	Timeout     time.Duration
	Description string
}

// String deliberately returns only the pre-redacted stable description.
func (c Command) String() string { return c.Description }

type Result struct {
	Stdout, Stderr string
	ExitCode       int
	Err            error
	TimedOut       bool
}
type Runner interface {
	Run(context.Context, Command) Result
}
type LookPath func(string) (string, error)

type VersionObservation struct {
	Executable        string
	Version           string
	Output            string
	Missing, TimedOut bool
	Err               error
}

type Compatibility string

const (
	CompatibilitySupported  Compatibility = "supported"
	CompatibilityMissing    Compatibility = "missing"
	CompatibilityBelowFloor Compatibility = "below-floor"
	CompatibilityPrerelease Compatibility = "prerelease"
	CompatibilityUnreadable Compatibility = "unreadable"
	CompatibilityFailed     Compatibility = "failed"
	CompatibilityTimedOut   Compatibility = "timed-out"
)

var semverPattern = regexp.MustCompile(`(?i)(?:claude(?: code)?\s+)?v?(\d+)\.(\d+)\.(\d+)([-+][0-9a-z.-]+)?`)

func ObserveVersion(ctx context.Context, executable string, runner Runner) VersionObservation {
	if strings.TrimSpace(executable) == "" {
		return VersionObservation{Missing: true}
	}
	if runner == nil {
		return VersionObservation{Executable: executable, Err: errors.New("Claude Code version runner is unavailable")}
	}
	result := runner.Run(ctx, Command{Executable: executable, Args: []string{"--version"}, Timeout: 5 * time.Second, Description: "inspect Claude Code version"})
	o := VersionObservation{Executable: executable, Output: strings.TrimSpace(result.Stdout), TimedOut: result.TimedOut, Err: result.Err}
	if o.Output == "" {
		o.Output = strings.TrimSpace(result.Stderr)
	}
	if m := semverPattern.FindStringSubmatch(o.Output); m != nil {
		o.Version = m[1] + "." + m[2] + "." + m[3] + m[4]
	}
	if result.ExitCode != 0 && o.Err == nil {
		o.Err = fmt.Errorf("Claude Code version command exited with status %d", result.ExitCode)
	}
	return o
}

func ClassifyVersion(o VersionObservation) Compatibility {
	if o.Missing || o.Executable == "" {
		return CompatibilityMissing
	}
	if o.TimedOut || errors.Is(o.Err, context.DeadlineExceeded) {
		return CompatibilityTimedOut
	}
	if o.Err != nil {
		return CompatibilityFailed
	}
	m := semverPattern.FindStringSubmatch(o.Version)
	if m == nil {
		return CompatibilityUnreadable
	}
	if m[4] != "" && strings.HasPrefix(m[4], "-") {
		return CompatibilityPrerelease
	}
	want := semverPattern.FindStringSubmatch(MinimumSupportedVersion)
	for i := 1; i <= 3; i++ {
		a, _ := strconv.Atoi(m[i])
		b, _ := strconv.Atoi(want[i])
		if a < b {
			return CompatibilityBelowFloor
		}
		if a > b {
			return CompatibilitySupported
		}
	}
	return CompatibilitySupported
}

func (c Compatibility) Remediation() string {
	switch c {
	case CompatibilitySupported:
		return ""
	case CompatibilityMissing:
		return "install Claude Code " + MinimumSupportedVersion + " or newer"
	default:
		return "install a stable Claude Code " + MinimumSupportedVersion + " or newer and retry"
	}
}
