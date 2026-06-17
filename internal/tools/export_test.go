package tools

import (
	"context"

	"github.com/adambenhassen/ryanair-mcp/internal/ryanair"
)

// Exported for white-box testing of the pure grouping helper and the handlers.

type DestinationGroup = destinationGroup

var GroupDestinations = groupDestinations

// ExploreArgs mirrors the explore_destinations tool input for tests.
type ExploreArgs struct {
	Origin, DateFrom, DateTo, Country, Region, City, GroupBy string
	WithFares                                                bool
}

// ExploreResult mirrors exploreOutput for tests.
type ExploreResult struct {
	Destinations []ryanair.Destination
	Groups       []DestinationGroup
}

// RunExplore invokes the explore_destinations handler end-to-end.
func RunExplore(c *ryanair.Client, in ExploreArgs) (ExploreResult, error) {
	h := exploreDestinations(c)
	_, out, err := h(context.Background(), nil, exploreInput{
		Origin:    in.Origin,
		WithFares: in.WithFares,
		DateFrom:  in.DateFrom,
		DateTo:    in.DateTo,
		Country:   in.Country,
		Region:    in.Region,
		City:      in.City,
		GroupBy:   in.GroupBy,
	})
	return ExploreResult(out), err
}

// RunActiveDates invokes the get_active_dates handler end-to-end.
func RunActiveDates(c *ryanair.Client, origin, dest string) ([]string, error) {
	h := getActiveDates(c)
	_, out, err := h(context.Background(), nil, activeDatesInput{Origin: origin, Destination: dest})
	return out.Dates, err
}

// RunCheapestReturnPerDay invokes the cheapest_return_per_day handler end-to-end.
func RunCheapestReturnPerDay(c *ryanair.Client, origin, dest, outMonth, inMonth string, minDur, maxDur int, currency string) ([]ryanair.DailyFare, []ryanair.DailyFare, error) {
	h := cheapestReturnPerDay(c)
	_, out, err := h(context.Background(), nil, returnCalendarInput{
		Origin: origin, Destination: dest, OutboundMonth: outMonth, InboundMonth: inMonth,
		MinTripDays: minDur, MaxTripDays: maxDur, Currency: currency,
	})
	return out.Outbound, out.Inbound, err
}

// RunCheapestWeekend invokes the cheapest_weekend handler end-to-end.
func RunCheapestWeekend(c *ryanair.Client, origin, dest string, monthsAhead, weekendLength int) (*ryanair.WeekendTrip, error) {
	h := cheapestWeekend(c)
	_, out, err := h(context.Background(), nil, weekendInput{
		Origin: origin, Destination: dest, MonthsAhead: monthsAhead, WeekendLength: weekendLength,
	})
	return out.Trip, err
}

// RunAnywhereUnder invokes the find_anywhere_under handler end-to-end.
func RunAnywhereUnder(c *ryanair.Client, origin, from, to string, maxPrice int) ([]ryanair.Flight, error) {
	h := findAnywhereUnder(c)
	_, out, err := h(context.Background(), nil, anywhereInput{
		Origin: origin, DateFrom: from, DateTo: to, MaxPrice: maxPrice,
	})
	return out.Flights, err
}

// RunActiveAirports invokes the active_airports handler end-to-end.
func RunActiveAirports(c *ryanair.Client) ([]ryanair.Airport, error) {
	h := activeAirports(c)
	_, out, err := h(context.Background(), nil, emptyInput{})
	return out.Airports, err
}

// RunAirportInfo invokes the airport_info handler end-to-end.
func RunAirportInfo(c *ryanair.Client, code string) (ryanair.Airport, error) {
	h := airportInfo(c)
	_, out, err := h(context.Background(), nil, airportCodeInput{Code: code})
	return out, err
}

// RunAirportDestinations invokes the airport_destinations handler end-to-end.
func RunAirportDestinations(c *ryanair.Client, origin string) ([]ryanair.Destination, error) {
	h := airportDestinations(c)
	_, out, err := h(context.Background(), nil, originInput{Origin: origin})
	return out.Destinations, err
}

// RunNearbyAirports invokes the nearby_airports handler end-to-end.
func RunNearbyAirports(c *ryanair.Client, market string) ([]ryanair.Airport, error) {
	h := nearbyAirports(c)
	_, out, err := h(context.Background(), nil, nearbyInput{Market: market})
	return out.Airports, err
}

// RunDefaultAirport invokes the default_airport handler end-to-end.
func RunDefaultAirport(c *ryanair.Client) (ryanair.Airport, error) {
	h := defaultAirport(c)
	_, out, err := h(context.Background(), nil, emptyInput{})
	return out, err
}
