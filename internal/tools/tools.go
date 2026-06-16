// Package tools registers Ryanair read APIs as MCP tools on a server. It deals
// only in clean domain types from the ryanair package and shapes results for
// the model; all wire-format concerns live in package ryanair.
package tools

import (
	"context"

	"github.com/adambenhassen/ryanair-mcp/internal/ryanair"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register adds every Ryanair tool to the server, backed by client.
func Register(server *mcp.Server, client *ryanair.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_one_way",
		Description: "Find the cheapest one-way Ryanair fares from an origin airport within a departure-date window. Omit destination/country to search anywhere.",
	}, searchOneWay(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_return",
		Description: "Find the cheapest Ryanair return fares across outbound and inbound date windows, with optional trip-duration limits.",
	}, searchReturn(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "cheapest_per_day",
		Description: "Get the cheapest one-way fare for each day of a month on a specific route (price calendar).",
	}, cheapestPerDay(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_schedules",
		Description: "Get the published timetable (days and times a route runs, no prices) for a month.",
	}, getSchedules(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_airports",
		Description: "List Ryanair airports, optionally filtered by ISO-3166 alpha-2 country code.",
	}, listAirports(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "validate_route",
		Description: "Check whether Ryanair flies a direct route between two airports.",
	}, validateRoute(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "explore_destinations",
		Description: "List airports reachable from an origin, optionally annotated with the cheapest fare in a date window.",
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

func searchOneWay(c *ryanair.Client) mcp.ToolHandlerFor[oneWayInput, flightsOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in oneWayInput) (*mcp.CallToolResult, flightsOutput, error) {
		flights, err := c.OneWayFares(ctx, ryanair.OneWayParams{
			Origin:      in.Origin,
			DateFrom:    in.DateFrom,
			DateTo:      in.DateTo,
			Destination: in.Destination,
			Country:     in.Country,
			MaxPrice:    in.MaxPrice,
			Currency:    in.Currency,
		})
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
			OneWayParams: ryanair.OneWayParams{
				Origin:      in.Origin,
				DateFrom:    in.DateFrom,
				DateTo:      in.DateTo,
				Destination: in.Destination,
				Country:     in.Country,
				MaxPrice:    in.MaxPrice,
				Currency:    in.Currency,
			},
			ReturnFrom:  in.ReturnFrom,
			ReturnTo:    in.ReturnTo,
			MinTripDays: in.MinTripDays,
			MaxTripDays: in.MaxTripDays,
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
		days, err := c.CheapestPerDay(ctx, in.Origin, in.Destination, in.Month, in.Currency)
		if err != nil {
			return nil, calendarOutput{}, err
		}
		return nil, calendarOutput{Days: days}, nil
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
		flights, err := c.Schedules(ctx, in.Origin, in.Destination, in.Year, in.Month)
		if err != nil {
			return nil, scheduleOutput{}, err
		}
		return nil, scheduleOutput{Flights: flights}, nil
	}
}

// --- list_airports ---

type airportsInput struct {
	Country string `json:"country,omitempty" jsonschema:"optional ISO2 country code filter, e.g. ie"`
}

type airportsOutput struct {
	Airports []ryanair.Airport `json:"airports"`
}

func listAirports(c *ryanair.Client) mcp.ToolHandlerFor[airportsInput, airportsOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in airportsInput) (*mcp.CallToolResult, airportsOutput, error) {
		airports, err := c.ListAirports(ctx, in.Country)
		if err != nil {
			return nil, airportsOutput{}, err
		}
		return nil, airportsOutput{Airports: airports}, nil
	}
}

// --- validate_route ---

type routeInput struct {
	Origin      string `json:"origin"      jsonschema:"departure airport IATA code"`
	Destination string `json:"destination" jsonschema:"arrival airport IATA code"`
}

type routeOutput struct {
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
	Exists      bool   `json:"exists"`
}

func validateRoute(c *ryanair.Client) mcp.ToolHandlerFor[routeInput, routeOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in routeInput) (*mcp.CallToolResult, routeOutput, error) {
		exists, err := c.ValidateRoute(ctx, in.Origin, in.Destination)
		if err != nil {
			return nil, routeOutput{}, err
		}
		return nil, routeOutput{Origin: in.Origin, Destination: in.Destination, Exists: exists}, nil
	}
}

// --- explore_destinations ---

type exploreInput struct {
	Origin    string `json:"origin"               jsonschema:"departure airport IATA code"`
	WithFares bool   `json:"with_fares,omitempty" jsonschema:"if true, annotate each destination with its cheapest fare in the date window"`
	DateFrom  string `json:"date_from,omitempty"  jsonschema:"earliest outbound date for fares, ISO YYYY-MM-DD (required when with_fares is true)"`
	DateTo    string `json:"date_to,omitempty"    jsonschema:"latest outbound date for fares, ISO YYYY-MM-DD (required when with_fares is true)"`
	Currency  string `json:"currency,omitempty"   jsonschema:"optional ISO 4217 currency"`
}

type exploreOutput struct {
	Destinations []ryanair.Destination `json:"destinations"`
}

func exploreDestinations(c *ryanair.Client) mcp.ToolHandlerFor[exploreInput, exploreOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in exploreInput) (*mcp.CallToolResult, exploreOutput, error) {
		dests, err := c.ExploreDestinations(ctx, in.Origin, in.WithFares, ryanair.OneWayParams{
			DateFrom: in.DateFrom,
			DateTo:   in.DateTo,
			Currency: in.Currency,
		})
		if err != nil {
			return nil, exploreOutput{}, err
		}
		return nil, exploreOutput{Destinations: dests}, nil
	}
}
