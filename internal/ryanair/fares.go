package ryanair

import (
	"cmp"
	"context"
	"fmt"
	"net/url"
	"slices"
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
	if p.MaxPrice < 0 {
		return nil, fmt.Errorf("max price must be >= 0, got %d", p.MaxPrice)
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
	if err := resp.checkComplete(); err != nil {
		return nil, err
	}
	flights := make([]Flight, 0, len(resp.Fares))
	for _, f := range resp.Fares {
		flight, err := legToFlight(f.Outbound)
		if err != nil {
			return nil, err
		}
		flights = append(flights, flight)
	}
	return flights, nil
}

// AnywhereUnder returns the cheapest one-way fare per reachable destination from
// an origin under params.MaxPrice, sorted ascending by price. Destination and
// Country are ignored: the probe is network-wide.
func (c *Client) AnywhereUnder(ctx context.Context, params OneWayParams) ([]Flight, error) {
	if params.MaxPrice <= 0 {
		return nil, fmt.Errorf("max price must be > 0, got %d", params.MaxPrice)
	}
	params.Destination = ""
	params.Country = ""
	flights, err := c.OneWayFares(ctx, params)
	if err != nil {
		return nil, err
	}
	cheapest := make(map[string]Flight, len(flights))
	for _, f := range flights {
		if cur, ok := cheapest[f.Destination]; !ok || f.Price < cur.Price {
			cheapest[f.Destination] = f
		}
	}
	out := make([]Flight, 0, len(cheapest))
	for _, f := range cheapest {
		out = append(out, f)
	}
	slices.SortFunc(out, func(a, b Flight) int {
		return cmp.Compare(a.Price, b.Price)
	})
	return out, nil
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
	if err := resp.checkComplete(); err != nil {
		return nil, err
	}
	trips := make([]ReturnFlight, 0, len(resp.Fares))
	for _, f := range resp.Fares {
		outbound, err := legToFlight(f.Outbound)
		if err != nil {
			return nil, err
		}
		inbound, err := legToFlight(f.Inbound)
		if err != nil {
			return nil, err
		}
		trip := ReturnFlight{
			Outbound:     outbound,
			Inbound:      inbound,
			TotalPrice:   f.Summary.Price.Value,
			Currency:     f.Summary.Price.CurrencyCode,
			TripDuration: f.Summary.TripDurationDays,
			NewRoute:     f.Summary.NewRoute,
		}
		if f.Summary.PreviousPrice != nil {
			v := f.Summary.PreviousPrice.Value
			trip.PreviousPrice = &v
		}
		trips = append(trips, trip)
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

// checkComplete guards against silently returning a partial fare list. We never
// send a limit, so these endpoints return the full result set in one response
// and nextPage stays null. A non-null nextPage means Ryanair began capping
// responses; the endpoint exposes no working cursor (offset and page params are
// ignored — verified against the live API as of 2026-06) to fetch the rest, so
// we surface a loud error instead of truncating.
func (r wireFaresResponse) checkComplete() error {
	if r.NextPage != nil {
		return fmt.Errorf("ryanair: fares response truncated (nextPage=%d, returned %d of %d); endpoint started paginating with no fetchable cursor", *r.NextPage, len(r.Fares), r.Size)
	}
	return nil
}

func legToFlight(leg wireLeg) (Flight, error) {
	dep, err := parseTime(leg.DepartureDate)
	if err != nil {
		return Flight{}, fmt.Errorf("ryanair: departure %s: %w", leg.FlightNumber, err)
	}
	arr, err := parseTime(leg.ArrivalDate)
	if err != nil {
		return Flight{}, fmt.Errorf("ryanair: arrival %s: %w", leg.FlightNumber, err)
	}
	f := Flight{
		Origin:        leg.DepartureAirport.IataCode,
		Destination:   leg.ArrivalAirport.IataCode,
		OriginName:    leg.DepartureAirport.Name,
		DestName:      leg.ArrivalAirport.Name,
		DepartureTime: dep,
		ArrivalTime:   arr,
		FlightNumber:  leg.FlightNumber,
		Price:         leg.Price.Value,
		Currency:      leg.Price.CurrencyCode,
	}
	if leg.PreviousPrice != nil {
		v := leg.PreviousPrice.Value
		f.PreviousPrice = &v
	}
	if leg.PriceUpdated > 0 {
		// Best-effort advisory metadata: the epoch-millis value is trusted as-is
		// (only guarded against the absent/zero case), not range-checked.
		t := time.UnixMilli(leg.PriceUpdated)
		f.PriceUpdated = &t
	}
	return f, nil
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

// parseTime parses a Ryanair local datetime.
func parseTime(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02T15:04:05", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("ryanair: parse time %q: %w", s, err)
	}
	return t, nil
}

// parseOptTime parses an optional datetime, returning nil when absent or
// unparseable. Used for calendar fares where times are genuinely optional.
func parseOptTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := parseTime(s)
	if err != nil {
		return nil
	}
	return &t
}
