package ryanair

import (
	"cmp"
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
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

// CalendarParams selects the per-day cheapest one-way fares for a route across a
// single month.
type CalendarParams struct {
	Origin      string // required, IATA
	Destination string // required, IATA
	Month       string // required, first day of the month (ISO date)
	Currency    string // optional, ISO 4217
}

// ReturnCalendarParams selects the per-day cheapest return fares for a route
// across the outbound and inbound month calendars.
type ReturnCalendarParams struct {
	Origin        string // required, IATA
	Destination   string // required, IATA
	OutboundMonth string // required, first day of the month (ISO date)
	InboundMonth  string // optional, defaults to OutboundMonth
	MinTripDays   int    // optional, 0 = omit
	MaxTripDays   int    // optional, 0 = omit
	Currency      string // optional, ISO 4217
}

// WeekendParams selects the cheapest weekend return trip for a route.
type WeekendParams struct {
	Origin        string // required, IATA
	Destination   string // required, IATA
	MonthsAhead   int    // required, >= 1
	WeekendLength int    // required, 2 (Fri-Sun) or 3 (Fri-Mon)
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
		q.Set("priceValueTo", strconv.Itoa(p.MaxPrice))
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
	if err := getJSON(ctx, c, servicesHost, endpoint, q, &resp); err != nil {
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
		q.Set("durationFrom", strconv.Itoa(params.MinTripDays))
	}
	if params.MaxTripDays > 0 {
		q.Set("durationTo", strconv.Itoa(params.MaxTripDays))
	}

	var resp wireFaresResponse
	endpoint := "farfnd/v4/roundTripFares"
	if err := getJSON(ctx, c, servicesHost, endpoint, q, &resp); err != nil {
		return nil, err
	}
	// NOTE: no checkComplete() here. Unlike oneWayFares, roundTripFares returns
	// nextPage=1 on every response — even a complete one (verified live: 98 of
	// 98 with nextPage=1) — so it carries no usable truncation signal to guard on.
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
// month. params.Month must be the first day of the month (ISO date).
func (c *Client) CheapestPerDay(ctx context.Context, params CalendarParams) ([]DailyFare, error) {
	o, d, err := normRoute(params.Origin, params.Destination)
	if err != nil {
		return nil, err
	}
	if _, err := time.Parse(dateLayout, params.Month); err != nil {
		return nil, fmt.Errorf("invalid month %q: %w", params.Month, err)
	}
	q := url.Values{}
	q.Set("outboundMonthOfDate", params.Month)
	if params.Currency != "" {
		q.Set("currency", params.Currency)
	}
	endpoint := fmt.Sprintf("farfnd/v4/oneWayFares/%s/%s/cheapestPerDay", o, d)
	var resp wireCalendarResponse
	if err := getJSON(ctx, c, servicesHost, endpoint, q, &resp); err != nil {
		return nil, err
	}
	return dailyFares(resp.Outbound.Fares), nil
}

// checkComplete guards oneWayFares against silently returning a partial fare
// list. We never send a limit, so oneWayFares returns the full result set in one
// response and nextPage stays null. A non-null nextPage means Ryanair began
// capping responses; the endpoint exposes no working cursor (offset and page
// params are ignored — verified against the live API as of 2026-06) to fetch the
// rest, so we surface a loud error instead of truncating.
//
// This applies to oneWayFares only. roundTripFares returns nextPage=1 on every
// response, complete or not, so it has no usable truncation signal to guard on.
func (r wireFaresResponse) checkComplete() error {
	if r.NextPage != nil {
		return fmt.Errorf("ryanair: fares response truncated (nextPage=%d, returned %d of %d); endpoint started paginating with no fetchable cursor", *r.NextPage, len(r.Fares), r.Size)
	}
	return nil
}

// RouteActiveDates returns the dates a route is currently bookable (ISO
// YYYY-MM-DD strings, no price info). It is a very cheap lookup.
func (c *Client) RouteActiveDates(ctx context.Context, origin, dest string) ([]string, error) {
	o, d, err := normRoute(origin, dest)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("farfnd/v4/oneWayFares/%s/%s/availabilities", o, d)
	var dates []string
	if err := getJSON(ctx, c, servicesHost, endpoint, nil, &dates); err != nil {
		return nil, err
	}
	return dates, nil
}

// CheapestReturnPerDay returns the per-day cheapest fares for a return trip
// across the outbound and inbound months (outbound and inbound calendars side
// by side). params.InboundMonth defaults to params.OutboundMonth when empty.
// params.MinTripDays and params.MaxTripDays constrain trip length in days (0 =
// omit). Months must be the first day of the month (ISO date).
func (c *Client) CheapestReturnPerDay(ctx context.Context, params ReturnCalendarParams) (ReturnDailyFares, error) {
	o, d, err := normRoute(params.Origin, params.Destination)
	if err != nil {
		return ReturnDailyFares{}, err
	}
	if _, err := time.Parse(dateLayout, params.OutboundMonth); err != nil {
		return ReturnDailyFares{}, fmt.Errorf("invalid outbound month %q: %w", params.OutboundMonth, err)
	}
	inboundMonth := params.InboundMonth
	if inboundMonth == "" {
		inboundMonth = params.OutboundMonth
	} else if _, err := time.Parse(dateLayout, inboundMonth); err != nil {
		return ReturnDailyFares{}, fmt.Errorf("invalid inbound month %q: %w", inboundMonth, err)
	}
	q := url.Values{}
	q.Set("outboundMonthOfDate", params.OutboundMonth)
	q.Set("inboundMonthOfDate", inboundMonth)
	if params.MinTripDays > 0 {
		q.Set("durationFrom", strconv.Itoa(params.MinTripDays))
	}
	if params.MaxTripDays > 0 {
		q.Set("durationTo", strconv.Itoa(params.MaxTripDays))
	}
	if params.Currency != "" {
		q.Set("currency", params.Currency)
	}
	endpoint := fmt.Sprintf("farfnd/v4/roundTripFares/%s/%s/cheapestPerDay", o, d)
	var resp wireCalendarResponse
	if err := getJSON(ctx, c, servicesHost, endpoint, q, &resp); err != nil {
		return ReturnDailyFares{}, err
	}
	return ReturnDailyFares{
		Outbound: dailyFares(resp.Outbound.Fares),
		Inbound:  dailyFares(resp.Inbound.Fares),
	}, nil
}

// CheapestWeekend finds the cheapest Friday->Sunday (WeekendLength 2) or
// Friday->Monday (WeekendLength 3) return trip over the next params.MonthsAhead
// months. It returns nil when no priced weekend exists in the window.
func (c *Client) CheapestWeekend(ctx context.Context, params WeekendParams) (*WeekendTrip, error) {
	if params.WeekendLength != 2 && params.WeekendLength != 3 {
		return nil, fmt.Errorf("weekend length must be 2 (Fri-Sun) or 3 (Fri-Mon), got %d", params.WeekendLength)
	}
	if params.MonthsAhead < 1 {
		return nil, fmt.Errorf("months ahead must be >= 1, got %d", params.MonthsAhead)
	}
	now := time.Now()
	end := now.AddDate(0, params.MonthsAhead, 0)
	var outbounds, inbounds []DailyFare
	for m := firstOfMonth(now); !m.After(end); m = m.AddDate(0, 1, 0) {
		month := m.Format(dateLayout)
		fares, err := c.CheapestReturnPerDay(ctx, ReturnCalendarParams{
			Origin:        params.Origin,
			Destination:   params.Destination,
			OutboundMonth: month,
			InboundMonth:  month,
			MinTripDays:   params.WeekendLength,
			MaxTripDays:   params.WeekendLength,
		})
		if err != nil {
			return nil, err
		}
		outbounds = append(outbounds, fares.Outbound...)
		inbounds = append(inbounds, fares.Inbound...)
	}
	return cheapestWeekendTrip(outbounds, inbounds, params.WeekendLength)
}

// cheapestWeekendTrip pairs each priced Friday outbound with the inbound
// weekendLength days later and returns the cheapest combined pair, or nil.
func cheapestWeekendTrip(outbounds, inbounds []DailyFare, weekendLength int) (*WeekendTrip, error) {
	inboundByDay := make(map[string]DailyFare, len(inbounds))
	for _, in := range inbounds {
		if in.Price != nil {
			inboundByDay[in.Day] = in
		}
	}
	var best *WeekendTrip
	for _, out := range outbounds {
		if out.Price == nil {
			continue
		}
		day, err := time.Parse(dateLayout, out.Day)
		if err != nil {
			return nil, fmt.Errorf("ryanair: parse weekend day %q: %w", out.Day, err)
		}
		if day.Weekday() != time.Friday {
			continue
		}
		retDay := day.AddDate(0, 0, weekendLength).Format(dateLayout)
		in, ok := inboundByDay[retDay]
		if !ok {
			continue
		}
		total := *out.Price + *in.Price
		if best == nil || total < best.TotalPrice {
			// Both legs come from one cheapestPerDay response priced in a single
			// currency, so the outbound's currency represents the whole trip.
			best = &WeekendTrip{Outbound: out, Inbound: in, TotalPrice: total, Currency: out.Currency}
		}
	}
	return best, nil
}

// firstOfMonth returns midnight on the first day of t's month.
func firstOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
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
