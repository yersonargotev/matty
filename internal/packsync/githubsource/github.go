// Package githubsource implements read-only acquisition from public GitHub
// repositories using a caller-supplied HTTP client. It parses metadata and
// archives as inert data and never invokes Git, shells, hooks, tests, or
// upstream programs.
package githubsource

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/packsync"
)

const maxArchiveBytes = 256 << 20

type Client struct {
	httpClient *http.Client
	apiBase    string
}

// HTTPError preserves rate-limit and service response metadata for the
// workflow-owned retry policy without making acquisition decide whether a
// failure is retryable.
type HTTPError struct {
	Operation          string
	StatusCode         int
	Status             string
	RetryAfter         string
	RateLimitRemaining string
	RateLimitReset     string
}

func (failure HTTPError) Error() string {
	return fmt.Sprintf("%s: GitHub returned %s", failure.Operation, failure.Status)
}

func New(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{httpClient: httpClient, apiBase: "https://api.github.com"}
}

func newClient(httpClient *http.Client, apiBase string) *Client {
	return &Client{httpClient: httpClient, apiBase: strings.TrimRight(apiBase, "/")}
}

func (client *Client) Releases(ctx context.Context, source packsync.SourceConfig) ([]packsync.Release, error) {
	type releaseResponse struct {
		ID          int64  `json:"id"`
		NodeID      string `json:"node_id"`
		Tag         string `json:"tag_name"`
		Name        string `json:"name"`
		Target      string `json:"target_commitish"`
		Immutable   bool   `json:"immutable"`
		CreatedAt   string `json:"created_at"`
		PublishedAt string `json:"published_at"`
		Draft       bool   `json:"draft"`
		Prerelease  bool   `json:"prerelease"`
		Author      actor  `json:"author"`
	}
	var releases []packsync.Release
	for page := 1; ; page++ {
		var response []releaseResponse
		endpoint := client.repoURL(source) + "/releases?per_page=100&page=" + strconv.Itoa(page)
		if err := client.getJSON(ctx, endpoint, &response); err != nil {
			return nil, err
		}
		for _, item := range response {
			var created, published time.Time
			if item.CreatedAt != "" {
				parsed, err := time.Parse(time.RFC3339, item.CreatedAt)
				if err != nil {
					return nil, fmt.Errorf("release %s has invalid created_at: %w", item.Tag, err)
				}
				created = parsed
			}
			if item.PublishedAt != "" {
				parsed, err := time.Parse(time.RFC3339, item.PublishedAt)
				if err != nil {
					return nil, fmt.Errorf("release %s has invalid published_at: %w", item.Tag, err)
				}
				published = parsed
			}
			releases = append(releases, packsync.Release{ID: item.ID, NodeID: item.NodeID, Tag: item.Tag, Name: item.Name, Target: item.Target, Immutable: item.Immutable, CreatedAt: created, PublishedAt: published, Draft: item.Draft, Prerelease: item.Prerelease, Author: item.Author.toActor()})
		}
		if len(response) < 100 {
			break
		}
	}
	return releases, nil
}

func (client *Client) ResolveRelease(ctx context.Context, source packsync.SourceConfig, release packsync.Release) (packsync.Candidate, error) {
	if release.Tag == "" {
		return packsync.Candidate{}, errors.New("release tag is required")
	}
	candidate, err := client.repositoryCandidate(ctx, source)
	if err != nil {
		return packsync.Candidate{}, err
	}
	tagPath := url.PathEscape(release.Tag)
	var ref struct {
		Object gitObject `json:"object"`
	}
	if err := client.getJSON(ctx, client.repoURL(source)+"/git/ref/tags/"+tagPath, &ref); err != nil {
		return packsync.Candidate{}, fmt.Errorf("resolve exact tag ref: %w", err)
	}
	candidate.Release = &release
	candidate.TagRefName = "refs/tags/" + release.Tag
	candidate.TagRefType = ref.Object.Type
	candidate.TagRefSHA = ref.Object.SHA
	object := ref.Object
	seen := map[string]bool{}
	for object.Type == "tag" {
		if object.SHA == "" || seen[object.SHA] {
			return packsync.Candidate{}, errors.New("tag chain is empty or cyclic")
		}
		seen[object.SHA] = true
		var tag struct {
			SHA          string       `json:"sha"`
			Name         string       `json:"tag"`
			Object       gitObject    `json:"object"`
			Verification verification `json:"verification"`
		}
		if err := client.getJSON(ctx, client.repoURL(source)+"/git/tags/"+object.SHA, &tag); err != nil {
			return packsync.Candidate{}, fmt.Errorf("peel tag object: %w", err)
		}
		verification, err := tag.Verification.toEvidence()
		if err != nil {
			return packsync.Candidate{}, fmt.Errorf("decode tag verification: %w", err)
		}
		candidate.TagObjects = append(candidate.TagObjects, packsync.TagObject{SHA: tag.SHA, Name: tag.Name, TargetSHA: tag.Object.SHA, TargetType: tag.Object.Type, Verification: verification})
		object = tag.Object
	}
	if object.Type != "commit" || len(object.SHA) != 40 {
		return packsync.Candidate{}, errors.New("release tag did not peel to one full commit SHA")
	}
	if err := client.addCommit(ctx, source, object.SHA, &candidate); err != nil {
		return packsync.Candidate{}, err
	}
	return candidate, nil
}

