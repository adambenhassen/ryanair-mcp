package ryanair

import (
	"context"
	"fmt"
)

// ScheduleParams selects the published timetable for a route in a given month.
type ScheduleParams struct {
	Origin      string // required, IATA
	Destination string // required, IATA
	Year        int    // required, four digits
	Month       int    // required, 1-12
}

// Schedules returns the published timetable for a route in a given month
// (no prices). params.Year is four digits; params.Month is 1-12.
func (c *Client) Schedules(ctx context.Context, params ScheduleParams) ([]TimetableFlight, error) {
	o, d, err := normRoute(params.Origin, params.Destination)
	if err != nil {
		return nil, err
	}
	if params.Month < 1 || params.Month > 12 {
		return nil, fmt.Errorf("invalid month %d", params.Month)
	}
	if params.Year < 2000 || params.Year > 2100 {
		return nil, fmt.Errorf("invalid year %d", params.Year)
	}
	endpoint := fmt.Sprintf("timtbl/3/schedules/%s/%s/years/%d/months/%d", o, d, params.Year, params.Month)
	var resp wireScheduleResponse
	if err := getJSON(ctx, c, servicesHost, endpoint, nil, &resp); err != nil {
		return nil, err
	}

	var flights []TimetableFlight
	for _, day := range resp.Days {
		for _, f := range day.Flights {
			flights = append(flights, TimetableFlight{
				Day:           day.Day,
				FlightNumber:  f.CarrierCode + f.Number,
				DepartureTime: f.DepartureTime,
				ArrivalTime:   f.ArrivalTime,
			})
		}
	}
	return flights, nil
}
