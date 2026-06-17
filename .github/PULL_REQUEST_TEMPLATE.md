## What

<!-- What does this change do, and why? -->

## How

<!-- Notable implementation choices, trade-offs, or anything reviewers should focus on. -->

## Checklist

- [ ] `go build ./...`, `go test ./...`, and `golangci-lint run` pass
- [ ] New behavior is covered by tests (fixtures under `internal/ryanair/testdata/`)
- [ ] If wire formats or endpoints changed, the live smoke test was run (`go test -tags live ./internal/ryanair/ -v`)
- [ ] No changes under `plans/` are staged (local-only)
