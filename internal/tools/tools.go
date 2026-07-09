// Package tools registers Ryanair read APIs as MCP tools on a server. It deals
// only in clean domain types from the ryanair package and shapes results for
// the model; all wire-format concerns live in package ryanair.
package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/adambenhassen/ryanair-mcp/internal/ryanair"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register adds every Ryanair tool to the server, backed by client.
func Register(srv *mcp.Server, client *ryanair.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "search_one_way",
		Description: "Find the cheapest one-way Ryanair fares from an origin airport within a departure-date window. Omit destination/country to search anywhere.",
	}, searchOneWay(client))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "search_return",
		Description: "Find the cheapest Ryanair return fares across outbound and inbound date windows, with optional trip-duration limits.",
	}, searchReturn(client))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "cheapest_per_day",
		Description: "Get the cheapest one-way fare for each day of a month on a specific route (price calendar).",
	}, cheapestPerDay(client))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "cheapest_return_per_day",
		Description: "Get the cheapest return fare per day on a route, with outbound and inbound price calendars side by side and optional trip-duration limits.",
	}, cheapestReturnPerDay(client))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "cheapest_weekend",
		Description: "Find the cheapest Friday-to-Sunday (or Friday-to-Monday) return weekend on a route over the next few months. Returns the outbound/inbound pair and total price.",
	}, cheapestWeekend(client))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_active_dates",
		Description: "List the dates a route is currently bookable (ISO YYYY-MM-DD, no prices). Useful for checking which days a route operates.",
	}, getActiveDates(client))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_schedules",
		Description: "Get the published timetable (days and times a route runs, no prices) for a month.",
	}, getSchedules(client))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_airports",
		Description: "List Ryanair airports, optionally filtered by ISO-3166 alpha-2 country code, or look up a single airport's full metadata (city, region, timezone, coordinates) by IATA code.",
	}, listAirports(client))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "explore_destinations",
		Description: "List airports reachable from an origin, each flagged seasonal-only and carrying region/country metadata. Optionally annotate with cheapest fares in a date window (with_fares) or with per-route details — operating carrier, recently-added flag, and tags (with_route_details) — filter by country/region/city, and group by country or region.",
	}, exploreDestinations(client))
}

// --- search_one_way ---

type oneWayInput struct {
	Origin      string `json:"origin"                jsonschema:"departure airport IATA code, e.g. DUB"`
	DateFrom    string `json:"date_from"             jsonschema:"earliest outbound date, ISO YYYY-MM-DD"`
	DateTo      string `json:"date_to"               jsonschema:"latest outbound date, ISO YYYY-MM-DD"`
	Destination string `json:"destination,omitempty" jsonschema:"optional arrival airport IATA code"`
	Country     string `json:"country,omitempty"     jsonschema:"optional arrival country ISO2 code, e.g. es"`
	MaxPrice    int    `json:"max_price,omitempty"   jsonschema:"optional maximum price"`
	Currency    string `json:"currency,omitempty"    jsonschema:"optional ISO 4217 currency, e.g. EUR"`
}

type flightsOutput struct {
	Flights []ryanair.Flight `json:"flights"`
}

// toParams translates the tool input into the client fare params. It is the one
// place the oneWayInput->OneWayParams field mapping lives, shared by the
// one-way and return handlers.
func (in oneWayInput) toParams() ryanair.OneWayParams {
	return ryanair.OneWayParams{
		Origin:      in.Origin,
		DateFrom:    in.DateFrom,
		DateTo:      in.DateTo,
		Destination: in.Destination,
		Country:     in.Country,
		MaxPrice:    in.MaxPrice,
		Currency:    in.Currency,
	}
}

func searchOneWay(c *ryanair.Client) mcp.ToolHandlerFor[oneWayInput, flightsOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in oneWayInput) (*mcp.CallToolResult, flightsOutput, error) {
		flights, err := c.OneWayFares(ctx, in.toParams())
		if err != nil {
			return nil, flightsOutput{}, err
		}
		return nil, flightsOutput{Flights: flights}, nil
	}
}

// --- search_return ---

