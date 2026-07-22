package governanceauth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixture struct {
	Event    Event    `json:"event"`
	Metadata Metadata `json:"metadata"`
	Want     string   `json:"want_error"`
}

func TestAuthorizationFixtures(t *testing.T) {
	paths, err := filepath.Glob("testdata/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no authorization fixtures found")
	}

	for _, path := range paths {
		path := path
		t.Run(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), func(t *testing.T) {
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var test fixture
			if err := json.Unmarshal(contents, &test); err != nil {
				t.Fatal(err)
			}

			err = Validate(test.Event, test.Metadata)
			if test.Want == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.Want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.Want)
			}
		})
	}
}
