<div align="center">

<img src="assets/logo.svg" alt="ryanair-mcp logo" width="128" height="128">

# ryanair-mcp

**An [MCP](https://modelcontextprotocol.io) server that exposes Ryanair's anonymous flight APIs as tools an LLM can call.**

Fare search · price calendars · timetables · the airport/route network — over **stdio** or **streamable HTTP**.

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![MCP](https://img.shields.io/badge/MCP-server-6E56CF)](https://modelcontextprotocol.io)
[![License: MIT](https://img.shields.io/badge/License-MIT-green)](LICENSE)
![Status](https://img.shields.io/badge/status-unofficial-orange)

</div>

> [!WARNING]
> **Unofficial.** This project consumes Ryanair's public, unauthenticated
> endpoints and is not affiliated with or endorsed by Ryanair. The APIs are
> undocumented and may change or rate-limit without notice.

## Contents

- [Features](#features)
- [Install](#install)
- [Usage](#usage)
- [Tools](#tools)
- [Agent skill](#agent-skill)
- [How it works](#how-it-works)
- [Development](#development)
- [Project layout](#project-layout)
- [Limitations](#limitations)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [License](#license)

## Features

- **16 read tools** covering Ryanair's full anonymous API surface — one-way and
  return fares, price calendars, cheapest-weekend search, timetables, and the
  airport/route network.
- **Price history** — fares carry `previous_price` / `price_updated` /
  `new_route` when Ryanair reports them, so callers can spot price drops and
  newly-added routes.
- **Resilient client** — session priming, User-Agent rotation, capped retries,
  and a cached network bundle (see [How it works](#how-it-works)).
- **Two transports** — stdio (default) for clients that spawn a subprocess, or
  streamable HTTP.
- **No keys, no accounts** — everything runs against public endpoints.

## Install

Requires Go 1.26+.

```sh
go build -o ryanair-mcp ./cmd/ryanair-mcp
```

## Usage

```sh
# stdio (default) — for MCP clients that spawn the server as a subprocess
./ryanair-mcp

# streamable HTTP
./ryanair-mcp --transport http --addr :8080
```

| Flag | Default | Description |
| --- | --- | --- |
| `--transport` | `stdio` | `stdio` or `http` |
| `--addr` | `:8080` | listen address (HTTP transport only) |

### With an MCP client

For a stdio client (e.g. Claude Desktop), point it at the built binary:

```json
{
  "mcpServers": {
    "ryanair": {
      "command": "/absolute/path/to/ryanair-mcp"
    }
  }
}
```

## Tools

| Tool | What it does | Key inputs |
| --- | --- | --- |
| `search_one_way` | Cheapest one-way fares from an origin in a date window. Omit destination/country to search anywhere. | `origin`, `date_from`, `date_to`, *(opt)* `destination`, `country`, `max_price`, `currency` |
| `search_return` | Cheapest return fares across outbound and inbound windows, with optional trip-length bounds. | `origin`, `date_from`, `date_to`, `return_from`, `return_to`, *(opt)* `min_trip_days`, `max_trip_days`, … |
| `find_anywhere_under` | Cheapest one-way fare to each reachable destination from an origin under a price cap, in a date window — returns a list of flights, one per destination, sorted by price. | `origin`, `date_from`, `date_to`, `max_price`, *(opt)* `currency` |
| `cheapest_per_day` | Cheapest one-way fare per day of a month on a route (price calendar). | `origin`, `destination`, `month` (`YYYY-MM-01`), *(opt)* `currency` |
| `cheapest_return_per_day` | Cheapest return fare per day on a route, outbound and inbound calendars side by side. | `origin`, `destination`, `outbound_month` (`YYYY-MM-01`), *(opt)* `inbound_month`, `min_trip_days`, `max_trip_days`, `currency` |
| `cheapest_weekend` | Cheapest Fri→Sun (or Fri→Mon) return weekend on a route over the next few months. | `origin`, `destination`, *(opt)* `months_ahead` (default 3), `weekend_length` (`2`\|`3`, default 2) |
| `get_active_dates` | Dates a route is currently bookable (ISO dates, no prices). | `origin`, `destination` |
| `get_schedules` | Published timetable (days/times a route runs, no prices) for a month. | `origin`, `destination`, `year`, `month` |
| `list_airports` | List Ryanair airports, optionally filtered by country. | *(opt)* `country` (ISO-3166 alpha-2) |
| `validate_route` | Whether Ryanair flies a direct route between two airports. | `origin`, `destination` |
| `explore_destinations` | Airports reachable from an origin (each flagged `seasonal` and carrying region/country metadata), optionally annotated with cheapest fares, filtered, and grouped. | `origin`, *(opt)* `with_fares`, `date_from`, `date_to`, `currency`, `country`, `region`, `city`, `group_by` (`country`\|`region`) |
| `active_airports` | Every airport Ryanair currently flies, with full location metadata, in one call. | *(none)* |
| `airport_info` | Metadata for a single airport (city, region, country, timezone, coordinates). | `code` (IATA) |
| `airport_destinations` | Destinations reachable from an origin, each carrying `operator`, `seasonal`, `recent`, and `tags` metadata. | `origin` |
| `nearby_airports` | Airports near the server's IP-derived location. | *(opt)* `market` (IETF locale) |
| `default_airport` | Closest airport to the server's IP-derived location. | *(none)* |

Airport inputs are IATA codes (e.g. `DUB`, `STN`). Dates are ISO `YYYY-MM-DD`.
Currencies are ISO 4217 (e.g. `EUR`).

> `nearby_airports` and `default_airport` geolocate by the caller's IP. Since
> this server makes the request, they resolve to the server's location, not the
> end user's — useful when the server runs near the user, less so otherwise.

## Agent skill

A [`SKILL.md`](skills/ryanair-flights/SKILL.md) ships under
`skills/ryanair-flights/`, following the [Agent Skills](https://github.com/anthropics/skills)
open standard (compatible with Claude, Hermes, Cursor, and Codex). It tells an
agent when to reach for this server and how to drive its tools. Point your
agent's skills directory at that folder, or copy it in.

## How it works

- **Session priming.** Cold calls to the services API sometimes return `403`, so
  the client warms a cookie jar against `www.ryanair.com` once before the first
  request.
- **Network caching.** The airport/route bundle backing `list_airports`,
  `validate_route`, and `explore_destinations` is cached for 6 hours.
- **Retries.** Requests retry up to 3 times with capped exponential backoff,
  only on `429`, `5xx`, and network errors. Other failures surface immediately
  as an `APIError` carrying the endpoint, status, and a body snippet.
- **User-Agent rotation.** Each request picks a realistic desktop browser UA;
  Ryanair blocks obvious non-browser clients.

## Development

```sh
go test ./...        # unit tests (parsing, quirks, retry classification, validation)
golangci-lint run    # lint (strict config in .golangci.yml)
```

Tests run against recorded fixtures in `internal/ryanair/testdata/` — no network
access required.

A build-tagged live smoke test hits the real Ryanair endpoints to catch
wire-format or endpoint drift. It is excluded from build/PR CI (so pull requests
never depend on Ryanair's availability) and instead runs on a daily schedule via
[`.github/workflows/live.yml`](.github/workflows/live.yml). Run it locally too:

```sh
go test -tags live ./internal/ryanair/ -v
```

## Project layout

```
cmd/ryanair-mcp     entry point, flag parsing, transport selection
internal/server     builds the MCP server, runs stdio / streamable HTTP
internal/tools      MCP tool definitions; clean domain types only
internal/ryanair    typed client; all Ryanair wire-format quirks live here
```

## Limitations

- Read-only. No booking or seat-availability endpoints.

## Roadmap

- **Session-based (authenticated) endpoints.** Today we cover only Ryanair's
  public, unauthenticated read surface. Booking flows, seat availability/seat
  maps, and live pricing sessions (bags, fare classes) sit behind Ryanair's
  session/login flow and return `409` without one. Bringing these in means
  reverse-engineering and maintaining that auth flow — a larger, more fragile
  effort tracked here as future scope, not yet started.

## Contributing

Contributions are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md) for the dev
workflow and conventions, and [SECURITY.md](SECURITY.md) to report a
vulnerability. Notable changes are tracked in [CHANGELOG.md](CHANGELOG.md).

## License

[MIT](LICENSE) © Adam Benhassen