type returnInput struct {
	oneWayInput

	ReturnFrom  string `json:"return_from"             jsonschema:"earliest inbound date, ISO YYYY-MM-DD"`
	ReturnTo    string `json:"return_to"               jsonschema:"latest inbound date, ISO YYYY-MM-DD"`
	MinTripDays int    `json:"min_trip_days,omitempty" jsonschema:"optional minimum trip length in days"`
	MaxTripDays int    `json:"max_trip_days,omitempty" jsonschema:"optional maximum trip length in days"`
}

type returnsOutput struct {
	Trips []ryanair.ReturnFlight `json:"trips"`
}

func searchReturn(c *ryanair.Client) mcp.ToolHandlerFor[returnInput, returnsOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in returnInput) (*mcp.CallToolResult, returnsOutput, error) {
		trips, err := c.RoundTripFares(ctx, ryanair.ReturnParams{
			OneWayParams: in.toParams(),
			ReturnFrom:   in.ReturnFrom,
			ReturnTo:     in.ReturnTo,
			MinTripDays:  in.MinTripDays,
			MaxTripDays:  in.MaxTripDays,
		})
		if err != nil {
			return nil, returnsOutput{}, err
		}
		return nil, returnsOutput{Trips: trips}, nil
	}
}

// --- cheapest_per_day ---

type calendarInput struct {
	Origin      string `json:"origin"             jsonschema:"departure airport IATA code"`
	Destination string `json:"destination"        jsonschema:"arrival airport IATA code"`
	Month       string `json:"month"              jsonschema:"first day of the month, ISO YYYY-MM-01"`
	Currency    string `json:"currency,omitempty" jsonschema:"optional ISO 4217 currency"`
}

type calendarOutput struct {
	Days []ryanair.DailyFare `json:"days"`
}

func cheapestPerDay(c *ryanair.Client) mcp.ToolHandlerFor[calendarInput, calendarOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in calendarInput) (*mcp.CallToolResult, calendarOutput, error) {
		days, err := c.CheapestPerDay(ctx, ryanair.CalendarParams{
			Origin:      in.Origin,
			Destination: in.Destination,
			Month:       in.Month,
			Currency:    in.Currency,
		})
		if err != nil {
			return nil, calendarOutput{}, err
		}
		return nil, calendarOutput{Days: days}, nil
	}
}

// --- cheapest_return_per_day ---

type returnCalendarInput struct {
	Origin        string `json:"origin"                  jsonschema:"departure airport IATA code"`
	Destination   string `json:"destination"             jsonschema:"arrival airport IATA code"`
	OutboundMonth string `json:"outbound_month"          jsonschema:"first day of the outbound month, ISO YYYY-MM-01"`
	InboundMonth  string `json:"inbound_month,omitempty" jsonschema:"first day of the inbound month, ISO YYYY-MM-01 (defaults to outbound month)"`
	MinTripDays   int    `json:"min_trip_days,omitempty" jsonschema:"optional minimum trip length in days"`
	MaxTripDays   int    `json:"max_trip_days,omitempty" jsonschema:"optional maximum trip length in days"`
	Currency      string `json:"currency,omitempty"      jsonschema:"optional ISO 4217 currency"`
}

func cheapestReturnPerDay(c *ryanair.Client) mcp.ToolHandlerFor[returnCalendarInput, ryanair.ReturnDailyFares] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in returnCalendarInput) (*mcp.CallToolResult, ryanair.ReturnDailyFares, error) {
		res, err := c.CheapestReturnPerDay(ctx, ryanair.ReturnCalendarParams{
			Origin:        in.Origin,
			Destination:   in.Destination,
			OutboundMonth: in.OutboundMonth,
			InboundMonth:  in.InboundMonth,
			MinTripDays:   in.MinTripDays,
			MaxTripDays:   in.MaxTripDays,
			Currency:      in.Currency,
		})
		if err != nil {
			return nil, ryanair.ReturnDailyFares{}, err
		}
		return nil, res, nil
	}
}

// --- cheapest_weekend ---

type weekendInput struct {
	Origin        string `json:"origin"                   jsonschema:"departure airport IATA code"`
	Destination   string `json:"destination"              jsonschema:"arrival airport IATA code"`
	MonthsAhead   int    `json:"months_ahead,omitempty"   jsonschema:"how many months ahead to search (default 3)"`
	WeekendLength int    `json:"weekend_length,omitempty" jsonschema:"2 for Fri-Sun or 3 for Fri-Mon (default 2)"`
}

