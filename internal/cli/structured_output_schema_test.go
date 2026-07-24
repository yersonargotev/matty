package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/yersonargotev/packy/internal/setuphealth"
)

var structuredOutputFixtures = map[string]string{
	"classic-lifecycle-preview.json": "classic-lifecycle.schema.json",
	"classic-lifecycle-result.json":  "classic-lifecycle.schema.json",
	"doctor.json":                    "doctor.schema.json",
	"pack-show.json":                 "pack-show.schema.json",
	"pack-lifecycle-preview.json":    "pack-lifecycle.schema.json",
	"pack-status.json":               "pack-status.schema.json",
}

func TestStructuredOutputV2SchemasValidateFixturesAndProducers(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fixtureRoot := filepath.Join("testdata", "structured-output", "v2")
	for fixtureName, schemaName := range structuredOutputFixtures {
		fixture, err := os.ReadFile(filepath.Join(fixtureRoot, fixtureName))
		if err != nil {
			t.Fatal(err)
		}
		if err := validateStructuredOutput(t, root, schemaName, fixture); err != nil {
			t.Fatalf("fixture %s: %v", fixtureName, err)
		}
		for _, forbidden := range []string{"TOKEN=", "SECRET=", "/Users/", "foreign-document", "mixed-store"} {
			if strings.Contains(string(fixture), forbidden) {
				t.Fatalf("fixture %s leaks %q", fixtureName, forbidden)
			}
		}
	}

	opts, _, _ := sandboxOptions(t)
	classic, err := executeCommand(t, NewRootCommand(opts), "install", "--dry-run", "--json")
	if err != nil {
		t.Fatal(err)
	}
	assertStructuredOutput(t, root, "classic-lifecycle.schema.json", classic)

	opts.SetupHealthDiagnose = func() (setuphealth.Report, error) {
		return setuphealth.Report{SchemaVersion: 2, Kind: "doctor", Checks: []setuphealth.Check{{Name: "claude-readiness", Severity: setuphealth.Warn, Detail: "runtime usability is unknown; start Claude Code explicitly"}}, Summary: setuphealth.Summary{Status: "warnings", Warnings: 1}}, nil
	}
	doctor, err := executeCommand(t, NewRootCommand(opts), "doctor", "--json")
	if err != nil {
		t.Fatal(err)
	}
	assertStructuredOutput(t, root, "doctor.schema.json", doctor)

	packReadOpts := Options{Env: MapEnv{"HOME": t.TempDir(), "XDG_CONFIG_HOME": filepath.Join(t.TempDir(), "xdg"), "PATH": "", "PACKY_SKILLS_SOURCE": filepath.Join(root, "bundle", "skills")}}
	show, err := executeCommand(t, NewRootCommand(packReadOpts), "pack", "show", "engram", "--json")
	if err != nil {
		t.Fatal(err)
	}
	assertStructuredOutput(t, root, "pack-show.schema.json", show)

	status, err := executeCommand(t, NewRootCommand(packReadOpts), "pack", "status", "ma"+"tty", "--surface", "claude", "--json")
	if err != nil {
		t.Fatal(err)
	}
	assertStructuredOutput(t, root, "pack-status.schema.json", status)

	packOpts, _, _ := packActivationOptions(t, &fakeTerminal{})
	preview, err := executeCommand(t, NewRootCommand(packOpts), "pack", "activate", "ma"+"tty", "--surface", "claude", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("pack preview: %v\n%s", err, preview)
	}
	assertStructuredOutput(t, root, "pack-lifecycle.schema.json", preview)
}

func TestStructuredOutputV2SchemasRejectWrongVersionAndUnknownFields(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	for _, document := range []string{
		`{"schema_version":1,"report":"doctor","checks":[],"summary":{"status":"healthy","passes":0,"warnings":0,"failures":0}}`,
		`{"schema_version":2,"report":"doctor","checks":[],"summary":{"status":"healthy","passes":0,"warnings":0,"failures":0},"unknown":true}`,
	} {
		if err := validateStructuredOutput(t, root, "doctor.schema.json", []byte(document)); err == nil {
			t.Fatalf("invalid document passed: %s", document)
		}
	}
}

