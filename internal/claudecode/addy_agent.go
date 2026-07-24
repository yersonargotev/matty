package claudecode

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

type addyAgentSource struct {
	Name, Description string
	Body              []byte
}

func decodeAddyAgentSource(source []byte) (addyAgentSource, error) {
	if !utf8.Valid(source) {
		return addyAgentSource{}, errors.New("Addy agent source is not valid UTF-8")
	}
	if !bytes.HasPrefix(source, []byte("---\n")) {
		return addyAgentSource{}, errors.New("Addy agent source must start with frontmatter")
	}
	end := bytes.Index(source[4:], []byte("\n---\n"))
	if end < 0 {
		return addyAgentSource{}, errors.New("Addy agent source has unterminated frontmatter")
	}
	end += 4
	header := source[4:end]
	body := append([]byte(nil), source[end+5:]...)
	values := map[string]string{}
	for _, line := range strings.Split(string(header), "\n") {
		key, raw, ok := strings.Cut(line, ":")
		if !ok || key == "" || strings.TrimSpace(key) != key || strings.TrimSpace(raw) == "" {
			return addyAgentSource{}, fmt.Errorf("malformed Addy agent frontmatter line %q", line)
		}
		if key != "name" && key != "description" {
			return addyAgentSource{}, fmt.Errorf("unknown Addy agent frontmatter key %q", key)
		}
		if _, duplicate := values[key]; duplicate {
			return addyAgentSource{}, fmt.Errorf("duplicate Addy agent frontmatter key %q", key)
		}
		value, err := decodeAddyAgentScalar(strings.TrimSpace(raw))
		if err != nil {
			return addyAgentSource{}, fmt.Errorf("decode Addy agent %s: %w", key, err)
		}
		values[key] = value
	}
	if values["name"] == "" || values["description"] == "" || len(values) != 2 {
		return addyAgentSource{}, errors.New("Addy agent frontmatter requires exactly name and description")
	}
	return addyAgentSource{Name: values["name"], Description: values["description"], Body: body}, nil
}

func decodeAddyAgentScalar(raw string) (string, error) {
	if strings.HasPrefix(raw, `"`) {
		var value string
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return "", err
		}
		if value == "" {
			return "", errors.New("empty value")
		}
		return value, nil
	}
	if strings.ContainsAny(raw, "[]{}&*!|>'\"#\t\r") || strings.TrimSpace(raw) != raw {
		return "", errors.New("unsupported YAML scalar")
	}
	return raw, nil
}

func renderAddyClaudeAgent(pack capabilitypack.Pack, resource capabilitypack.Resource, binding capabilitypack.Binding, source []byte) ([]byte, error) {
	decoded, err := decodeAddyAgentSource(source)
	if err != nil {
		return nil, fmt.Errorf("decode Addy agent %s: %w", resource.ID, err)
	}
	if decoded.Name != resource.ID {
		return nil, fmt.Errorf("Addy agent source name %q does not match portable name %q", decoded.Name, resource.ID)
	}
	if binding.AgentAuthority == nil {
		return nil, fmt.Errorf("Claude agent %s is missing explicit authority translations", resource.ID)
	}
	dependency, err := addyClaudeAgentDependency(pack, resource)
	if err != nil {
		return nil, err
	}
	authority := *binding.AgentAuthority
	if authority.PermissionMode != "default" {
		return nil, fmt.Errorf("Addy Claude agent %s requires permissionMode default", resource.ID)
	}
	toolSet := map[string]bool{}
	records := append([]capabilitypack.AuthorityRecord(nil), authority.Authorities...)
	sort.Slice(records, func(i, j int) bool { return records[i].Portable < records[j].Portable })
	contract := make([]string, 0, len(records))
	for _, record := range records {
		for _, tool := range record.ClaudeTools {
			toolSet[tool] = true
		}
		declarations, tools := strings.Join(record.Declarations, ", "), strings.Join(record.ClaudeTools, ", ")
		if declarations == "" {
			declarations = "none"
		}
		if tools == "" {
			tools = "none"
		}
		contract = append(contract, fmt.Sprintf("- %s: declarations=[%s]; outcome=%s; claude_tools=[%s]; fallback=%s", record.Portable, declarations, record.Outcome, tools, record.Fallback))
	}
	tools := make([]string, 0, len(toolSet))
	for tool := range toolSet {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	var out bytes.Buffer
	fmt.Fprintf(&out, "---\nname: %s\ndescription: %s\npermissionMode: default\ntools: %s\nskills:\n  - %s\n---\n\n## Packy authority contract\n\n- permission_mode: default\n%s\n", binding.Name, yamlScalar(decoded.Description), strings.Join(tools, ", "), dependency, strings.Join(contract, "\n"))
	out.Write(decoded.Body)
	return out.Bytes(), nil
}

func addyClaudeAgentDependency(pack capabilitypack.Pack, agent capabilitypack.Resource) (string, error) {
	count := 0
	for _, requirement := range agent.Requires {
		if requirement == "skill:using-agent-skills" {
			count++
		}
	}
	if count != 1 {
		return "", fmt.Errorf("Addy agent %s must require skill:using-agent-skills exactly once", agent.ID)
	}
	for _, resource := range pack.Resources {
		if resource.Kind != "skill" || resource.ID != "using-agent-skills" {
			continue
		}
		for _, binding := range resource.Bindings {
			if binding.Surface == capabilitypack.SurfaceClaude && binding.Projection == "skill" {
				if binding.Name == "" {
					break
				}
				return binding.Name, nil
			}
		}
	}
	return "", errors.New("Addy agent dependency skill:using-agent-skills has no effective Claude skill binding")
}
