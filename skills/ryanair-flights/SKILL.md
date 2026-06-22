---
name: ryanair-flights
description: Use when a user wants to find or compare Ryanair flights across Europe — cheapest one-way or return fares, "anywhere under €X" discovery, price calendars, cheapest-weekend trips, published timetables, or the airport/route network. Backed by the ryanair-mcp server (12 read-only tools over Ryanair's unofficial public API). Not for booking, seat selection, or live pricing/bags.
license: MIT
---

# Ryanair flights (ryanair-mcp)

An MCP server exposing Ryanair's anonymous, read-only flight APIs as 12 tools.
Use it to answer questions like "cheapest weekend from Dublin in August?",
"where can I fly from Stansted under £30 next month?", or "does Ryanair fly
DUB→BCN?".

## When to use

- Searching/comparing **Ryanair** fares, calendars, or routes in Europe.
- Discovering cheap destinations from an origin within a budget or date window.
- Checking whether a route exists, when it operates, or which airports are nearby.

## When NOT to use

- **Booking, seat maps, baggage, or live per-passenger pricing** — not supported
  (read-only; those sit behind Ryanair's authenticated session).
- **Other airlines** — this is Ryanair-only.
- Authoritative/real-time guarantees — the upstream API is undocumented and may
  change or rate-limit; treat results as best-effort.

## Input conventions (apply to every tool)

- **Airports** are IATA codes, uppercase: `DUB`, `STN`, `BCN`.
- **Dates** are ISO `YYYY-MM-DD`. Month-anchored inputs want the first of the
  month: `YYYY-MM-01`.
- **Country** is ISO 3166-1 alpha-2: `es`, `gb`. Case-insensitive — the server
  normalizes it (the upstream API only accepts lowercase).
- **Currency** is ISO 4217: `EUR`, `GBP`. If omitted, prices come back in the
  departure airport's local currency.
- Date filters are a **window**, not a single day — the server returns the
  cheapest fare per route per day inside it. For one exact day, set from = to.

## Tools

**Fares**
- `search_one_way` — cheapest one-way fares from an origin in a date window;
  omit destination/country to search anywhere. (`origin`, `date_from`, `date_to`)
- `search_return` — cheapest returns across outbound + inbound windows, optional
  trip-length bounds. (`origin`, `date_from`, `date_to`, `return_from`, `return_to`)
- `find_anywhere_under` — cheapest reachable destination under a price cap, one
  flight per destination, sorted by price. (`origin`, `date_from`, `date_to`, `max_price`)
- `cheapest_per_day` — one-way price calendar for a route over a month.
  (`origin`, `destination`, `month`)
- `cheapest_return_per_day` — return price calendar, outbound + inbound side by
  side. (`origin`, `destination`, `outbound_month`)
- `cheapest_weekend` — cheapest Fri→Sun (or Fri→Mon) return over the next few
  months. (`origin`, `destination`, opt `months_ahead`, `weekend_length`)

**Schedules & availability**
- `get_active_dates` — dates a route is currently bookable (no prices).
- `get_schedules` — published timetable for a month (days/times, no prices).
  (`origin`, `destination`, `year`, `month`)

**Network & airports**
- `list_airports` — all Ryanair airports, optional country filter.
- `validate_route` — does Ryanair fly origin→destination directly?
- `explore_destinations` — airports reachable from an origin, flagged seasonal,
  with region/country metadata; optional fares (`with_fares`), per-route
  operator/recent/tags details (`with_route_details`), filters, and `group_by`.
- `airport_info` — metadata for one airport. (`code`)

## Recipes

- **"Cheapest weekend away from X soon"** → `cheapest_weekend` (origin = X,
  destination = candidate, or loop a few candidates from `explore_destinations`).
- **"Where can I go from X under €40 in July?"** → `find_anywhere_under`
  (`max_price: 40`, `date_from`/`date_to` spanning July).
- **"When are flights cheapest from X to Y this month?"** → `cheapest_per_day`.
- **"Does Ryanair fly X→Y, and when does it run?"** → `validate_route`, then
  `get_schedules`.
- **"What's reachable from X in Spain?"** → `explore_destinations`
  (`origin: X`, `country: es`).
- **"Cheapest return to Y for a 3–5 night trip in August"** → `search_return`
  (`date_from`/`date_to` for outbound, `return_from`/`return_to` for inbound,
  trip-length bounds).
- **"Best dates for a return to Y this month"** → `cheapest_return_per_day` for
  an outbound + inbound price calendar side by side.
- **"Which days is X→Y actually bookable?"** → `get_active_dates` (no prices,
  just the dates the route is open).

## Pitfalls

- Empty results are valid answers (no fare/route in range), **not** errors.
- This is unofficial and rate-limited; retry/backoff is built in, but expect
  occasional gaps. No booking, seats, or bag pricing.

## Reading results

Fare results carry price-history fields when Ryanair reports them
(`previous_price`, `price_updated`, `new_route`) — useful for flagging price
drops or newly-added routes.
