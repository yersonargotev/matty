# 07 — Add CI workflow

Type: task
Status: open
Blocked by:

## Question

Add GitHub Actions CI for Matty using the dots CI workflow as the starting point.

## Acceptance criteria

- Runs on pull requests and pushes to `main`.
- Checks out the repo, sets up Go from `go.mod`, checks gofmt, runs `go vet ./...`, builds, and runs `go test ./...`.
- Uses minimal permissions (`contents: read`) and sensible concurrency.
- Any workflow tests/docs are updated if release automation tests inspect CI files.
