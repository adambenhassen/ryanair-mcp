# ryanair-mcp

An [MCP](https://modelcontextprotocol.io) server that exposes Ryanair's
anonymous read APIs as tools an LLM can call: fare search, price calendars,
timetables, and the airport/route network. Written in Go, served over **stdio**
(default) or **streamable HTTP**.

> Unofficial. This project consumes Ryanair's public, unauthenticated endpoints
> and is not affiliated with or endorsed by Ryanair. The APIs are undocumented
> and may change or rate-limit without notice.

## Tools

| Tool | What it does | Key inputs |
| --- | --- | --- |
| `search_one_way` | Cheapest one-way fares from an origin in a date window. Omit destination/country to search anywhere. | `origin`, `date_from`, `date_to`, *(opt)* `destination`, `country`, `max_price`, `currency` |
| `search_return` | Cheapest return fares across outbound and inbound windows, with optional trip-length bounds. | `origin`, `date_from`, `date_to`, `return_from`, `return_to`, *(opt)* `min_trip_days`, `max_trip_days`, … |
| `find_anywhere_under` | Cheapest one-way fare to each reachable destination from an origin under a price cap, in a date window — returns a list of flights, one per destination, sorted by price. | `origin`, `date_from`, `date_to`, `max_price`, *(opt)* `currency` |
| `cheapest_per_day` | Cheapest one-way fare per day of a month on a route (price calendar). | `origin`, `destination`, `month` (`YYYY-MM-01`), *(opt)* `currency` |
| `get_schedules` | Published timetable (days/times a route runs, no prices) for a month. | `origin`, `destination`, `year`, `month` |
| `list_airports` | List Ryanair airports, optionally filtered by country. | *(opt)* `country` (ISO-3166 alpha-2) |
| `validate_route` | Whether Ryanair flies a direct route between two airports. | `origin`, `destination` |
| `explore_destinations` | Airports reachable from an origin (each flagged `seasonal` and carrying region/country metadata), optionally annotated with cheapest fares, filtered, and grouped. | `origin`, *(opt)* `with_fares`, `date_from`, `date_to`, `currency`, `country`, `region`, `city`, `group_by` (`country`\|`region`) |

Airport inputs are IATA codes (e.g. `DUB`, `STN`). Dates are ISO `YYYY-MM-DD`.
Currencies are ISO 4217 (e.g. `EUR`).

Fare results carry price-history fields when Ryanair reports them, so callers can
detect price drops and newly-added routes: one-way flights carry `previous_price`
and `price_updated`; return trips carry `previous_price` and `new_route`.

## Build

Requires Go 1.26+.

```sh
go build -o ryanair-mcp ./cmd/ryanair-mcp
```

## Run

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

### Use with an MCP client

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

## Behavior notes

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
wire-format or endpoint drift. It is excluded from normal builds and CI; run it
explicitly:

```sh
go test -tags live ./internal/ryanair/ -v
```

### Layout

```
cmd/ryanair-mcp     entry point, flag parsing, transport selection
internal/server     builds the MCP server, runs stdio / streamable HTTP
internal/tools      MCP tool definitions; clean domain types only
internal/ryanair    typed client; all Ryanair wire-format quirks live here
```

## Limitations

- Read-only. No booking or seat-availability endpoints.
- Fare-search responses are not paginated (`nextPage` is not followed).
