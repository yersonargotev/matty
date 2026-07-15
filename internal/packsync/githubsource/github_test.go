package githubsource

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/matty/internal/packsync"
)

const (
	commitSHA = "d574778f94cf620fcc8ce741584093bc650a61d3"
	tagSHA    = "eabea89380927aadb93abf6e290a19334d249292"
)

func TestPublicGitHubAcquisitionUsesNoCredentialsNeverExecutesAndCleans(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "executed")
	archive := archiveBytes(t, []tarEntry{
		{name: "repo-root/", mode: 0o775, kind: tar.TypeDir},
		{name: "repo-root/skills/hostile/SKILL.md", mode: 0o664, data: []byte("never execute\n")},
		{name: "repo-root/hostile.sh", mode: 0o775, data: []byte("#!/bin/sh\ntouch " + sentinel + "\n")},
	})
	var sawAuthorization bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "" {
			sawAuthorization = true
		}
		switch request.URL.Path {
		case "/repos/mattpocock/skills":
			writeJSON(writer, map[string]any{"id": 1148788086, "node_id": "repo-node", "full_name": "mattpocock/skills", "html_url": "https://github.com/mattpocock/skills", "clone_url": "https://github.com/mattpocock/skills.git", "url": "https://api.github.com/repos/mattpocock/skills", "visibility": "public", "private": false, "archived": false, "disabled": false, "owner": map[string]any{"id": 28293365, "node_id": "owner-node", "login": "mattpocock"}})
		case "/repos/mattpocock/skills/releases":
			writeJSON(writer, []map[string]any{{"id": 1, "node_id": "release-node", "tag_name": "v1.1.0", "name": "v1.1.0", "target_commitish": "main", "immutable": false, "created_at": "2026-07-08T13:20:55Z", "published_at": "2026-07-08T13:20:57Z", "draft": false, "prerelease": false, "author": map[string]any{"login": "bot", "id": 7, "node_id": "author-node"}}})
		case "/repos/mattpocock/skills/git/ref/tags/v1.1.0":
			writeJSON(writer, map[string]any{"object": map[string]any{"sha": tagSHA, "type": "tag"}})
		case "/repos/mattpocock/skills/git/tags/" + tagSHA:
			writeJSON(writer, map[string]any{"sha": tagSHA, "tag": "v1.1.0", "object": map[string]any{"sha": commitSHA, "type": "commit"}, "verification": map[string]any{"verified": false, "reason": "unsigned"}})
		case "/repos/mattpocock/skills/git/commits/" + commitSHA:
			writeJSON(writer, map[string]any{"sha": commitSHA, "node_id": "commit-node", "tree": map[string]any{"sha": strings.Repeat("f", 40)}, "parents": []map[string]any{{"sha": strings.Repeat("a", 40)}}, "verification": map[string]any{"verified": true, "reason": "valid", "verified_at": "2026-07-08T13:20:40Z", "signature": "signature", "payload": "payload"}})
		case "/repos/mattpocock/skills/tarball/" + commitSHA:
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write(archive)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := newClient(server.Client(), server.URL)
	source := packsync.SourceConfig{Repository: "mattpocock/skills"}
	releases, err := client.Releases(context.Background(), source)
	if err != nil || len(releases) != 1 {
		t.Fatalf("releases = %#v, %v", releases, err)
	}
	candidate, err := client.ResolveRelease(context.Background(), source, releases[0])
	if err != nil {
		t.Fatal(err)
	}
	if candidate.RepositoryID != 1148788086 || candidate.RepositoryNodeID != "repo-node" || candidate.OwnerID != 28293365 || candidate.OwnerNodeID != "owner-node" || candidate.Commit != commitSHA || candidate.CommitNodeID != "commit-node" || candidate.TagRefSHA != tagSHA || candidate.CommitVerify.SignatureSHA256 == nil || candidate.CommitVerify.PayloadSHA256 == nil {
		t.Fatalf("candidate = %#v", candidate)
	}
	temporary := t.TempDir()
	err = client.WithSnapshot(context.Background(), candidate, temporary, func(root string) error {
		data, err := os.ReadFile(filepath.Join(root, "hostile.sh"))
		if err != nil || !bytes.Contains(data, []byte("touch")) {
			t.Fatalf("inert hostile bytes unavailable: %q, %v", data, err)
		}
		info, err := os.Stat(filepath.Join(root, "hostile.sh"))
		if err != nil || info.Mode().Perm() != 0o755 {
			t.Fatalf("normalized executable evidence mode = %v, %v", info.Mode(), err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(temporary)
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary acquisition area not cleaned: %#v, %v", entries, err)
	}
	if _, err := os.Stat(sentinel); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("upstream content executed: %v", err)
	}
	if sawAuthorization {
		t.Fatal("public acquisition sent credentials")
	}
	if err := client.WithSnapshot(context.Background(), candidate, temporary, func(string) error { return errors.New("visitor failed") }); err == nil {
		t.Fatal("visitor failure was lost")
	}
	entries, err = os.ReadDir(temporary)
	if err != nil || len(entries) != 0 {
		t.Fatalf("visitor failure did not clean acquisition area: %#v, %v", entries, err)
	}
}

func TestAcquisitionRejectsUnsafeArchiveAndCleansEveryFailure(t *testing.T) {
	for _, test := range []struct {
		name    string
		entries []tarEntry
	}{
		{name: "symlink", entries: []tarEntry{{name: "root/", mode: 0o755, kind: tar.TypeDir}, {name: "root/link", mode: 0o777, kind: tar.TypeSymlink, link: "../escape"}}},
		{name: "setuid", entries: []tarEntry{{name: "root/", mode: 0o755, kind: tar.TypeDir}, {name: "root/file", mode: 0o4644, data: []byte("x")}}},
		{name: "parent", entries: []tarEntry{{name: "root/", mode: 0o755, kind: tar.TypeDir}, {name: "root/../escape", mode: 0o644, data: []byte("x")}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			archive := archiveBytes(t, test.entries)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { _, _ = writer.Write(archive) }))
			defer server.Close()
			client := newClient(server.Client(), server.URL)
			temporary := t.TempDir()
			candidate := packsync.Candidate{Repository: "o/r", Commit: commitSHA}
			if err := client.WithSnapshot(context.Background(), candidate, temporary, func(string) error { return nil }); err == nil {
				t.Fatal("unsafe archive unexpectedly accepted")
			}
			entries, err := os.ReadDir(temporary)
			if err != nil || len(entries) != 0 {
				t.Fatalf("failed acquisition was not cleaned: %#v, %v", entries, err)
			}
		})
	}
}