func (client *Client) ResolveCommit(ctx context.Context, source packsync.SourceConfig, sha string) (packsync.Candidate, error) {
	if len(sha) != 40 || strings.Trim(sha, "0123456789abcdef") != "" {
		return packsync.Candidate{}, errors.New("commit acquisition requires a full lowercase 40-character SHA")
	}
	candidate, err := client.repositoryCandidate(ctx, source)
	if err != nil {
		return packsync.Candidate{}, err
	}
	if err := client.addCommit(ctx, source, sha, &candidate); err != nil {
		return packsync.Candidate{}, err
	}
	if candidate.Commit != sha {
		return packsync.Candidate{}, errors.New("GitHub returned a different commit SHA")
	}
	return candidate, nil
}

func (client *Client) WithSnapshot(ctx context.Context, candidate packsync.Candidate, temporaryRoot string, visit func(string) error) (result error) {
	if visit == nil {
		return errors.New("snapshot visitor is required")
	}
	if err := emptyDirectory(temporaryRoot); err != nil {
		return err
	}
	defer func() {
		if cleanupErr := cleanDirectory(temporaryRoot); result == nil && cleanupErr != nil {
			result = cleanupErr
		}
	}()
	request, err := client.request(ctx, client.apiBase+"/repos/"+candidate.Repository+"/tarball/"+candidate.Commit)
	if err != nil {
		return err
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return newHTTPError("download immutable archive", response)
	}
	snapshot := filepath.Join(temporaryRoot, "snapshot")
	if err := os.Mkdir(snapshot, 0o755); err != nil {
		return err
	}
	if err := extractArchive(io.LimitReader(response.Body, maxArchiveBytes+1), snapshot); err != nil {
		return fmt.Errorf("extract inert archive: %w", err)
	}
	return visit(snapshot)
}

type gitObject struct {
	SHA  string `json:"sha"`
	Type string `json:"type"`
}

type verification struct {
	Verified   bool    `json:"verified"`
	Reason     string  `json:"reason"`
	VerifiedAt *string `json:"verified_at"`
	Signature  *string `json:"signature"`
	Payload    *string `json:"payload"`
}

func (value verification) toEvidence() (packsync.Verification, error) {
	result := packsync.Verification{Verified: value.Verified, Reason: value.Reason, SignatureSHA256: hashOptional(value.Signature), PayloadSHA256: hashOptional(value.Payload)}
	if value.VerifiedAt != nil {
		parsed, err := time.Parse(time.RFC3339, *value.VerifiedAt)
		if err != nil {
			return packsync.Verification{}, err
		}
		result.VerifiedAt = &parsed
	}
	return result, nil
}

type actor struct {
	Login  string `json:"login"`
	ID     int64  `json:"id"`
	NodeID string `json:"node_id"`
}

func (value actor) toActor() packsync.Actor {
	return packsync.Actor{Login: value.Login, ID: value.ID, NodeID: value.NodeID}
}

func (client *Client) repositoryCandidate(ctx context.Context, source packsync.SourceConfig) (packsync.Candidate, error) {
	var repository struct {
		ID         int64  `json:"id"`
		NodeID     string `json:"node_id"`
		FullName   string `json:"full_name"`
		HTMLURL    string `json:"html_url"`
		CloneURL   string `json:"clone_url"`
		APIURL     string `json:"url"`
		Archived   bool   `json:"archived"`
		Disabled   bool   `json:"disabled"`
		Visibility string `json:"visibility"`
		Private    bool   `json:"private"`
		Owner      actor  `json:"owner"`
	}
	if err := client.getJSON(ctx, client.repoURL(source), &repository); err != nil {
		return packsync.Candidate{}, fmt.Errorf("observe repository identity: %w", err)
	}
	return packsync.Candidate{Repository: repository.FullName, RepositoryID: repository.ID, RepositoryNodeID: repository.NodeID, RepositoryHTML: repository.HTMLURL, RepositoryClone: repository.CloneURL, RepositoryAPI: repository.APIURL, Visibility: repository.Visibility, Owner: repository.Owner.Login, OwnerID: repository.Owner.ID, OwnerNodeID: repository.Owner.NodeID, Public: !repository.Private && repository.Visibility == "public", Archived: repository.Archived, Disabled: repository.Disabled}, nil
}

