package cli

// EngramInstalled checks whether the engram binary is available through the
// injected runner. Missing or otherwise unresolvable binaries both mean install
// should include the Homebrew formula step; command execution will surface any
// actionable Homebrew failure later.
func EngramInstalled(runner Runner) bool {
	_, err := runner.LookPath("engram")
	return err == nil
}
