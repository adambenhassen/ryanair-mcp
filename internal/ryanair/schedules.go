package ryanair

import (
	"context"
	"fmt"
)

// Schedules returns the published timetable for a route in a given month
// (no prices). year is four digits; month is 1-12.
func (c *Client) Schedules(ctx context.Context, origin, dest string, year, month int) ([]TimetableFlight, error) {
	o, d := normIATA(origin), normIATA(dest)
	if !validIATA(o) || !validIATA(d) {
		return nil, fmt.Errorf("invalid route %q-%q", origin, dest)
	}
	if month < 1 || month > 12 {
		return nil, fmt.Errorf("invalid month %d", month)
	}
	if year < 2000 || year > 2100 {
		return nil, fmt.Errorf("invalid year %d", year)
	}
	endpoint := fmt.Sprintf("timtbl/3/schedules/%s/%s/years/%d/months/%d", o, d, year, month)
	var resp wireScheduleResponse
	if err := getJSON(ctx, c, endpoint, servicesHost+"/"+endpoint, nil, &resp); err != nil {
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
