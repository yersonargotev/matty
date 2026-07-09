package opencode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/matty/internal/prompt"
)

// WriteResult reports non-fatal conditions Matty noticed while preserving user
// OpenCode config.
type WriteResult struct {
	Warnings []string
}

type Inspection struct {
	ConfigExists        bool
	PromptExists        bool
	HasMattyInstruction bool
	Warnings            []string
}

func promptContent() string {
	workflow := strings.TrimSpace(`## Matty global workflow
- Global skills live in ~/.agents/skills. When a task matches a skill, read that skill's SKILL.md before acting.
- Use ask-matt at ~/.agents/skills/ask-matt as the router when you are unsure which skill or workflow applies.
- Use Engram memory tools when available: search before past-work or project-sensitive tasks; save decisions, discoveries, bug fixes, and conventions; summarize sessions before finishing.
- Apply host delegation rules when this OpenCode session exposes subagent/delegation tools. If unavailable, proceed inline and mention that delegation was unavailable.`)
	return workflow + "\n\n" + prompt.RulesSectionContent() + "\n"
}
func Write(configPath, promptPath string) (WriteResult, error) {
	existing, err := readOptionalFile(configPath)
	if err != nil {
		return WriteResult{}, err
	}
	result := WriteResult{Warnings: detectExternalManagedConfig(existing)}
	config, err := mergeInstruction(existing, configPath, promptPath)
	if err != nil {
		return WriteResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o700); err != nil {
		return WriteResult{}, fmt.Errorf("create OpenCode config directory %s: %w", filepath.Dir(promptPath), err)
	}
	if err := os.WriteFile(promptPath, []byte(promptContent()), 0o600); err != nil {
		return WriteResult{}, fmt.Errorf("write OpenCode Matty prompt %s: %w", promptPath, err)
	}
	if config != existing {
		if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
			return WriteResult{}, fmt.Errorf("write OpenCode config %s: %w", configPath, err)
		}
	}
	return result, nil
}
func Remove(configPath, promptPath string) error {
	existing, err := readOptionalFile(configPath)
	if err != nil {
		return err
	}
	config, err := removeInstruction(existing, configPath, promptPath)
	if err != nil {
		return err
	}
	if config != existing {
		if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
			return fmt.Errorf("remove OpenCode Matty config %s: %w", configPath, err)
		}
	}
	if err := os.Remove(promptPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove OpenCode Matty prompt %s: %w", promptPath, err)
	}
	return nil
}
func detectExternalManagedConfig(content string) []string {
	if !hasKnownGentleAIOverlay(content) {
		return nil
	}
	return []string{"OpenCode config contains gentle-ai references; Matty preserved them and only updated Matty instruction entries"}
}

func hasKnownGentleAIOverlay(content string) bool {
	if strings.TrimSpace(content) == "" {
		return false
	}
	config := map[string]any{}
	jsonData, err := jsoncToJSON(content)
	if err != nil {
		return false
	}
	if err := json.Unmarshal(jsonData, &config); err != nil {
		return false
	}
	return stringArrayContains(config["plugin"], "gentle-ai") ||
		objectHasKey(config["agent"], "gentle-ai") ||
		objectHasKey(config["profile"], "gentle-ai")
}

func stringArrayContains(value any, needle string) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		text, ok := item.(string)
		if ok && text == needle {
			return true
		}
	}
	return false
}

func objectHasKey(value any, key string) bool {
	items, ok := value.(map[string]any)
	if !ok {
		return false
	}
	if _, ok := items[key]; ok {
		return true
	}
	for _, item := range items {
		nested, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := nested[key]; ok {
			return true
		}
	}
	return false
}
func Inspect(configPath, promptPath string) (Inspection, error) {
	existing, err := readOptionalFile(configPath)
	if err != nil {
		return Inspection{}, err
	}
	inspection := Inspection{ConfigExists: strings.TrimSpace(existing) != "", Warnings: detectExternalManagedConfig(existing)}
	if _, err := os.Stat(promptPath); err == nil {
		inspection.PromptExists = true
	} else if err != nil && !os.IsNotExist(err) {
		return Inspection{}, fmt.Errorf("inspect OpenCode Matty prompt %s: %w", promptPath, err)
	}
	if strings.TrimSpace(existing) == "" {
		return inspection, nil
	}
	config := map[string]any{}
	jsonData, err := jsoncToJSON(existing)
	if err != nil {
		return Inspection{}, fmt.Errorf("read OpenCode config %s: invalid JSONC: %w", configPath, err)
	}
	if err := json.Unmarshal(jsonData, &config); err != nil {
		return Inspection{}, fmt.Errorf("read OpenCode config %s: invalid JSONC: %w", configPath, err)
	}
	instructions, err := instructionStrings(config["instructions"])
	if err != nil {
		return Inspection{}, err
	}
	for _, instruction := range instructions {
		if instruction == promptPath {
			inspection.HasMattyInstruction = true
			break
		}
	}
	return inspection, nil
}
func mergeInstruction(existing, configPath, promptPath string) (string, error) {
	return updateInstructions(existing, configPath, promptPath, instructionMerge)
}
func removeInstruction(existing, configPath, promptPath string) (string, error) {
	return updateInstructions(existing, configPath, promptPath, instructionRemove)
}

