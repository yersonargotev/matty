# 07 — Add CI workflow

Type: task
Status: resolved
Blocked by:

## Question

Add GitHub Actions CI for Matty using the dots CI workflow as the starting point.

## Acceptance criteria

- Runs on pull requests and pushes to `main`.
- Checks out the repo, sets up Go from `go.mod`, checks gofmt, runs `go vet ./...`, builds, and runs `go test ./...`.
- Uses minimal permissions (`contents: read`) and sensible concurrency.
- Any workflow tests/docs are updated if release automation tests inspect CI files.

## Answer

Added `.github/workflows/ci.yml` based on the dots CI pattern. CI runs on pull requests and pushes to `main`, uses minimal `contents: read` permissions, cancels superseded pull-request runs with workflow/ref concurrency, checks out the repo, sets up Go from `go.mod`, verifies gofmt for Matty-owned tracked Go files, runs `go vet ./...`, builds with `go build ./...`, and runs `go test ./...`.
