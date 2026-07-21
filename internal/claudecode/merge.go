package claudecode

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

type InstructionContribution struct{ ContributorID, Content string }

// UpsertInstructionContribution changes one contributor and retains every
// unrelated byte, including other Packy contributors.
func UpsertInstructionContribution(document string, contribution InstructionContribution) (string, error) {
	if err := validateMarkerPair(document, instructionStart, instructionEnd, "Packy instruction"); err != nil {
		return "", err
	}
	start := "<!-- contributor:" + contribution.ContributorID + " -->"
	end := "<!-- /contributor:" + contribution.ContributorID + " -->"
	block := start + "\n" + strings.TrimSpace(contribution.Content) + "\n" + end
	if err := validateMarkerPair(document, start, end, "Packy contributor"); err != nil {
		return "", err
	}
	if i := strings.Index(document, start); i >= 0 {
		j := strings.Index(document[i:], end)
		return document[:i] + block + document[i+j+len(end):], nil
	}
	if i := strings.Index(document, instructionStart); i >= 0 {
		j := i + strings.Index(document[i:], instructionEnd)
		prefix := document[:j]
		if !strings.HasSuffix(prefix, "\n") {
			prefix += "\n"
		}
		return prefix + block + "\n" + document[j:], nil
	}
	return MergeInstructions(document, []InstructionContribution{contribution})
}

func RemoveInstructionContribution(document, contributorID string) (string, error) {
	if err := validateMarkerPair(document, instructionStart, instructionEnd, "Packy instruction"); err != nil {
		return "", err
	}
	start := "<!-- contributor:" + contributorID + " -->"
	end := "<!-- /contributor:" + contributorID + " -->"
	if err := validateMarkerPair(document, start, end, "Packy contributor"); err != nil {
		return "", err
	}
	i := strings.Index(document, start)
	if i < 0 {
		return document, nil
	}
	j := strings.Index(document[i:], end)
	result := document[:i] + document[i+j+len(end):]
	insideStart := strings.Index(result, instructionStart)
	insideEnd := strings.Index(result, instructionEnd)
	if insideStart >= 0 && insideEnd >= 0 && strings.TrimSpace(result[insideStart+len(instructionStart):insideEnd]) == "" {
		result = result[:insideStart] + result[insideEnd+len(instructionEnd):]
	}
	return result, nil
}

func MergeInstructions(document string, contributions []InstructionContribution) (string, error) {
	if err := validateMarkerPair(document, instructionStart, instructionEnd, "Packy instruction"); err != nil {
		return "", err
	}
	seen := map[string]bool{}
	sort.Slice(contributions, func(i, j int) bool { return contributions[i].ContributorID < contributions[j].ContributorID })
	var body strings.Builder
	for _, c := range contributions {
		if strings.TrimSpace(c.ContributorID) == "" || seen[c.ContributorID] {
			return "", fmt.Errorf("duplicate or empty instruction contributor %q", c.ContributorID)
		}
		seen[c.ContributorID] = true
		fmt.Fprintf(&body, "<!-- contributor:%s -->\n%s\n<!-- /contributor:%s -->\n", c.ContributorID, strings.TrimSpace(c.Content), c.ContributorID)
	}
	block := instructionStart + "\n" + body.String() + instructionEnd
	if i := strings.Index(document, instructionStart); i >= 0 {
		j := strings.Index(document[i:], instructionEnd)
		return document[:i] + block + document[i+j+len(instructionEnd):], nil
	}
	if strings.TrimSpace(document) == "" {
		return block + "\n", nil
	}
	return strings.TrimRight(document, "\n") + "\n\n" + block + "\n", nil
}

func validateMarkerPair(document, start, end, label string) error {
	starts, ends := strings.Count(document, start), strings.Count(document, end)
	if starts != ends || starts > 1 {
		return fmt.Errorf("invalid or duplicate %s markers", label)
	}
	return nil
}

