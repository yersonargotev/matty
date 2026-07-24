package claudecode

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

var addyReferenceIDs = []string{"accessibility-checklist", "definition-of-done", "observability-checklist", "orchestration-patterns", "performance-checklist", "security-checklist", "testing-patterns"}

type compositeFile struct {
	Path    string `json:"path"`
	Content []byte `json:"content"`
	Mode    uint32 `json:"mode"`
}

type compositeOwnership struct {
	PackID                string `json:"pack_id"`
	PackVersion           string `json:"pack_version"`
	PortableKind          string `json:"portable_kind"`
	PortableID            string `json:"portable_id"`
	EffectiveName         string `json:"effective_name"`
	TargetType            string `json:"target_type"`
	SourceFingerprint     string `json:"source_fingerprint"`
	TreeFingerprint       string `json:"tree_fingerprint"`
	DefinitionFingerprint string `json:"definition_fingerprint"`
}

type compositeSkill struct {
	Files                 []compositeFile
	TreeFingerprint       string
	SourceFingerprint     string
	DefinitionFingerprint string
	Ownership             compositeOwnership
}

// addyCompositeSkill is a pure translation: it reads selected bundle bytes but
// never interprets or executes skill content.
func addyCompositeSkill(pack capabilitypack.Pack, resource capabilitypack.Resource, binding capabilitypack.Binding, bundleRoot string) (compositeSkill, error) {
	if pack.ID != "addy" || pack.Version != "1.1.0" {
		return compositeSkill{}, fmt.Errorf("composite Claude skills require addy@1.1.0")
	}
	if resource.Kind != "skill" && resource.Kind != "command" {
		return compositeSkill{}, fmt.Errorf("unsupported Addy composite kind %q", resource.Kind)
	}
	if binding.Surface != capabilitypack.SurfaceClaude || binding.Projection != "skill" || binding.Name == "" {
		return compositeSkill{}, errors.New("invalid Addy Claude skill binding")
	}
	source, err := safeBundlePath(bundleRoot, resource.Source)
	if err != nil {
		return compositeSkill{}, err
	}
	var files []compositeFile
	if resource.Kind == "skill" {
		files, err = readCompositeTree(source)
	} else {
		var raw []byte
		var info os.FileInfo
		info, err = os.Lstat(source)
		if err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
			err = errors.New("Addy command source is not a regular file")
		}
		if err == nil {
			raw, err = os.ReadFile(source)
		}
		if err == nil {
			var command addyCommand
			command, err = decodeAddyCommand(raw)
			if err == nil {
				var dependencies []string
				dependencies, err = resolveAddyDependencies(pack, resource)
				if err == nil {
					files = []compositeFile{
						{Path: "SKILL.md", Content: renderAddyCommandSkill(binding.Name, command, dependencies), Mode: 0o644},
						{Path: "source/" + filepath.Base(resource.Source), Content: append([]byte(nil), raw...), Mode: 0o644},
					}
				}
			}
		}
	}
	if err != nil {
		return compositeSkill{}, err
	}
	for _, id := range addyReferenceIDs {
		asset, err := uniqueResource(pack, "asset", id)
		if err != nil {
			return compositeSkill{}, err
		}
		path, err := safeBundlePath(bundleRoot, asset.Source)
		if err != nil {
			return compositeSkill{}, err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return compositeSkill{}, fmt.Errorf("read Addy reference %s: %w", id, err)
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return compositeSkill{}, fmt.Errorf("Addy reference %s is not a regular file", id)
		}
		files = append(files, compositeFile{Path: "references/" + filepath.Base(asset.Source), Content: content, Mode: normalizedMode(info.Mode())})
	}
	if err := canonicalizeCompositeFiles(files); err != nil {
		return compositeSkill{}, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	sourceFP, err := sourceFingerprint(source)
	if err != nil {
		return compositeSkill{}, err
	}
	definition, err := json.Marshal(struct {
		PackID, Version string
		Resource        capabilitypack.Resource
		Binding         capabilitypack.Binding
	}{pack.ID, pack.Version, resource, binding})
	if err != nil {
		return compositeSkill{}, err
	}
	treeFP, err := fingerprintCompositeFiles(files)
	if err != nil {
		return compositeSkill{}, err
	}
	result := compositeSkill{Files: files, TreeFingerprint: treeFP, SourceFingerprint: sourceFP, DefinitionFingerprint: localprojection.FingerprintBytes(definition)}
	result.Ownership = compositeOwnership{PackID: pack.ID, PackVersion: pack.Version, PortableKind: resource.Kind, PortableID: resource.ID, EffectiveName: binding.Name, TargetType: "claude-personal-skill", SourceFingerprint: result.SourceFingerprint, TreeFingerprint: result.TreeFingerprint, DefinitionFingerprint: result.DefinitionFingerprint}
	return result, nil
}

