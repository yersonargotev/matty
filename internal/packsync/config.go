package packsync

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

func LoadConfig(r io.Reader) (Config, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode source configuration: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Config{}, err
	}
	if config.SchemaVersion != 1 {
		return Config{}, fmt.Errorf("unsupported source configuration schema %d", config.SchemaVersion)
	}
	if len(config.Sources) == 0 {
		return Config{}, errors.New("source configuration has no sources")
	}
	seenSources := map[string]bool{}
	for i := range config.Sources {
		source := &config.Sources[i]
		if source.ID == "" || seenSources[source.ID] {
			return Config{}, fmt.Errorf("source id %q is empty or duplicated", source.ID)
		}
		seenSources[source.ID] = true
		if source.Provider != "github" {
			return Config{}, fmt.Errorf("source %s has unsupported provider %q", source.ID, source.Provider)
		}
		parts := strings.Split(source.Repository, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return Config{}, fmt.Errorf("source %s repository must be owner/name", source.ID)
		}
		if err := validateSelector(source.Selector); err != nil {
			return Config{}, fmt.Errorf("source %s: %w", source.ID, err)
		}
		seenBindings := map[string]bool{}
		for j := range source.Resources {
			binding := &source.Resources[j]
			if binding.PackID == "" || binding.Kind == "" || binding.ResourceID == "" {
				return Config{}, fmt.Errorf("source %s has incomplete binding", source.ID)
			}
			if !safeSlashPath(binding.UpstreamPath) {
				return Config{}, fmt.Errorf("source %s binding %s has unsafe upstream path %q", source.ID, bindingKey(*binding), binding.UpstreamPath)
			}
			key := bindingKey(*binding)
			if seenBindings[key] {
				return Config{}, fmt.Errorf("source %s duplicates binding %s", source.ID, key)
			}
			seenBindings[key] = true
		}
		sort.Slice(source.Resources, func(i, j int) bool { return bindingKey(source.Resources[i]) < bindingKey(source.Resources[j]) })
	}
	sort.Slice(config.Sources, func(i, j int) bool { return config.Sources[i].ID < config.Sources[j].ID })
	return config, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("source configuration contains multiple JSON values")
		}
		return fmt.Errorf("decode trailing source configuration: %w", err)
	}
	return nil
}

func validateSelector(selector Selector) error {
	switch selector.Mode {
	case SelectorStableRelease:
		if selector.Ref != "" {
			return errors.New("stable-release selector cannot carry a ref")
		}
	case SelectorPrerelease:
		if selector.Ref == "" || strings.HasPrefix(selector.Ref, "refs/") {
			return errors.New("prerelease selector requires one exact published tag")
		}
	case SelectorCommit:
		if len(selector.Ref) != 40 || strings.Trim(selector.Ref, "0123456789abcdef") != "" {
			return errors.New("commit selector requires a full lowercase 40-character SHA")
		}
	default:
		return fmt.Errorf("floating or unknown selector %q is forbidden", selector.Mode)
	}
	return nil
}

func safeSlashPath(value string) bool {
	return value != "" && !strings.HasPrefix(value, "/") && !strings.Contains(value, "\\") && path.Clean(value) == value && value != "." && !strings.HasPrefix(value, "../")
}

func bindingKey(binding Binding) string {
	return binding.PackID + "/" + binding.Kind + "/" + binding.ResourceID
}