func TestResolveCommitRequiresFullExactSHA(t *testing.T) {
	client := New(nil)
	if _, err := client.ResolveCommit(context.Background(), packsync.SourceConfig{}, "abc"); err == nil {
		t.Fatal("abbreviated SHA accepted")
	}
}

func TestHTTPFailurePreservesRetryAfterWithoutChoosingRetryPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Retry-After", "9")
		writer.Header().Set("X-RateLimit-Remaining", "0")
		writer.Header().Set("X-RateLimit-Reset", "4102444800")
		writer.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := newClient(server.Client(), server.URL)
	_, err := client.Releases(context.Background(), packsync.SourceConfig{Repository: "o/r"})
	var responseError HTTPError
	if !errors.As(err, &responseError) || responseError.StatusCode != http.StatusTooManyRequests || responseError.RetryAfter != "9" || responseError.RateLimitRemaining != "0" || responseError.RateLimitReset != "4102444800" {
		t.Fatalf("HTTP error = %#v, %v", responseError, err)
	}
}

func TestReleasesFollowsEveryPageNeededForStableDiscovery(t *testing.T) {
	requestedSecondPage := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		page := request.URL.Query().Get("page")
		if page == "1" {
			items := make([]map[string]any, 100)
			for i := range items {
				items[i] = map[string]any{"id": i + 1, "tag_name": "preview", "published_at": "2026-07-14T00:00:00Z", "draft": false, "prerelease": true}
			}
			writeJSON(writer, items)
			return
		}
		if page == "2" {
			requestedSecondPage = true
			writeJSON(writer, []map[string]any{{"id": 101, "tag_name": "v1.0.0", "published_at": "2026-07-01T00:00:00Z", "draft": false, "prerelease": false}})
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()
	client := newClient(server.Client(), server.URL)
	releases, err := client.Releases(context.Background(), packsync.SourceConfig{Repository: "o/r"})
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 101 || !requestedSecondPage || releases[100].Tag != "v1.0.0" {
		t.Fatalf("complete releases = %d, second-page=%t, last=%#v", len(releases), requestedSecondPage, releases[len(releases)-1])
	}
}

type tarEntry struct {
	name string
	mode int64
	kind byte
	data []byte
	link string
}

func archiveBytes(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		kind := entry.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		header := &tar.Header{Name: entry.name, Mode: entry.mode, Typeflag: kind, Size: int64(len(entry.data)), Linkname: entry.link}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if len(entry.data) > 0 {
			if _, err := tarWriter.Write(entry.data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func writeJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}
