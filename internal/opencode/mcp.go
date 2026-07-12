package opencode

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// MCPInspection describes only the requested server entry. Credentials and
// tool-owned data are intentionally never read.
type MCPInspection struct {
	Exists              bool
	ObservedFingerprint string
	DesiredFingerprint  string
}

func InspectMCPProjection(configPath, id, command string, args []string) (MCPInspection, error) {
	existing, err := readOptionalFile(configPath)
	if err != nil {
		return MCPInspection{}, err
	}
	return InspectMCPContent(existing, configPath, id, command, args)
}

func InspectMCPContent(content, configPath, id, command string, args []string) (MCPInspection, error) {
	want := mcpServerValue(command, args)
	desired := fingerprintMCPValue(want)
	if strings.TrimSpace(content) == "" {
		return MCPInspection{DesiredFingerprint: desired}, nil
	}
	config, err := decodeConfig(content, configPath)
	if err != nil {
		return MCPInspection{}, err
	}
	mcp, ok := config["mcp"]
	if !ok {
		return MCPInspection{DesiredFingerprint: desired}, nil
	}
	servers, ok := mcp.(map[string]any)
	if !ok {
		return MCPInspection{}, fmt.Errorf("read OpenCode config %s: mcp must be an object", configPath)
	}
	value, ok := servers[id]
	if !ok {
		return MCPInspection{DesiredFingerprint: desired}, nil
	}
	return MCPInspection{Exists: true, ObservedFingerprint: fingerprintMCPValue(value), DesiredFingerprint: desired}, nil
}

// MergeMCPProjection adds one local MCP definition while preserving the
// surrounding JSONC text, comments, and unrelated host configuration.
func MergeMCPProjection(existing, configPath, id, command string, args []string) (string, error) {
	server := mcpServerValue(command, args)
	if strings.TrimSpace(existing) == "" {
		return "{\n  \"mcp\": {\n    \"" + id + "\": " + compactJSON(server) + "\n  }\n}\n", nil
	}
	config, err := decodeConfig(existing, configPath)
	if err != nil {
		return "", err
	}
	if value, ok := config["mcp"]; ok {
		servers, ok := value.(map[string]any)
		if !ok {
			return "", fmt.Errorf("read OpenCode config %s: mcp must be an object", configPath)
		}
		if current, exists := servers[id]; exists {
			if fingerprintMCPValue(current) != fingerprintMCPValue(server) {
				return "", fmt.Errorf("OpenCode MCP server %q already exists with unmanaged settings", id)
			}
			return existing, nil
		}
		property, found, err := findTopLevelProperty(existing, "mcp")
		if err != nil {
			return "", err
		}
		if !found {
			return "", fmt.Errorf("read OpenCode config %s: could not locate mcp property", configPath)
		}
		objectStart := skipWhitespaceAndComments(existing, property.valueStart)
		objectClose, err := objectCloseAt(existing, objectStart)
		if err != nil {
			return "", err
		}
		return insertObjectProperty(existing, objectClose, property.indent+"  ", id, compactJSON(server), len(servers) > 0), nil
	}

	close, err := rootObjectClose(existing)
	if err != nil {
		return "", err
	}
	return insertObjectProperty(existing, close, inferPropertyIndent(existing, close), "mcp", "{\n    \""+id+"\": "+compactJSON(server)+"\n  }", len(config) > 0), nil
}

// RemoveMCPProjection removes one server member while preserving the
// surrounding JSONC text, comments, and unrelated host configuration.
func RemoveMCPProjection(existing, configPath, id string) (string, error) {
	if strings.TrimSpace(existing) == "" {
		return existing, nil
	}
	config, err := decodeConfig(existing, configPath)
	if err != nil {
		return "", err
	}
	value, ok := config["mcp"]
	if !ok {
		return existing, nil
	}
	servers, ok := value.(map[string]any)
	if !ok {
		return "", fmt.Errorf("read OpenCode config %s: mcp must be an object", configPath)
	}
	if _, ok := servers[id]; !ok {
		return existing, nil
	}
	mcp, found, err := findTopLevelProperty(existing, "mcp")
	if err != nil || !found {
		return "", err
	}
	objectStart := skipWhitespaceAndComments(existing, mcp.valueStart)
	objectEnd, err := endJSONValue(existing, objectStart)
	if err != nil {
		return "", err
	}
	object := existing[objectStart:objectEnd]
	member, found, err := findTopLevelProperty(object, id)
	if err != nil || !found {
		return "", err
	}
	member.propertyStart += objectStart
	member.propertyEnd += objectStart
	member.valueStart += objectStart
	member.valueEnd += objectStart
	return removeProperty(existing, member), nil
}

func ValidateMCPProjection(content, configPath, id, command string, args []string) error {
	inspection, err := InspectMCPContent(content, configPath, id, command, args)
	if err != nil {
		return err
	}
	if !inspection.Exists || inspection.ObservedFingerprint != inspection.DesiredFingerprint {
		return fmt.Errorf("OpenCode MCP projection is missing or differs for %q", id)
	}
	return nil
}

func decodeConfig(content, configPath string) (map[string]any, error) {
	data, err := jsoncToJSON(content)
	if err != nil {
		return nil, fmt.Errorf("read OpenCode config %s: invalid JSONC: %w", configPath, err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("read OpenCode config %s: invalid JSONC: %w", configPath, err)
	}
	return config, nil
}

func mcpServerValue(command string, args []string) map[string]any {
	commandArgs := make([]any, 0, len(args)+1)
	commandArgs = append(commandArgs, command)
	for _, arg := range args {
		commandArgs = append(commandArgs, arg)
	}
	return map[string]any{"type": "local", "command": commandArgs, "enabled": true}
}

func fingerprintMCPValue(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func compactJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func objectCloseAt(content string, start int) (int, error) {
	if start >= len(content) || content[start] != '{' {
		return 0, fmt.Errorf("JSONC object does not start with an object")
	}
	depth := 0
	close := -1
	err := walkJSONC(content, start, func(event jsoncEvent) (bool, error) {
		switch event.ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				close = event.pos
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return 0, err
	}
	if close < 0 {
		return 0, fmt.Errorf("JSONC object is not closed")
	}
	return close, nil
}

func insertObjectProperty(content string, objectClose int, indent, key, value string, hasProperties bool) string {
	prefix := strings.TrimRight(content[:objectClose], " \t\r\n")
	if hasProperties {
		prefix = ensureTrailingComma(prefix)
	}
	suffix := content[objectClose:]
	return prefix + "\n" + indent + "\"" + key + "\": " + value + "\n" + suffix
}