type instructionOperation int

const (
	instructionMerge instructionOperation = iota
	instructionRemove
)

func updateInstructions(existing, configPath, promptPath string, operation instructionOperation) (string, error) {
	config := map[string]any{}
	if strings.TrimSpace(existing) != "" {
		jsonData, err := jsoncToJSON(existing)
		if err != nil {
			return "", fmt.Errorf("read OpenCode config %s: invalid JSONC: %w", configPath, err)
		}
		if err := json.Unmarshal(jsonData, &config); err != nil {
			return "", fmt.Errorf("read OpenCode config %s: invalid JSONC: %w", configPath, err)
		}
	}
	instructions, err := instructionStrings(config["instructions"])
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(existing) == "" {
		if operation == instructionRemove {
			return existing, nil
		}
		instructions = append(removeString(instructions, promptPath), promptPath)
		return marshalConfigWithInstructions(config, instructions)
	}
	if operation == instructionMerge {
		return patchInstructionMerge(existing, promptPath)
	}
	return patchInstructionRemove(existing, promptPath)
}
func marshalConfigWithInstructions(config map[string]any, instructions []string) (string, error) {
	if len(instructions) == 0 {
		delete(config, "instructions")
	} else {
		config["instructions"] = instructions
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode OpenCode config: %w", err)
	}
	return string(append(data, '\n')), nil
}
func patchInstructionMerge(existing, promptPath string) (string, error) {
	property, found, err := findTopLevelProperty(existing, "instructions")
	if err != nil {
		return "", err
	}
	if !found {
		return insertInstructionProperty(existing, []string{promptPath})
	}
	elements, close, err := instructionArrayElements(existing, property)
	if err != nil {
		return "", err
	}
	for _, element := range elements {
		if element.value == promptPath {
			return existing, nil
		}
	}
	return insertInstructionArrayEntry(existing, close, property.indent, elements, promptPath), nil
}
func patchInstructionRemove(existing, promptPath string) (string, error) {
	property, found, err := findTopLevelProperty(existing, "instructions")
	if err != nil {
		return "", err
	}
	if !found {
		return existing, nil
	}
	elements, _, err := instructionArrayElements(existing, property)
	if err != nil {
		return "", err
	}
	remaining := 0
	updated := existing
	for i := len(elements) - 1; i >= 0; i-- {
		if elements[i].value == promptPath {
			updated = removeArrayElement(updated, elements[i])
			continue
		}
		remaining++
	}
	if updated == existing {
		return existing, nil
	}
	if remaining == 0 && !arrayValueHasComments(existing[property.valueStart:property.valueEnd]) {
		property, found, err = findTopLevelProperty(updated, "instructions")
		if err != nil {
			return "", err
		}
		if found {
			updated = removeProperty(updated, property)
		}
	}
	return updated, nil
}

type propertyRange struct {
	propertyStart int
	propertyEnd   int
	valueStart    int
	valueEnd      int
	indent        string
}
type arrayStringElement struct {
	value string
	start int
	end   int
}