func safeBundlePath(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) || filepath.Clean(relative) != relative || relative == "." || strings.HasPrefix(filepath.ToSlash(relative), "../") {
		return "", fmt.Errorf("unsafe Addy source path %q", relative)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve Addy bundle root: %w", err)
	}
	candidate := filepath.Join(root, filepath.FromSlash(relative))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	within, err := filepath.Rel(canonicalRoot, resolved)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) || filepath.IsAbs(within) {
		return "", fmt.Errorf("Addy source escapes bundle root %q", relative)
	}
	return candidate, nil
}

func readCompositeTree(root string) ([]compositeFile, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("Addy skill source is not a directory")
	}
	var files []compositeFile
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("Addy skill source contains symlink %q", path)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Addy skill source contains non-regular file %q", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, compositeFile{Path: filepath.ToSlash(rel), Content: content, Mode: normalizedMode(info.Mode())})
		return nil
	})
	return files, err
}

func normalizedMode(mode os.FileMode) uint32 {
	if mode&0o111 != 0 {
		return 0o755
	}
	return 0o644
}

func sourceFingerprint(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return localprojection.FingerprintExactTree(path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return localprojection.FingerprintBytes(b), nil
}

func uniqueResource(pack capabilitypack.Pack, kind, id string) (capabilitypack.Resource, error) {
	var found *capabilitypack.Resource
	for i := range pack.Resources {
		if pack.Resources[i].Kind == kind && pack.Resources[i].ID == id {
			if found != nil {
				return capabilitypack.Resource{}, fmt.Errorf("duplicate Addy dependency %s:%s", kind, id)
			}
			r := pack.Resources[i]
			found = &r
		}
	}
	if found == nil {
		return capabilitypack.Resource{}, fmt.Errorf("missing Addy dependency %s:%s", kind, id)
	}
	return *found, nil
}

func resolveAddyDependencies(pack capabilitypack.Pack, command capabilitypack.Resource) ([]string, error) {
	seen := map[string]bool{}
	result := []string{}
	for _, required := range command.Requires {
		if seen[required] {
			return nil, fmt.Errorf("duplicate Addy requirement %q", required)
		}
		seen[required] = true
		parts := strings.Split(required, ":")
		if len(parts) != 2 || (parts[0] != "skill" && parts[0] != "agent" && parts[0] != "asset") {
			return nil, fmt.Errorf("unsupported Addy requirement %q", required)
		}
		r, err := uniqueResource(pack, parts[0], parts[1])
		if err != nil {
			return nil, err
		}
		if parts[0] == "asset" {
			result = append(result, "reference:"+filepath.Base(r.Source))
			continue
		}
		var names []string
		for _, b := range r.Bindings {
			if b.Surface == capabilitypack.SurfaceClaude {
				names = append(names, b.Name)
			}
		}
		if len(names) != 1 || names[0] == "" {
			return nil, fmt.Errorf("Addy dependency %s has %d Claude bindings", required, len(names))
		}
		result = append(result, parts[0]+":"+names[0])
	}
	sort.Strings(result)
	return result, nil
}

type addyCommand struct{ Description, Prompt string }

func decodeAddyCommand(data []byte) (addyCommand, error) {
	if !utf8.Valid(data) {
		return addyCommand{}, errors.New("Addy command is not valid UTF-8")
	}
	i, values := 0, map[string]string{}
	for {
		skipTOMLSpace(data, &i)
		if i == len(data) {
			break
		}
		start := i
		for i < len(data) && ((data[i] >= 'a' && data[i] <= 'z') || data[i] == '_') {
			i++
		}
		if start == i {
			return addyCommand{}, fmt.Errorf("invalid Addy command TOML at byte %d", i)
		}
		key := string(data[start:i])
		if key != "description" && key != "prompt" {
			return addyCommand{}, fmt.Errorf("unknown Addy command key %q", key)
		}
		if _, ok := values[key]; ok {
			return addyCommand{}, fmt.Errorf("duplicate Addy command key %q", key)
		}
		for i < len(data) && (data[i] == ' ' || data[i] == '\t') {
			i++
		}
		if i >= len(data) || data[i] != '=' {
			return addyCommand{}, fmt.Errorf("invalid Addy command assignment %q", key)
		}
		i++
		for i < len(data) && (data[i] == ' ' || data[i] == '\t') {
			i++
		}
		value, next, err := parseTOMLString(data, i)
		if err != nil {
			return addyCommand{}, fmt.Errorf("Addy command %s: %w", key, err)
		}
		values[key], i = value, next
		for i < len(data) && (data[i] == ' ' || data[i] == '\t') {
			i++
		}
		if i < len(data) && data[i] == '#' {
			for i < len(data) && data[i] != '\n' {
				i++
			}
		}
		if i < len(data) && data[i] != '\n' && data[i] != '\r' {
			return addyCommand{}, fmt.Errorf("trailing Addy command syntax at byte %d", i)
		}
	}
	if _, ok := values["description"]; !ok {
		return addyCommand{}, errors.New("Addy command is missing description")
	}
	if _, ok := values["prompt"]; !ok {
		return addyCommand{}, errors.New("Addy command is missing prompt")
	}
	return addyCommand{values["description"], values["prompt"]}, nil
}

func skipTOMLSpace(data []byte, i *int) {
	for *i < len(data) {
		if data[*i] == ' ' || data[*i] == '\t' || data[*i] == '\r' || data[*i] == '\n' {
			*i++
			continue
		}
		if data[*i] == '#' {
			for *i < len(data) && data[*i] != '\n' {
				*i++
			}
			continue
		}
		break
	}
}

func parseTOMLString(data []byte, i int) (string, int, error) {
	if i >= len(data) || (data[i] != '"' && data[i] != '\'') {
		return "", i, errors.New("value must be a string")
	}
	quote := data[i]
	triple := i+2 < len(data) && data[i+1] == quote && data[i+2] == quote
	if triple {
		i += 3
		if i < len(data) && data[i] == '\n' {
			i++
		}
		start := i
		marker := bytes.Repeat([]byte{quote}, 3)
		end := bytes.Index(data[i:], marker)
		if end < 0 {
			return "", i, errors.New("unterminated multiline string")
		}
		raw := data[start : i+end]
		i += end + 3
		if quote == '\'' {
			return string(raw), i, nil
		}
		value, err := unescapeTOML(raw)
		return value, i, err
	}
	i++
	start := i
	if quote == '\'' {
		end := bytes.IndexByte(data[i:], quote)
		if end < 0 {
			return "", i, errors.New("unterminated string")
		}
		return string(data[start : i+end]), i + end + 1, nil
	}
	for i < len(data) {
		if data[i] == '\\' {
			i += 2
			continue
		}
		if i < len(data) && data[i] == quote {
			value, err := strconv.Unquote(string(data[start-1 : i+1]))
			return value, i + 1, err
		}
		if data[i] == '\n' || data[i] == '\r' {
			break
		}
		i++
	}
	return "", i, errors.New("unterminated string")
}

func unescapeTOML(raw []byte) (string, error) {
	escaped := strings.ReplaceAll(string(raw), "\r", `\r`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	value, err := strconv.Unquote(`"` + escaped + `"`)
	return value, err
}

func renderAddyCommandSkill(name string, command addyCommand, dependencies []string) []byte {
	var b strings.Builder
	b.WriteString("---\nname: ")
	b.WriteString(yamlScalar(name))
	b.WriteString("\ndescription: ")
	b.WriteString(yamlScalar(command.Description))
	b.WriteString("\n---\n\n")
	b.WriteString("## Packy dependency contract\n\n")
	for _, dependency := range dependencies {
		b.WriteString("- `")
		b.WriteString(dependency)
		b.WriteString("`\n")
	}
	b.WriteString("\n## Arguments\n\nUse the caller-provided arguments exactly as `$ARGUMENTS`.\n\n")
	b.WriteString(command.Prompt)
	return []byte(b.String())
}

func yamlScalar(value string) string { encoded, _ := json.Marshal(value); return string(encoded) }

func canonicalizeCompositeFiles(files []compositeFile) error {
	seen := map[string]bool{}
	for _, file := range files {
		if file.Path == "" || file.Path != filepath.ToSlash(filepath.Clean(file.Path)) || strings.HasPrefix(file.Path, "/") || strings.HasPrefix(file.Path, "../") {
			return fmt.Errorf("noncanonical composite path %q", file.Path)
		}
		if seen[file.Path] {
			return fmt.Errorf("duplicate composite path %q", file.Path)
		}
		seen[file.Path] = true
		if file.Mode != 0o644 && file.Mode != 0o755 {
			return fmt.Errorf("invalid composite mode %o", file.Mode)
		}
	}
	return nil
}

func fingerprintCompositeFiles(files []compositeFile) (string, error) {
	treeFiles := make([]localprojection.TreeFile, len(files))
	for i, file := range files {
		treeFiles[i] = localprojection.TreeFile{Path: file.Path, Content: file.Content, Mode: fs.FileMode(file.Mode)}
	}
	return localprojection.FingerprintTreeFiles(treeFiles)
}

type compositeSkillPayload struct {
	Files                 []compositeFile    `json:"files"`
	TreeFingerprint       string             `json:"tree_fingerprint"`
	SourceFingerprint     string             `json:"source_fingerprint"`
	DefinitionFingerprint string             `json:"definition_fingerprint"`
	Ownership             compositeOwnership `json:"ownership"`
}

func canonicalCompositeSkillPayload(skill compositeSkill) ([]byte, error) {
	p := compositeSkillPayload{skill.Files, skill.TreeFingerprint, skill.SourceFingerprint, skill.DefinitionFingerprint, skill.Ownership}
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func canonicalCompositeOwnership(ownership compositeOwnership) (string, error) {
	data, err := json.Marshal(ownership)
	return string(data), err
}

func decodeCompositeOwnership(data string) (compositeOwnership, error) {
	var ownership compositeOwnership
	decoder := json.NewDecoder(strings.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&ownership); err != nil {
		return compositeOwnership{}, fmt.Errorf("decode composite ownership: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return compositeOwnership{}, errors.New("trailing composite ownership data")
	}
	canonical, err := canonicalCompositeOwnership(ownership)
	if err != nil || canonical != data {
		return compositeOwnership{}, errors.New("noncanonical composite ownership")
	}
	if ownership.PackID == "" || ownership.PackVersion == "" || (ownership.PortableKind != "skill" && ownership.PortableKind != "command") || ownership.PortableID == "" || ownership.EffectiveName == "" || ownership.TargetType != "claude-personal-skill" {
		return compositeOwnership{}, errors.New("incomplete composite ownership")
	}
	for _, fingerprint := range []string{ownership.SourceFingerprint, ownership.TreeFingerprint, ownership.DefinitionFingerprint} {
		if len(fingerprint) != 64 {
			return compositeOwnership{}, errors.New("invalid composite ownership fingerprint")
		}
		if _, err := hex.DecodeString(fingerprint); err != nil {
			return compositeOwnership{}, errors.New("invalid composite ownership fingerprint")
		}
	}
	return ownership, nil
}

func compositeTreeFiles(skill compositeSkill) []localprojection.TreeFile {
	files := make([]localprojection.TreeFile, len(skill.Files))
	for i, file := range skill.Files {
		files[i] = localprojection.TreeFile{Path: file.Path, Content: append([]byte(nil), file.Content...), Mode: fs.FileMode(file.Mode)}
	}
	return files
}

func decodeCompositeSkillPayload(data []byte) (compositeSkill, error) {
	var p compositeSkillPayload
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return compositeSkill{}, fmt.Errorf("decode composite skill payload: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return compositeSkill{}, errors.New("trailing composite skill payload")
	}
	canonical, err := json.Marshal(p)
	if err != nil || !bytes.Equal(canonical, data) {
		return compositeSkill{}, errors.New("noncanonical composite skill payload")
	}
	if err := canonicalizeCompositeFiles(p.Files); err != nil {
		return compositeSkill{}, err
	}
	if !sort.SliceIsSorted(p.Files, func(i, j int) bool { return p.Files[i].Path < p.Files[j].Path }) {
		return compositeSkill{}, errors.New("unsorted composite skill payload")
	}
	fingerprint, err := fingerprintCompositeFiles(p.Files)
	if err != nil || fingerprint != p.TreeFingerprint {
		return compositeSkill{}, errors.New("composite skill tree fingerprint mismatch")
	}
	if p.Ownership.TreeFingerprint != p.TreeFingerprint || p.Ownership.SourceFingerprint != p.SourceFingerprint || p.Ownership.DefinitionFingerprint != p.DefinitionFingerprint {
		return compositeSkill{}, errors.New("composite skill ownership fingerprint mismatch")
	}
	if _, err := decodeCompositeOwnership(mustCompositeOwnership(p.Ownership)); err != nil {
		return compositeSkill{}, err
	}
	return compositeSkill{p.Files, p.TreeFingerprint, p.SourceFingerprint, p.DefinitionFingerprint, p.Ownership}, nil
}

func mustCompositeOwnership(ownership compositeOwnership) string {
	data, _ := canonicalCompositeOwnership(ownership)
	return data
}