type CommandHookEntry struct {
	Type, Event, Matcher, Command string
	Args                          []string
	TimeoutSeconds                int
	Blocking                      bool
	Failure                       string
	Authorities                   []string
}
type HookMergeProvenance struct{ CreatedHooksContainer, CreatedEvent bool }

func (p HookMergeProvenance) Seal() string {
	parts := []string{}
	if p.CreatedHooksContainer {
		parts = append(parts, "hooks")
	}
	if p.CreatedEvent {
		parts = append(parts, "event")
	}
	return strings.Join(parts, ",")
}
func ParseHookMergeProvenance(value string) HookMergeProvenance {
	return HookMergeProvenance{CreatedHooksContainer: slices.Contains(strings.Split(value, ","), "hooks"), CreatedEvent: slices.Contains(strings.Split(value, ","), "event")}
}

type JSONSpan struct{ Start, End int }
type JSONMemberSpan struct{ Full, Value JSONSpan }

func (h CommandHookEntry) Validate() error {
	if h.Type != "command" || h.Event == "" || h.Command == "" || h.TimeoutSeconds <= 0 {
		return errors.New("noncanonical Claude command hook")
	}
	if h.Failure != "block" && h.Failure != "warn" {
		return errors.New("invalid Claude command hook failure behavior")
	}
	return nil
}
func (h CommandHookEntry) Fingerprint() string { return canonicalFingerprint(hookJSON(h)) }

// MergeCommandHook preserves all unrelated JSON values and entries.
func MergeCommandHook(settings []byte, hook CommandHookEntry, remove bool) ([]byte, error) {
	result, _, err := MergeCommandHookWithProvenance(settings, hook, remove, HookMergeProvenance{})
	return result, err
}
func MergeCommandHookWithProvenance(settings []byte, hook CommandHookEntry, remove bool, provenance HookMergeProvenance) ([]byte, HookMergeProvenance, error) {
	if err := hook.Validate(); err != nil {
		return nil, provenance, err
	}
	var root map[string]any
	if len(strings.TrimSpace(string(settings))) == 0 {
		root = map[string]any{}
	} else if err := json.Unmarshal(settings, &root); err != nil {
		return nil, provenance, fmt.Errorf("invalid Claude settings JSON: %w", err)
	}
	hooks, ok := root["hooks"].(map[string]any)
	if root["hooks"] != nil && !ok {
		return nil, provenance, errors.New("Claude settings hooks must be an object")
	}
	if hooks == nil {
		hooks = map[string]any{}
	}
	entries, ok := hooks[hook.Event].([]any)
	if hooks[hook.Event] != nil && !ok {
		return nil, provenance, errors.New("Claude hook event entries must be an array")
	}
	wanted := hookJSON(hook)
	wantedBytes, _ := json.Marshal(wanted)
	matches := 0
	for _, e := range entries {
		if canonicalFingerprint(e) == canonicalFingerprint(wanted) {
			matches++
		}
	}
	if matches > 1 {
		return nil, provenance, errors.New("duplicate canonical Claude command hook")
	}
	if remove && matches == 0 {
		return append([]byte(nil), settings...), provenance, nil
	}
	if !remove && matches == 1 {
		return append([]byte(nil), settings...), provenance, nil
	}
	data := settings
	if len(strings.TrimSpace(string(data))) == 0 {
		data = []byte("{}")
	}
	hookSpan, found, err := jsonField(data, JSONSpan{0, len(data)}, "hooks")
	if err != nil {
		return nil, provenance, err
	}
	if !found {
		if remove {
			return append([]byte(nil), data...), provenance, nil
		}
		eventObject, _ := json.Marshal(map[string]any{hook.Event: []any{wanted}})
		result, err := insertObjectField(data, JSONSpan{0, len(data)}, "hooks", eventObject)
		return result, HookMergeProvenance{CreatedHooksContainer: true, CreatedEvent: true}, err
	}
	eventSpan, found, err := jsonField(data, hookSpan, hook.Event)
	if err != nil {
		return nil, provenance, err
	}
	if !found {
		if remove {
			return append([]byte(nil), data...), provenance, nil
		}
		arr := append([]byte{'['}, wantedBytes...)
		arr = append(arr, ']')
		result, err := insertObjectField(data, hookSpan, hook.Event, arr)
		return result, HookMergeProvenance{CreatedEvent: true}, err
	}
	if remove {
		result, err := removeMatchingArrayElement(data, eventSpan, canonicalFingerprint(wanted))
		if err != nil {
			return nil, provenance, err
		}
		if provenance.CreatedEvent && arrayEmpty(result, eventSpan.Start) {
			result, err = removeObjectField(result, hookSpan, hook.Event)
			if err != nil {
				return nil, provenance, err
			}
		}
		if provenance.CreatedHooksContainer {
			rootSpan := JSONSpan{0, len(result)}
			hs, found, e := jsonField(result, rootSpan, "hooks")
			if e != nil {
				return nil, provenance, e
			}
			if found && objectEmpty(result, hs) {
				result, e = removeObjectField(result, rootSpan, "hooks")
				if e != nil {
					return nil, provenance, e
				}
			}
		}
		return result, provenance, nil
	}
	result, err := appendArrayElement(data, eventSpan, wantedBytes)
	return result, provenance, err
}
func hookJSON(h CommandHookEntry) map[string]any {
	return map[string]any{"type": h.Type, "matcher": h.Matcher, "command": h.Command, "args": h.Args, "timeout_seconds": h.TimeoutSeconds, "blocking": h.Blocking, "failure": h.Failure, "authorities": h.Authorities}
}

