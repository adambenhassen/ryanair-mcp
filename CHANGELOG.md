# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- `list_airports` gained an optional `code` input that returns a single
  airport's full metadata (city, region, timezone, coordinates), replacing
  `airport_info`.

### Removed

- Tools `find_anywhere_under`, `validate_route`, and `airport_info`.
  `find_anywhere_under` was `search_one_way` with the destination omitted and a
  price cap; `validate_route` is answered by `get_active_dates` (empty = no
  route) or `explore_destinations`; `airport_info` is subsumed by
  `list_airports` with `code`. **Breaking** for any caller relying on these
  tool names.

## [0.3.1] - 2026-07-02

### Changed

- The MCP server now advertises its real release version to clients, stamped
  from the release tag at build time (with a `go install module@vX` build-info
  fallback) instead of a hand-maintained constant.
- Unified the fare/calendar/schedule client surface on parameter structs and
  consolidated repeated route-normalization logic. No change to the MCP tool
  surface or behavior.

[0.3.1]: https://github.com/adambenhassen/ryanair-mcp/releases/tag/v0.3.1

## [0.3.0] - 2026-06-18

### Added

- `explore_destinations` gained a `with_route_details` option that annotates
  each destination with `operator`, `recent`, and `tags` from the searchWidget
  route endpoint — the data previously exposed only by `airport_destinations`.

### Removed

- Tools `nearby_airports`, `default_airport`, `active_airports`, and
  `airport_destinations`. The first two geolocated by the server's IP rather
  than the user's (limited value); `active_airports` duplicated `list_airports`
  with no country filter; and `airport_destinations` is now subsumed by
  `explore_destinations` with `with_route_details`. **Breaking** for any caller
  relying on these tool names.

[0.3.0]: https://github.com/adambenhassen/ryanair-mcp/releases/tag/v0.3.0

## [0.2.0] - 2026-06-17

### Added

- Multi-arch Docker image (`linux/amd64`, `linux/arm64`) published to GHCR on
  release, plus a release workflow that cuts the GitHub release.
- Agent skill (`skills/ryanair-flights/SKILL.md`) following the Agent Skills
  open standard.
- CI (build/vet/race tests/lint/Docker build), Dependabot, and a scheduled
  live smoke workflow that hits the real API daily for drift detection.
- In-process MCP protocol e2e and stdio-subprocess smoke tests.
- Contributor docs (CONTRIBUTING, SECURITY, issue/PR templates).

### Changed

- Bumped `github.com/modelcontextprotocol/go-sdk` 1.2.0 → 1.6.1, and restored
  cross-origin protection on the HTTP transport (the SDK disabled it by default
  in v1.6).

[0.2.0]: https://github.com/adambenhassen/ryanair-mcp/releases/tag/v0.2.0

## [0.1.0] - 2026-06-17

Initial release. Full coverage of Ryanair's anonymous (unauthenticated) API as
MCP tools, over stdio or streamable HTTP.

### Added

- **Fares:** `search_one_way`, `search_return`, `find_anywhere_under`,
  `cheapest_per_day`, `cheapest_return_per_day`, `cheapest_weekend`.
- **Schedules & availability:** `get_active_dates`, `get_schedules`.
- **Network & airports:** `list_airports`, `validate_route`,
  `explore_destinations`, `active_airports`, `airport_info`,
  `airport_destinations`, `nearby_airports`, `default_airport`.
- Price-history fields (`previous_price`, `price_updated`, `new_route`) on fare
  results where Ryanair reports them.
- Resilient client: session priming, User-Agent rotation, capped retries on
  `429`/`5xx`/network errors, and a 6-hour network-bundle cache.
- Build-tagged live smoke test covering every endpoint for drift detection.

[0.1.0]: https://github.com/adambenhassen/ryanair-mcp/releases/tag/v0.1.0