type weekendOutput struct {
	Found bool                 `json:"found"`
	Trip  *ryanair.WeekendTrip `json:"trip,omitempty"`
}

func cheapestWeekend(c *ryanair.Client) mcp.ToolHandlerFor[weekendInput, weekendOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in weekendInput) (*mcp.CallToolResult, weekendOutput, error) {
		monthsAhead := in.MonthsAhead
		if monthsAhead == 0 {
			monthsAhead = 3
		}
		weekendLength := in.WeekendLength
		if weekendLength == 0 {
			weekendLength = 2
		}
		trip, err := c.CheapestWeekend(ctx, ryanair.WeekendParams{
			Origin:        in.Origin,
			Destination:   in.Destination,
			MonthsAhead:   monthsAhead,
			WeekendLength: weekendLength,
		})
		if err != nil {
			return nil, weekendOutput{}, err
		}
		return nil, weekendOutput{Found: trip != nil, Trip: trip}, nil
	}
}

// --- get_active_dates ---

type activeDatesInput struct {
	Origin      string `json:"origin"      jsonschema:"departure airport IATA code"`
	Destination string `json:"destination" jsonschema:"arrival airport IATA code"`
}

type activeDatesOutput struct {
	Dates []string `json:"dates"`
}

func getActiveDates(c *ryanair.Client) mcp.ToolHandlerFor[activeDatesInput, activeDatesOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in activeDatesInput) (*mcp.CallToolResult, activeDatesOutput, error) {
		dates, err := c.RouteActiveDates(ctx, in.Origin, in.Destination)
		if err != nil {
			return nil, activeDatesOutput{}, err
		}
		return nil, activeDatesOutput{Dates: dates}, nil
	}
}

// --- get_schedules ---

type scheduleInput struct {
	Origin      string `json:"origin"      jsonschema:"departure airport IATA code"`
	Destination string `json:"destination" jsonschema:"arrival airport IATA code"`
	Year        int    `json:"year"        jsonschema:"four-digit year"`
	Month       int    `json:"month"       jsonschema:"month number 1-12"`
}

type scheduleOutput struct {
	Flights []ryanair.TimetableFlight `json:"flights"`
}

func getSchedules(c *ryanair.Client) mcp.ToolHandlerFor[scheduleInput, scheduleOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in scheduleInput) (*mcp.CallToolResult, scheduleOutput, error) {
		flights, err := c.Schedules(ctx, ryanair.ScheduleParams{
			Origin:      in.Origin,
			Destination: in.Destination,
			Year:        in.Year,
			Month:       in.Month,
		})
		if err != nil {
			return nil, scheduleOutput{}, err
		}
		return nil, scheduleOutput{Flights: flights}, nil
	}
}

// --- list_airports ---

type airportsInput struct {
	Country string `json:"country,omitempty" jsonschema:"optional ISO2 country code filter, e.g. ie"`
	Code    string `json:"code,omitempty"    jsonschema:"optional airport IATA code, e.g. DUB — returns just that airport's metadata (mutually exclusive with country)"`
}

type airportsOutput struct {
	Airports []ryanair.Airport `json:"airports"`
}

func listAirports(c *ryanair.Client) mcp.ToolHandlerFor[airportsInput, airportsOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in airportsInput) (*mcp.CallToolResult, airportsOutput, error) {
		if in.Code != "" {
			if in.Country != "" {
				return nil, airportsOutput{}, errors.New("code and country are mutually exclusive")
			}
			airport, err := c.AirportInfo(ctx, in.Code)
			if err != nil {
				return nil, airportsOutput{}, err
			}
			return nil, airportsOutput{Airports: []ryanair.Airport{airport}}, nil
		}
		airports, err := c.ListAirports(ctx, in.Country)
		if err != nil {
			return nil, airportsOutput{}, err
		}
		return nil, airportsOutput{Airports: airports}, nil
	}
}

// --- explore_destinations ---