func findTopLevelProperty(content, name string) (propertyRange, bool, error) {
	open := skipWhitespaceAndComments(content, 0)
	if open >= len(content) {
		return propertyRange{}, false, nil
	}
	if content[open] != '{' {
		return propertyRange{}, false, fmt.Errorf("read OpenCode config: root must be an object")
	}
	var found propertyRange
	matched := false
	depth := 0
	err := walkJSONC(content, open, func(event jsoncEvent) (bool, error) {
		if event.ch == '"' {
			if depth != 1 || event.stringValue != name {
				return false, nil
			}
			colon := skipWhitespaceAndComments(content, event.end)
			if colon >= len(content) || content[colon] != ':' {
				return true, fmt.Errorf("read OpenCode config: property %q missing colon", name)
			}
			valueStart := skipWhitespaceAndComments(content, colon+1)
			valueEnd, err := endJSONValue(content, valueStart)
			if err != nil {
				return true, err
			}
			propertyEnd := valueEnd
			if comma := skipWhitespaceAndComments(content, valueEnd); comma < len(content) && content[comma] == ',' {
				propertyEnd = comma + 1
			}
			found = propertyRange{
				propertyStart: propertyLineStart(content, event.pos),
				propertyEnd:   propertyEnd,
				valueStart:    valueStart,
				valueEnd:      valueEnd,
				indent:        lineIndent(content, event.pos),
			}
			matched = true
			return true, nil
		}
		switch event.ch {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
		return false, nil
	})
	return found, matched, err
}
func instructionArrayElements(content string, property propertyRange) ([]arrayStringElement, int, error) {
	start := skipWhitespaceAndComments(content, property.valueStart)
	if start >= len(content) || content[start] != '[' {
		return nil, 0, fmt.Errorf("read OpenCode config: instructions must be an array of strings")
	}
	var elements []arrayStringElement
	close := 0
	closed := false
	depth := 0
	err := walkJSONC(content, start, func(event jsoncEvent) (bool, error) {
		if event.ch == '"' {
			if depth == 1 {
				elements = append(elements, arrayStringElement{value: event.stringValue, start: event.pos, end: event.end})
			}
			return false, nil
		}
		switch event.ch {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				close = event.pos
				closed = true
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return nil, 0, err
	}
	if !closed {
		return nil, 0, fmt.Errorf("read OpenCode config: instructions array is not closed")
	}
	return elements, close, nil
}
func insertInstructionProperty(existing string, instructions []string) (string, error) {
	close, err := rootObjectClose(existing)
	if err != nil {
		return "", err
	}
	indent := inferPropertyIndent(existing, close)
	prefix := strings.TrimRight(existing[:close], " \t\r\n")
	suffix := existing[close:]
	if hasTopLevelProperties(existing[:close]) {
		prefix = ensureTrailingComma(prefix)
	}
	return prefix + "\n" + indent + `"instructions": ` + renderInstructionArray(instructions, indent) + "\n" + suffix, nil
}
func ensureTrailingComma(content string) string {
	commentStart := trailingCommentStart(content)
	insertAt := len(content)
	checkBefore := len(content)
	if commentStart >= 0 {
		insertAt = commentStart
		for insertAt > 0 && (content[insertAt-1] == ' ' || content[insertAt-1] == '\t') {
			insertAt--
		}
		checkBefore = insertAt
	}
	last := previousNonWhitespace(content, checkBefore-1)
	if last >= 0 && content[last] == ',' {
		return content
	}
	return content[:insertAt] + "," + content[insertAt:]
}
func trailingCommentStart(content string) int {
	lineStart := strings.LastIndexAny(content, "\n\r") + 1
	for i := lineStart; i < len(content); i++ {
		if content[i] == '"' {
			end, _, err := readJSONString(content, i)
			if err != nil {
				return -1
			}
			i = end - 1
			continue
		}
		if i+1 >= len(content) || content[i] != '/' {
			continue
		}
		switch content[i+1] {
		case '/':
			return i
		case '*':
			end, err := skipComment(content, i)
			if err != nil {
				return -1
			}
			if strings.TrimSpace(content[end:]) == "" {
				return i
			}
			i = end - 1
		}
	}
	return -1
}
func insertInstructionArrayEntry(existing string, close int, propertyIndent string, elements []arrayStringElement, value string) string {
	entryIndent := propertyIndent + "  "
	encoded, _ := json.Marshal(value)
	updated := existing
	adjustedClose := close
	if len(elements) > 0 {
		last := elements[len(elements)-1]
		if comma := skipWhitespaceAndComments(existing, last.end); comma >= len(existing) || existing[comma] != ',' {
			updated = existing[:last.end] + "," + existing[last.end:]
			adjustedClose++
		}
	}
	prefix := strings.TrimRight(updated[:adjustedClose], " \t\r\n")
	return prefix + "\n" + entryIndent + string(encoded) + updated[adjustedClose:]
}
func removeArrayElement(existing string, element arrayStringElement) string {
	start := propertyLineStart(existing, element.start)
	end := element.end
	if comma := skipWhitespaceAndComments(existing, element.end); comma < len(existing) && existing[comma] == ',' {
		end = comma + 1
		for end < len(existing) && (existing[end] == ' ' || existing[end] == '\t') {
			end++
		}
		if end < len(existing) && (existing[end] == '\n' || existing[end] == '\r') {
			if existing[end] == '\r' && end+1 < len(existing) && existing[end+1] == '\n' {
				end++
			}
			end++
		}
		return existing[:start] + existing[end:]
	}
	if comma := previousNonWhitespace(existing, start-1); comma >= 0 && existing[comma] == ',' {
		start = comma
		for start > 0 && (existing[start-1] == ' ' || existing[start-1] == '\t') {
			start--
		}
	}
	return existing[:start] + existing[end:]
}
func removeProperty(existing string, property propertyRange) string {
	start := property.propertyStart
	end := property.propertyEnd
	if property.propertyEnd == property.valueEnd {
		comma := previousNonWhitespace(existing, start-1)
		if comma >= 0 && existing[comma] == ',' {
			start = comma
			for start > 0 && (existing[start-1] == ' ' || existing[start-1] == '\t') {
				start--
			}
		}
	}
	for end < len(existing) && (existing[end] == ' ' || existing[end] == '\t') {
		end++
	}
	if end < len(existing) && (existing[end] == '\n' || existing[end] == '\r') {
		if existing[end] == '\r' && end+1 < len(existing) && existing[end+1] == '\n' {
			end++
		}
		end++
	}
	return existing[:start] + existing[end:]
}
func renderInstructionArray(instructions []string, indent string) string {
	data, _ := json.MarshalIndent(instructions, indent, "  ")
	return string(data)
}
func arrayValueHasComments(content string) bool {
	for i := 0; i < len(content); i++ {
		switch content[i] {
		case '"':
			end, _, err := readJSONString(content, i)
			if err != nil {
				return false
			}
			i = end - 1
		case '/':
			if i+1 < len(content) && (content[i+1] == '/' || content[i+1] == '*') {
				return true
			}
		}
	}
	return false
}

type jsoncEvent struct {
	pos         int
	end         int
	ch          byte
	stringValue string
}

func walkJSONC(content string, start int, visit func(jsoncEvent) (bool, error)) error {
	for i := start; i < len(content); i++ {
		if content[i] == '"' {
			end, value, err := readJSONString(content, i)
			if err != nil {
				return err
			}
			stop, err := visit(jsoncEvent{pos: i, end: end, ch: '"', stringValue: value})
			if stop || err != nil {
				return err
			}
			i = end - 1
			continue
		}
		if end, err := skipComment(content, i); err != nil {
			return err
		} else if end != i {
			i = end - 1
			continue
		}
		stop, err := visit(jsoncEvent{pos: i, end: i + 1, ch: content[i]})
		if stop || err != nil {
			return err
		}
	}
	return nil
}
func readJSONString(content string, start int) (int, string, error) {
	for i := start + 1; i < len(content); i++ {
		if content[i] == '\\' {
			i++
			continue
		}
		if content[i] == '"' {
			var value string
			if err := json.Unmarshal([]byte(content[start:i+1]), &value); err != nil {
				return 0, "", err
			}
			return i + 1, value, nil
		}
	}
	return 0, "", fmt.Errorf("unterminated string")
}
func skipWhitespaceAndComments(content string, i int) int {
	for i < len(content) {
		if isJSONWhitespace(content[i]) {
			i++
			continue
		}
		end, err := skipComment(content, i)
		if err == nil && end != i {
			i = end
			continue
		}
		return i
	}
	return i
}
func skipComment(content string, start int) (int, error) {
	if start+1 >= len(content) || content[start] != '/' {
		return start, nil
	}
	switch content[start+1] {
	case '/':
		i := start + 2
		for i < len(content) && content[i] != '\n' && content[i] != '\r' {
			i++
		}
		return i, nil
	case '*':
		for i := start + 2; i+1 < len(content); i++ {
			if content[i] == '*' && content[i+1] == '/' {
				return i + 2, nil
			}
		}
		return 0, fmt.Errorf("unterminated block comment")
	default:
		return start, nil
	}
}
func endJSONValue(content string, start int) (int, error) {
	if start >= len(content) {
		return 0, fmt.Errorf("read OpenCode config: missing property value")
	}
	if content[start] == '"' {
		end, _, err := readJSONString(content, start)
		return end, err
	}
	end := len(content)
	depth := 0
	err := walkJSONC(content, start, func(event jsoncEvent) (bool, error) {
		switch event.ch {
		case '{', '[':
			depth++
		case '}', ']':
			if depth == 0 {
				end = event.pos
				return true, nil
			}
			depth--
		case ',':
			if depth == 0 {
				end = event.pos
				return true, nil
			}
		}
		return false, nil
	})
	return end, err
}
func rootObjectClose(content string) (int, error) {
	close := 0
	closed := false
	depth := 0
	err := walkJSONC(content, 0, func(event jsoncEvent) (bool, error) {
		switch event.ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				close = event.pos
				closed = true
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return 0, err
	}
	if !closed {
		return 0, fmt.Errorf("read OpenCode config: root object is not closed")
	}
	return close, nil
}
func propertyLineStart(content string, i int) int {
	for i > 0 && content[i-1] != '\n' && content[i-1] != '\r' {
		i--
	}
	return i
}
func lineIndent(content string, i int) string {
	start := propertyLineStart(content, i)
	for i > start && (content[start] == ' ' || content[start] == '\t') {
		start++
	}
	return content[propertyLineStart(content, i):start]
}
func inferPropertyIndent(content string, close int) string {
	for i := close - 1; i >= 0; i-- {
		if content[i] == '\n' || content[i] == '\r' {
			j := i + 1
			for j < close && (content[j] == ' ' || content[j] == '\t') {
				j++
			}
			if j < close && content[j] == '"' {
				return content[i+1 : j]
			}
		}
	}
	return "  "
}
func hasTopLevelProperties(content string) bool {
	depth := 0
	found := false
	_ = walkJSONC(content, 0, func(event jsoncEvent) (bool, error) {
		if event.ch == '"' {
			if depth == 1 {
				if colon := skipWhitespaceAndComments(content, event.end); colon < len(content) && content[colon] == ':' {
					found = true
					return true, nil
				}
			}
			return false, nil
		}
		switch event.ch {
		case '{':
			depth++
		case '}':
			depth--
		}
		return false, nil
	})
	return found
}
func previousNonWhitespace(content string, i int) int {
	for i >= 0 && isJSONWhitespace(content[i]) {
		i--
	}
	return i
}
func jsoncToJSON(content string) ([]byte, error) {
	withoutComments, err := stripJSONCComments(content)
	if err != nil {
		return nil, err
	}
	withoutTrailingCommas, err := stripTrailingCommas(withoutComments)
	if err != nil {
		return nil, err
	}
	return []byte(withoutTrailingCommas), nil
}
func stripJSONCComments(content string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(content); i++ {
		ch := content[i]
		switch ch {
		case '"':
			end, err := copyJSONString(content, i, &out)
			if err != nil {
				return "", err
			}
			i = end - 1
		case '/':
			if i+1 >= len(content) {
				out.WriteByte(ch)
				continue
			}
			next := content[i+1]
			switch next {
			case '/':
				i += 2
				for i < len(content) && content[i] != '\n' && content[i] != '\r' {
					i++
				}
				if i < len(content) {
					out.WriteByte(content[i])
				}
			case '*':
				i += 2
				closed := false
				for i+1 < len(content) {
					if content[i] == '*' && content[i+1] == '/' {
						closed = true
						i++
						break
					}
					if content[i] == '\n' || content[i] == '\r' {
						out.WriteByte(content[i])
					}
					i++
				}
				if !closed {
					return "", fmt.Errorf("unterminated block comment")
				}
			default:
				out.WriteByte(ch)
			}
		default:
			out.WriteByte(ch)
		}
	}
	return out.String(), nil
}
func stripTrailingCommas(content string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(content); i++ {
		ch := content[i]
		if ch == '"' {
			end, err := copyJSONString(content, i, &out)
			if err != nil {
				return "", err
			}
			i = end - 1
			continue
		}
		if ch == ',' {
			j := i + 1
			for j < len(content) && isJSONWhitespace(content[j]) {
				j++
			}
			if j < len(content) && (content[j] == '}' || content[j] == ']') {
				continue
			}
		}
		out.WriteByte(ch)
	}
	return out.String(), nil
}
func copyJSONString(content string, start int, out *strings.Builder) (int, error) {
	for i := start; i < len(content); i++ {
		out.WriteByte(content[i])
		if i == start {
			continue
		}
		if content[i] == '\\' {
			i++
			if i < len(content) {
				out.WriteByte(content[i])
			}
			continue
		}
		if content[i] == '"' {
			return i + 1, nil
		}
	}
	return 0, fmt.Errorf("unterminated string")
}
func isJSONWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t'
}
func instructionStrings(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("read OpenCode config: instructions must be an array of strings")
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("read OpenCode config: instructions must be an array of strings")
		}
		out = append(out, s)
	}
	return out, nil
}
func removeString(items []string, value string) []string {
	out := items[:0]
	for _, item := range items {
		if item != value {
			out = append(out, item)
		}
	}
	return out
}
func readOptionalFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}
