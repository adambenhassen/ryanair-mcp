package ryanair

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

// OneWayParams selects cheapest one-way fares from an origin in a date window.
type OneWayParams struct {
	Origin      string // required, IATA
	DateFrom    string // required, ISO date
	DateTo      string // required, ISO date
	Destination string // optional, IATA; omit for "anywhere"
	Country     string // optional, ISO2 (auto-lowercased)
	MaxPrice    int    // optional, 0 = no cap
	Currency    string // optional, ISO 4217
	TimeFrom    string // optional, HH:MM
	TimeTo      string // optional, HH:MM
}

// ReturnParams extends OneWayParams with the inbound window and trip duration.
type ReturnParams struct {
	OneWayParams

	ReturnFrom     string // required, ISO date
	ReturnTo       string // required, ISO date
	MinTripDays    int    // optional
	MaxTripDays    int    // optional, 0 = no cap
	ReturnTimeFrom string // optional, HH:MM
	ReturnTimeTo   string // optional, HH:MM
}

func (p OneWayParams) values() (url.Values, error) {
	origin := normIATA(p.Origin)
	if !validIATA(origin) {
		return nil, fmt.Errorf("invalid origin IATA %q", p.Origin)
	}
	if err := validateDateRange("outbound", p.DateFrom, p.DateTo); err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("departureAirportIataCode", origin)
	q.Set("outboundDepartureDateFrom", p.DateFrom)
	q.Set("outboundDepartureDateTo", p.DateTo)
	q.Set("outboundDepartureTimeFrom", timeOr(p.TimeFrom, defaultTimeFrom))
	q.Set("outboundDepartureTimeTo", timeOr(p.TimeTo, defaultTimeTo))
	if dest := normIATA(p.Destination); dest != "" {
		if !validIATA(dest) {
			return nil, fmt.Errorf("invalid destination IATA %q", p.Destination)
		}
		q.Set("arrivalAirportIataCode", dest)
	}
	if p.Country != "" {
		q.Set("arrivalCountryCode", normCountry(p.Country))
	}
	if p.MaxPrice > 0 {
		q.Set("priceValueTo", itoa(p.MaxPrice))
	}
	if p.Currency != "" {
		q.Set("currency", p.Currency)
	}
	return q, nil
}

// OneWayFares returns the cheapest one-way fares matching params.
func (c *Client) OneWayFares(ctx context.Context, params OneWayParams) ([]Flight, error) {
	q, err := params.values()
	if err != nil {
		return nil, err
	}
	var resp wireFaresResponse
	endpoint := "farfnd/v4/oneWayFares"
	if err := getJSON(ctx, c, endpoint, servicesHost+"/"+endpoint, q, &resp); err != nil {
		return nil, err
	}
	flights := make([]Flight, 0, len(resp.Fares))
	for _, f := range resp.Fares {
		flights = append(flights, legToFlight(f.Outbound))
	}
	return flights, nil
}

// RoundTripFares returns the cheapest return fares matching params.
func (c *Client) RoundTripFares(ctx context.Context, params ReturnParams) ([]ReturnFlight, error) {
	q, err := params.values()
	if err != nil {
		return nil, err
	}
	if err := validateDateRange("inbound", params.ReturnFrom, params.ReturnTo); err != nil {
		return nil, err
	}
	q.Set("inboundDepartureDateFrom", params.ReturnFrom)
	q.Set("inboundDepartureDateTo", params.ReturnTo)
	q.Set("inboundDepartureTimeFrom", timeOr(params.ReturnTimeFrom, defaultTimeFrom))
	q.Set("inboundDepartureTimeTo", timeOr(params.ReturnTimeTo, defaultTimeTo))
	if params.MinTripDays > 0 {
		q.Set("durationFrom", itoa(params.MinTripDays))
	}
	if params.MaxTripDays > 0 {
		q.Set("durationTo", itoa(params.MaxTripDays))
	}

	var resp wireFaresResponse
	endpoint := "farfnd/v4/roundTripFares"
	if err := getJSON(ctx, c, endpoint, servicesHost+"/"+endpoint, q, &resp); err != nil {
		return nil, err
	}
	trips := make([]ReturnFlight, 0, len(resp.Fares))
	for _, f := range resp.Fares {
		trips = append(trips, ReturnFlight{
			Outbound:     legToFlight(f.Outbound),
			Inbound:      legToFlight(f.Inbound),
			TotalPrice:   f.Summary.Price.Value,
			Currency:     f.Summary.Price.CurrencyCode,
			TripDuration: f.Summary.TripDurationDays,
		})
	}
	return trips, nil
}

// CheapestPerDay returns the per-day cheapest one-way fare for a route across a
// month. month must be the first day of the month (ISO date).
func (c *Client) CheapestPerDay(ctx context.Context, origin, dest, month, currency string) ([]DailyFare, error) {
	o, d := normIATA(origin), normIATA(dest)
	if !validIATA(o) || !validIATA(d) {
		return nil, fmt.Errorf("invalid route %q-%q", origin, dest)
	}
	if _, err := time.Parse(dateLayout, month); err != nil {
		return nil, fmt.Errorf("invalid month %q: %w", month, err)
	}
	q := url.Values{}
	q.Set("outboundMonthOfDate", month)
	if currency != "" {
		q.Set("currency", currency)
	}
	endpoint := fmt.Sprintf("farfnd/v4/oneWayFares/%s/%s/cheapestPerDay", o, d)
	var resp wireCalendarResponse
	if err := getJSON(ctx, c, endpoint, servicesHost+"/"+endpoint, q, &resp); err != nil {
		return nil, err
	}
	return dailyFares(resp.Outbound.Fares), nil
}

func legToFlight(leg wireLeg) Flight {
	return Flight{
		Origin:        leg.DepartureAirport.IataCode,
		Destination:   leg.ArrivalAirport.IataCode,
		OriginName:    leg.DepartureAirport.Name,
		DestName:      leg.ArrivalAirport.Name,
		DepartureTime: parseTime(leg.DepartureDate),
		ArrivalTime:   parseTime(leg.ArrivalDate),
		FlightNumber:  leg.FlightNumber,
		Price:         leg.Price.Value,
		Currency:      leg.Price.CurrencyCode,
	}
}

func dailyFares(raw []wireDailyFare) []DailyFare {
	out := make([]DailyFare, 0, len(raw))
	for _, f := range raw {
		df := DailyFare{
			Day:         f.Day,
			SoldOut:     f.SoldOut,
			Unavailable: f.Unavailable,
		}
		if f.Price != nil {
			v := f.Price.Value
			df.Price = &v
			df.Currency = f.Price.CurrencyCode
		}
		if dep := parseOptTime(f.DepartureDate); dep != nil {
			df.DepartureTime = dep
		}
		if arr := parseOptTime(f.ArrivalDate); arr != nil {
			df.ArrivalTime = arr
		}
		out = append(out, df)
	}
	return out
}

// parseTime parses a Ryanair local datetime, returning the zero time on failure.
func parseTime(s string) time.Time {
	t, err := time.Parse("2006-01-02T15:04:05", s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// parseOptTime parses an optional datetime, returning nil when absent/invalid.
func parseOptTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	if t := parseTime(s); !t.IsZero() {
		return &t
	}
	return nil
}
