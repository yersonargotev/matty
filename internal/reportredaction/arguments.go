package reportredaction

import "strings"

type redactedError struct {
	message string
	cause   error
}

type environmentArgument struct {
	index  int
	prefix string
	value  string
}

func (err redactedError) Error() string { return err.message }
func (err redactedError) Unwrap() error { return err.cause }

// EnvironmentArguments returns a report-safe copy of command arguments.
// Values passed through the conventional --env/-e forms are never disclosed.
func EnvironmentArguments(args []string) []string {
	result := append([]string(nil), args...)
	for _, argument := range environmentArguments(args) {
		result[argument.index] = argument.prefix + environmentValue(argument.value)
	}
	return result
}

func environmentValue(value string) string {
	if key, _, ok := strings.Cut(value, "="); ok {
		return key + "=<redacted>"
	}
	return "<redacted>"
}

// Error returns an error whose printable form excludes environment argument
// values and other sealed payloads while preserving errors.Is/As behavior.
func Error(err error, argumentSets [][]string, sealedPayloads []string) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	for _, args := range argumentSets {
		for _, argument := range environmentArguments(args) {
			message = strings.ReplaceAll(message, argument.value, environmentValue(argument.value))
			if _, value, ok := strings.Cut(argument.value, "="); ok && value != "" {
				message = strings.ReplaceAll(message, value, "<redacted>")
			}
		}
	}
	for _, payload := range sealedPayloads {
		if payload != "" {
			message = strings.ReplaceAll(message, payload, "<redacted>")
		}
	}
	return redactedError{message: message, cause: err}
}

func environmentArguments(args []string) []environmentArgument {
	values := make([]environmentArgument, 0)
	for i := range args {
		if args[i] == "--env" || args[i] == "-e" {
			if i+1 < len(args) {
				values = append(values, environmentArgument{index: i + 1, value: args[i+1]})
				i++
			}
			continue
		}
		for _, prefix := range []string{"--env=", "-e="} {
			if strings.HasPrefix(args[i], prefix) {
				values = append(values, environmentArgument{index: i, prefix: prefix, value: strings.TrimPrefix(args[i], prefix)})
			}
		}
	}
	return values
}
