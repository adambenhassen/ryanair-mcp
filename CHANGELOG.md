# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