func jsonField(data []byte, object JSONSpan, key string) (JSONSpan, bool, error) {
	member, found, err := jsonObjectMember(data, object, key)
	return member.Value, found, err
}
func jsonObjectMember(data []byte, object JSONSpan, key string) (JSONMemberSpan, bool, error) {
	i := skipSpace(data, object.Start)
	if i >= object.End || data[i] != '{' {
		return JSONMemberSpan{}, false, errors.New("JSON value must be an object")
	}
	i++
	for {
		i = skipDelimiters(data, i)
		if i >= object.End || data[i] == '}' {
			return JSONMemberSpan{}, false, nil
		}
		memberStart := i
		ks, ke, err := scanString(data, i)
		if err != nil {
			return JSONMemberSpan{}, false, err
		}
		var name string
		if err = json.Unmarshal(data[ks:ke], &name); err != nil {
			return JSONMemberSpan{}, false, err
		}
		i = skipSpace(data, ke)
		if i >= object.End || data[i] != ':' {
			return JSONMemberSpan{}, false, errors.New("invalid JSON object")
		}
		vs := skipSpace(data, i+1)
		ve, err := scanValue(data, vs)
		if err != nil {
			return JSONMemberSpan{}, false, err
		}
		if name == key {
			return JSONMemberSpan{Full: JSONSpan{memberStart, ve}, Value: JSONSpan{vs, ve}}, true, nil
		}
		i = ve
	}
}
func scanString(data []byte, i int) (int, int, error) {
	if i >= len(data) || data[i] != '"' {
		return 0, 0, errors.New("expected JSON string")
	}
	start := i
	i++
	for i < len(data) {
		if data[i] == '\\' {
			i += 2
			continue
		}
		if data[i] == '"' {
			return start, i + 1, nil
		}
		i++
	}
	return 0, 0, errors.New("unterminated JSON string")
}
func scanValue(data []byte, i int) (int, error) {
	if i >= len(data) {
		return 0, errors.New("missing JSON value")
	}
	if data[i] == '"' {
		_, e, err := scanString(data, i)
		return e, err
	}
	depth := 0
	inString := false
	for j := i; j < len(data); j++ {
		c := data[j]
		if inString {
			if c == '\\' {
				j++
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			continue
		}
		if c == '{' || c == '[' {
			depth++
		}
		if c == '}' || c == ']' {
			if depth == 0 {
				return j, nil
			}
			depth--
			if depth == 0 {
				return j + 1, nil
			}
		}
		if depth == 0 && (c == ',' || c == '}' || c == ']') {
			return j, nil
		}
	}
	return len(data), nil
}
func skipSpace(data []byte, i int) int {
	for i < len(data) && (data[i] == ' ' || data[i] == '\n' || data[i] == '\r' || data[i] == '\t') {
		i++
	}
	return i
}
func skipDelimiters(data []byte, i int) int {
	i = skipSpace(data, i)
	if i < len(data) && data[i] == ',' {
		i = skipSpace(data, i+1)
	}
	return i
}
func insertObjectField(data []byte, object JSONSpan, key string, value []byte) ([]byte, error) {
	close := object.End - 1
	for close >= object.Start && data[close] != '}' {
		close--
	}
	if close < object.Start {
		return nil, errors.New("invalid JSON object")
	}
	inner := strings.TrimSpace(string(data[object.Start+1 : close]))
	field, _ := json.Marshal(key)
	insert := append(field, ':')
	insert = append(insert, value...)
	if inner != "" {
		insert = append([]byte{','}, insert...)
	}
	out := append([]byte(nil), data[:close]...)
	out = append(out, insert...)
	out = append(out, data[close:]...)
	return out, nil
}
func appendArrayElement(data []byte, array JSONSpan, value []byte) ([]byte, error) {
	close := array.End - 1
	for close >= array.Start && data[close] != ']' {
		close--
	}
	if close < array.Start {
		return nil, errors.New("invalid hook array")
	}
	insert := value
	if strings.TrimSpace(string(data[array.Start+1:close])) != "" {
		insert = append([]byte{','}, insert...)
	}
	out := append([]byte(nil), data[:close]...)
	out = append(out, insert...)
	out = append(out, data[close:]...)
	return out, nil
}
func removeMatchingArrayElement(data []byte, array JSONSpan, want string) ([]byte, error) {
	i := skipSpace(data, array.Start)
	if i >= array.End || data[i] != '[' {
		return nil, errors.New("hook entries must be an array")
	}
	i++
	type span struct{ s, e int }
	var spans []span
	for {
		i = skipDelimiters(data, i)
		if i >= array.End || data[i] == ']' {
			break
		}
		e, err := scanValue(data, i)
		if err != nil {
			return nil, err
		}
		var v any
		if err = json.Unmarshal(data[i:e], &v); err != nil {
			return nil, err
		}
		if canonicalFingerprint(v) == want {
			spans = append(spans, span{i, e})
		}
		i = e
	}
	if len(spans) != 1 {
		return nil, errors.New("duplicate or missing canonical Claude command hook")
	}
	return spliceRemovingJSONItem(data, array, JSONSpan{spans[0].s, spans[0].e}), nil
}
func arrayEmpty(data []byte, start int) bool {
	end, err := scanValue(data, start)
	return err == nil && strings.TrimSpace(string(data[start+1:end-1])) == ""
}
func objectEmpty(data []byte, object JSONSpan) bool {
	return object.Start < len(data) && object.End <= len(data) && strings.TrimSpace(string(data[object.Start+1:object.End-1])) == ""
}
func removeObjectField(data []byte, object JSONSpan, key string) ([]byte, error) {
	member, found, err := jsonObjectMember(data, object, key)
	if err != nil || !found {
		return data, err
	}
	return spliceRemovingJSONItem(data, object, member.Full), nil
}
func spliceRemovingJSONItem(data []byte, container, item JSONSpan) []byte {
	s, e := item.Start, item.End
	left := s - 1
	for left > container.Start && (data[left] == ' ' || data[left] == '\n' || data[left] == '\r' || data[left] == '\t') {
		left--
	}
	if left >= container.Start && data[left] == ',' {
		s = left
	} else {
		right := skipSpace(data, e)
		if right < container.End && data[right] == ',' {
			e = right + 1
		}
	}
	out := append([]byte(nil), data[:s]...)
	return append(out, data[e:]...)
}