func (client *Client) addCommit(ctx context.Context, source packsync.SourceConfig, sha string, candidate *packsync.Candidate) error {
	var commit struct {
		SHA          string       `json:"sha"`
		NodeID       string       `json:"node_id"`
		Tree         gitObject    `json:"tree"`
		Parents      []gitObject  `json:"parents"`
		Verification verification `json:"verification"`
	}
	if err := client.getJSON(ctx, client.repoURL(source)+"/git/commits/"+sha, &commit); err != nil {
		return fmt.Errorf("observe exact commit: %w", err)
	}
	candidate.Commit = commit.SHA
	candidate.CommitNodeID = commit.NodeID
	candidate.Tree = commit.Tree.SHA
	verification, err := commit.Verification.toEvidence()
	if err != nil {
		return fmt.Errorf("decode commit verification: %w", err)
	}
	candidate.CommitVerify = verification
	for _, parent := range commit.Parents {
		candidate.Parents = append(candidate.Parents, parent.SHA)
	}
	return nil
}

func hashOptional(value *string) *string {
	if value == nil {
		return nil
	}
	sum := sha256.Sum256([]byte(*value))
	encoded := hex.EncodeToString(sum[:])
	return &encoded
}

func (client *Client) repoURL(source packsync.SourceConfig) string {
	return client.apiBase + "/repos/" + source.Repository
}

func (client *Client) getJSON(ctx context.Context, endpoint string, target any) error {
	request, err := client.request(ctx, endpoint)
	if err != nil {
		return err
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return newHTTPError("read GitHub API", response)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 8<<20))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode GitHub response: %w", err)
	}
	return nil
}

func newHTTPError(operation string, response *http.Response) HTTPError {
	return HTTPError{
		Operation:          operation,
		StatusCode:         response.StatusCode,
		Status:             response.Status,
		RetryAfter:         response.Header.Get("Retry-After"),
		RateLimitRemaining: response.Header.Get("X-RateLimit-Remaining"),
		RateLimitReset:     response.Header.Get("X-RateLimit-Reset"),
	}
}

func (client *Client) request(ctx context.Context, endpoint string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", packsync.GitHubAPIVersion)
	request.Header.Set("User-Agent", "packy-pack-source-check")
	return request, nil
}

func extractArchive(source io.Reader, destination string) error {
	gzipReader, err := gzip.NewReader(source)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	root := ""
	seen := map[string]bool{}
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeXGlobalHeader || header.Typeflag == tar.TypeXHeader {
			continue
		}
		clean := path.Clean(header.Name)
		parts := strings.Split(clean, "/")
		if clean == "." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) || len(parts) == 0 {
			return fmt.Errorf("unsafe archive path %q", header.Name)
		}
		if root == "" {
			root = parts[0]
		}
		if parts[0] != root {
			return errors.New("archive has multiple roots")
		}
		if len(parts) == 1 {
			if header.Typeflag != tar.TypeDir {
				return errors.New("archive root is not a directory")
			}
			continue
		}
		relative := path.Join(parts[1:]...)
		if !safeArchivePath(relative) || seen[relative] {
			return fmt.Errorf("unsafe or duplicate archive path %q", relative)
		}
		seen[relative] = true
		archiveMode := os.FileMode(header.Mode) & os.ModePerm
		if header.Mode&0o7000 != 0 {
			return fmt.Errorf("unsafe archive permissions %04o for %s", archiveMode, relative)
		}
		target := filepath.Join(destination, filepath.FromSlash(relative))
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			// GitHub's generated tarballs commonly add group-write bits. They
			// are transport metadata, not repository tree modes, so normalize
			// writes while retaining the executable bit as evidence.
			mode := os.FileMode(0o644)
			if archiveMode&0o111 != 0 {
				mode = 0o755
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(file, reader, header.Size)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("archive contains forbidden link or special entry %s", relative)
		}
	}
	if root == "" {
		return errors.New("archive is empty")
	}
	return nil
}

func safeArchivePath(value string) bool {
	return value != "" && value != "." && !path.IsAbs(value) && path.Clean(value) == value && !strings.HasPrefix(value, "../") && !strings.Contains(value, "\\")
}

func emptyDirectory(name string) error {
	info, err := os.Lstat(name)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("caller-supplied acquisition area must be a real directory")
	}
	entries, err := os.ReadDir(name)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("caller-supplied acquisition area must be empty")
	}
	return nil
}

func cleanDirectory(name string) error {
	entries, err := os.ReadDir(name)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(name, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