func TestStructuredOutputV2SchemasRejectMismatchedReadinessState(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	fixture, err := os.ReadFile(filepath.Join("testdata", "structured-output", "v2", "pack-status.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(fixture, &document); err != nil {
		t.Fatal(err)
	}
	entry := document["entries"].([]any)[0].(map[string]any)
	readiness := entry["readiness"].(map[string]any)
	for name, invalid := range map[string]map[string]any{
		"unknown boolean": {"state": "unknown", "value": true},
		"known null":      {"state": "known", "value": nil},
	} {
		t.Run(name, func(t *testing.T) {
			original := readiness["configured"]
			readiness["configured"] = invalid
			defer func() { readiness["configured"] = original }()
			encoded, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateStructuredOutput(t, root, "pack-status.schema.json", encoded); err == nil {
				t.Fatalf("mismatched readiness passed: %s", encoded)
			}
		})
	}
}

func TestPackOperatorSchemasRejectCanonicalNegativeTwins(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	fixtureRoot := filepath.Join("testdata", "structured-output", "v2")
	load := func(t *testing.T, name string) map[string]any {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(fixtureRoot, name))
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]any
		if err := json.Unmarshal(data, &document); err != nil {
			t.Fatal(err)
		}
		return document
	}
	reject := func(t *testing.T, schema string, document map[string]any) {
		t.Helper()
		encoded, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateStructuredOutput(t, root, schema, encoded); err == nil {
			t.Fatalf("negative twin passed %s: %s", schema, encoded)
		}
	}

	t.Run("unknown fact", func(t *testing.T) {
		document := load(t, "pack-show.json")
		document["unknown"] = true
		reject(t, "pack-show.schema.json", document)
	})
	t.Run("duplicate fact", func(t *testing.T) {
		document := load(t, "pack-show.json")
		surfaces := document["surfaces"].([]any)
		document["surfaces"] = append(surfaces, surfaces[0])
		reject(t, "pack-show.schema.json", document)
	})
	t.Run("missing fact", func(t *testing.T) {
		document := load(t, "pack-show.json")
		delete(document, "source_identity")
		reject(t, "pack-show.schema.json", document)
	})
	t.Run("contradictory fact", func(t *testing.T) {
		document := load(t, "pack-show.json")
		intent := document["surface_contracts"].([]any)[0].(map[string]any)["intent"].(map[string]any)
		intent["state"] = "known"
		reject(t, "pack-show.schema.json", document)
	})
	t.Run("unredacted ambient target", func(t *testing.T) {
		document := load(t, "pack-status.json")
		detail := document["entries"].([]any)[0].(map[string]any)["projection_details"].([]any)[0].(map[string]any)
		detail["target"] = "/Users/operator/.claude/skills/example"
		reject(t, "pack-status.schema.json", document)
	})
}

func assertStructuredOutput(t *testing.T, root, schemaName, document string) {
	t.Helper()
	if err := validateStructuredOutput(t, root, schemaName, []byte(document)); err != nil {
		t.Fatalf("%s producer: %v\n%s", schemaName, err, document)
	}
}

func validateStructuredOutput(t *testing.T, root, schemaName string, instance []byte) error {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	schemaRoot := filepath.Join(root, "schemas", "cli", "v2")
	entries, err := os.ReadDir(schemaRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		schemaBytes, err := os.ReadFile(filepath.Join(schemaRoot, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaBytes))
		if err != nil {
			t.Fatalf("parse schema %s: %v", entry.Name(), err)
		}
		if err := compiler.AddResource("https://yersonargotev.github.io/packy/schemas/cli/v2/"+entry.Name(), document); err != nil {
			t.Fatal(err)
		}
	}
	schema, err := compiler.Compile("https://yersonargotev.github.io/packy/schemas/cli/v2/" + schemaName)
	if err != nil {
		t.Fatalf("compile schema %s: %v", schemaName, err)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(instance))
	if err != nil {
		return err
	}
	if encoded, err := json.Marshal(value); err != nil || !json.Valid(encoded) {
		t.Fatalf("invalid decoded JSON: %v", err)
	}
	return schema.Validate(value)
}
