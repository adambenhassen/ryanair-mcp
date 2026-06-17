# Contributing

Thanks for your interest. This is an unofficial client for Ryanair's
undocumented public endpoints; see the [README](README.md) for scope.

## Development

```sh
go build ./...       # build
go test ./...        # unit tests (fixtures, no network)
go vet ./...         # vet
golangci-lint run    # lint (strict config in .golangci.yml)
```

A build-tagged live smoke test hits the real Ryanair endpoints to catch
wire-format or endpoint drift. Run it when you touch parsing or add an endpoint:

```sh
go test -tags live ./internal/ryanair/ -v
```

## Conventions

- **Tests first.** New behavior and bug fixes come with tests; fixtures live in
  `internal/ryanair/testdata/`.
- **External test packages.** Test files use `package foo_test`; reach internal
  helpers through an `export_test.go` bridge.
- **Layering.** Wire-format quirks stay in `internal/ryanair`; `internal/tools`
  deals only in clean domain types. Validate external input, trust internal calls.
- **Errors are values.** Handle them explicitly — no ignored errors.
- **Conventional Commits.** e.g. `feat(ryanair): …`, `fix: …`, `docs: …`.
- **`plans/` is local-only** and git-ignored — never commit it.

## Pull requests

Make sure `go build`, `go test ./...`, and `golangci-lint run` all pass before
opening a PR. Keep changes focused and explain the "why".
