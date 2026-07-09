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
	WithFares, WithRouteDetails                              bool
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
		Origin:           in.Origin,
		WithFares:        in.WithFares,
		WithRouteDetails: in.WithRouteDetails,
		DateFrom:         in.DateFrom,
		DateTo:           in.DateTo,
		Country:          in.Country,
		Region:           in.Region,
		City:             in.City,
		GroupBy:          in.GroupBy,
	})
	return ExploreResult(out), err
}

// RunActiveDates invokes the get_active_dates handler end-to-end.
func RunActiveDates(c *ryanair.Client, origin, dest string) ([]string, error) {
	h := getActiveDates(c)
	_, out, err := h(context.Background(), nil, activeDatesInput{Origin: origin, Destination: dest})
	return out.Dates, err
}

// OneWayArgs mirrors the search_one_way tool input for tests.
type OneWayArgs struct {
	Origin, DateFrom, DateTo, Destination, Country, Currency string
	MaxPrice                                                 int
}

// RunSearchOneWay invokes the search_one_way handler end-to-end.
func RunSearchOneWay(c *ryanair.Client, in OneWayArgs) ([]ryanair.Flight, error) {
	h := searchOneWay(c)
	_, out, err := h(context.Background(), nil, oneWayInput{
		Origin: in.Origin, DateFrom: in.DateFrom, DateTo: in.DateTo,
		Destination: in.Destination, Country: in.Country, MaxPrice: in.MaxPrice, Currency: in.Currency,
	})
	return out.Flights, err
}

// ReturnArgs mirrors the search_return tool input for tests.
type ReturnArgs struct {
	Origin, DateFrom, DateTo, Destination, Country, Currency string
	ReturnFrom, ReturnTo                                     string
	MinTripDays, MaxTripDays, MaxPrice                       int
}

// RunSearchReturn invokes the search_return handler end-to-end.
func RunSearchReturn(c *ryanair.Client, in ReturnArgs) ([]ryanair.ReturnFlight, error) {
	h := searchReturn(c)
	_, out, err := h(context.Background(), nil, returnInput{
		oneWayInput: oneWayInput{
			Origin: in.Origin, DateFrom: in.DateFrom, DateTo: in.DateTo,
			Destination: in.Destination, Country: in.Country, MaxPrice: in.MaxPrice, Currency: in.Currency,
		},
		ReturnFrom: in.ReturnFrom, ReturnTo: in.ReturnTo, MinTripDays: in.MinTripDays, MaxTripDays: in.MaxTripDays,
	})
	return out.Trips, err
}

// RunCheapestPerDay invokes the cheapest_per_day handler end-to-end.
func RunCheapestPerDay(c *ryanair.Client, origin, dest, month, currency string) ([]ryanair.DailyFare, error) {
	h := cheapestPerDay(c)
	_, out, err := h(context.Background(), nil, calendarInput{Origin: origin, Destination: dest, Month: month, Currency: currency})
	return out.Days, err
}

// RunGetSchedules invokes the get_schedules handler end-to-end.
func RunGetSchedules(c *ryanair.Client, origin, dest string, year, month int) ([]ryanair.TimetableFlight, error) {
	h := getSchedules(c)
	_, out, err := h(context.Background(), nil, scheduleInput{Origin: origin, Destination: dest, Year: year, Month: month})
	return out.Flights, err
}

// RunListAirports invokes the list_airports handler end-to-end.
func RunListAirports(c *ryanair.Client, country, code string) ([]ryanair.Airport, error) {
	h := listAirports(c)
	_, out, err := h(context.Background(), nil, airportsInput{Country: country, Code: code})
	return out.Airports, err
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