type exploreInput struct {
	Origin           string `json:"origin"                       jsonschema:"departure airport IATA code"`
	WithFares        bool   `json:"with_fares,omitempty"         jsonschema:"if true, annotate each destination with its cheapest fare in the date window"`
	WithRouteDetails bool   `json:"with_route_details,omitempty" jsonschema:"if true, add per-route details via an extra lookup: operating carrier (operator, e.g. FR for Ryanair), whether the route was recently added (recent), and marketing tags like 'popular'. Omit unless the user asks about the airline, new routes, or route labels."`
	DateFrom         string `json:"date_from,omitempty"          jsonschema:"earliest outbound date for fares, ISO YYYY-MM-DD (required when with_fares is true)"`
	DateTo           string `json:"date_to,omitempty"            jsonschema:"latest outbound date for fares, ISO YYYY-MM-DD (required when with_fares is true)"`
	Currency         string `json:"currency,omitempty"           jsonschema:"optional ISO 4217 currency"`
	Country          string `json:"country,omitempty"            jsonschema:"optional arrival country ISO2 filter, e.g. es"`
	Region           string `json:"region,omitempty"             jsonschema:"optional region-code filter, e.g. CATALONIA"`
	City             string `json:"city,omitempty"               jsonschema:"optional city-code filter, e.g. LONDON"`
	GroupBy          string `json:"group_by,omitempty"           jsonschema:"optional grouping: 'country' or 'region'; omit for a flat list"`
}

type destinationGroup struct {
	Key          string                `json:"key"`
	Name         string                `json:"name"`
	Destinations []ryanair.Destination `json:"destinations"`
}

// exploreOutput carries either a flat destination list (when group_by is empty)
// or grouped buckets (when group_by is set). Exactly one field is populated.
type exploreOutput struct {
	Destinations []ryanair.Destination `json:"destinations,omitempty"`
	Groups       []destinationGroup    `json:"groups,omitempty"`
}

func exploreDestinations(c *ryanair.Client) mcp.ToolHandlerFor[exploreInput, exploreOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in exploreInput) (*mcp.CallToolResult, exploreOutput, error) {
		// Validate group_by up front so a bad value fails before any network call.
		if in.GroupBy != "" {
			if err := validateGroupBy(in.GroupBy); err != nil {
				return nil, exploreOutput{}, err
			}
		}
		dests, err := c.ExploreDestinations(ctx, ryanair.ExploreParams{
			Origin:           in.Origin,
			WithFares:        in.WithFares,
			WithRouteDetails: in.WithRouteDetails,
			Country:          in.Country,
			Region:           in.Region,
			City:             in.City,
			Fare: ryanair.FareWindow{
				DateFrom: in.DateFrom,
				DateTo:   in.DateTo,
				Currency: in.Currency,
			},
		})
		if err != nil {
			return nil, exploreOutput{}, err
		}
		if in.GroupBy == "" {
			return nil, exploreOutput{Destinations: dests}, nil
		}
		groups, err := groupDestinations(dests, in.GroupBy)
		if err != nil {
			return nil, exploreOutput{}, err
		}
		return nil, exploreOutput{Groups: groups}, nil
	}
}

// validateGroupBy is the single source of truth for which group_by values are
// supported. It rejects anything other than "country" or "region" (the empty
// "no grouping" case is handled by the caller before this point).
func validateGroupBy(by string) error {
	switch by {
	case "country", "region":
		return nil
	default:
		return fmt.Errorf("invalid group_by %q (want country or region)", by)
	}
}

// groupDestinations buckets destinations by country or region, preserving
// first-seen order of both groups and their members.
func groupDestinations(dests []ryanair.Destination, by string) ([]destinationGroup, error) {
	if err := validateGroupBy(by); err != nil {
		return nil, err
	}
	type keyName struct{ key, name string }
	var pick func(ryanair.Destination) keyName
	switch by {
	case "region":
		pick = func(d ryanair.Destination) keyName { return keyName{d.RegionCode, d.RegionName} }
	default: // "country" — validateGroupBy guarantees only country/region reach here
		pick = func(d ryanair.Destination) keyName { return keyName{d.CountryCode, d.CountryName} }
	}
	groups := make([]destinationGroup, 0)
	index := make(map[string]int)
	for _, d := range dests {
		kn := pick(d)
		i, ok := index[kn.key]
		if !ok {
			i = len(groups)
			index[kn.key] = i
			groups = append(groups, destinationGroup{Key: kn.key, Name: kn.name})
		}
		groups[i].Destinations = append(groups[i].Destinations, d)
	}
	return groups, nil
}
