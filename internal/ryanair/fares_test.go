package ryanair_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/adambenhassen/ryanair-mcp/internal/ryanair"
)

func TestOneWayFares(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/farfnd/v4/oneWayFares": "one_way_fares.json",
	}))

	flights, err := client.OneWayFares(context.Background(), ryanair.OneWayParams{
		Origin:   "dub",
		DateFrom: "2026-07-01",
		DateTo:   "2026-07-31",
		Country:  "ES", // must be sent lowercase
		Currency: "EUR",
	})
	if err != nil {
		t.Fatalf("OneWayFares: %v", err)
	}
	if len(flights) == 0 {
		t.Fatal("expected at least one flight")
	}
	if flights[0].Origin != "DUB" {
		t.Errorf("origin = %q, want DUB", flights[0].Origin)
	}
	if flights[0].Price <= 0 {
		t.Errorf("price = %v, want > 0", flights[0].Price)
	}

	q := fs.lastQuery.Load()
	if got := q.Get("arrivalCountryCode"); got != "es" {
		t.Errorf("arrivalCountryCode = %q, want lowercase es", got)
	}
	if got := q.Get("currency"); got != "EUR" {
		t.Errorf("currency = %q, want EUR", got)
	}
	if got := q.Get("outboundDepartureTimeFrom"); got != "00:00" {
		t.Errorf("default time-from = %q, want 00:00", got)
	}
	if got := q.Get("outboundDepartureTimeTo"); got != "23:59" {
		t.Errorf("default time-to = %q, want 23:59", got)
	}
	if fs.primeHits.Load() != 1 {
		t.Errorf("prime hits = %d, want 1", fs.primeHits.Load())
	}
}

func TestRoundTripFares(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/farfnd/v4/roundTripFares": "round_trip_fares.json",
	}))

	trips, err := client.RoundTripFares(context.Background(), ryanair.ReturnParams{
		OneWayParams: ryanair.OneWayParams{Origin: "DUB", DateFrom: "2026-07-01", DateTo: "2026-07-15"},
		ReturnFrom:   "2026-07-08",
		ReturnTo:     "2026-07-22",
	})
	if err != nil {
		t.Fatalf("RoundTripFares: %v", err)
	}
	if len(trips) == 0 {
		t.Fatal("expected at least one trip")
	}
	if trips[0].TotalPrice <= 0 {
		t.Errorf("total price = %v, want > 0", trips[0].TotalPrice)
	}
	if trips[0].Inbound.FlightNumber == "" {
		t.Error("expected inbound flight number")
	}
	q := fs.lastQuery.Load()
	if q.Get("inboundDepartureDateFrom") != "2026-07-08" {
		t.Errorf("inbound from = %q", q.Get("inboundDepartureDateFrom"))
	}
}

func TestCheapestPerDay(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/farfnd/v4/oneWayFares/DUB/STN/cheapestPerDay": "cheapest_per_day.json",
	}))

	days, err := client.CheapestPerDay(context.Background(), ryanair.CalendarParams{Origin: "DUB", Destination: "STN", Month: "2026-07-01"})
	if err != nil {
		t.Fatalf("CheapestPerDay: %v", err)
	}
	if len(days) == 0 {
		t.Fatal("expected daily fares")
	}
	for _, d := range days {
		if d.Day == "" {
			t.Error("daily fare missing day")
		}
	}
	// A sold-out day has no price and no times; the flags must propagate.
	var soldOut *ryanair.DailyFare
	for i := range days {
		if days[i].Day == "2026-07-05" {
			soldOut = &days[i]
		}
	}
	if soldOut == nil {
		t.Fatal("expected the sold-out day 2026-07-05")
	}
	if !soldOut.SoldOut {
		t.Error("2026-07-05 should be SoldOut")
	}
	if soldOut.Price != nil {
		t.Errorf("sold-out day price = %v, want nil", soldOut.Price)
	}
	if soldOut.DepartureTime != nil || soldOut.ArrivalTime != nil {
		t.Error("sold-out day should have nil times")
	}
}

func TestRoundTripInboundValidation(t *testing.T) {
	client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	// Outbound window valid, inbound reversed → must error on the inbound check.
	_, err := client.RoundTripFares(context.Background(), ryanair.ReturnParams{
		OneWayParams: ryanair.OneWayParams{Origin: "DUB", DateFrom: "2026-07-01", DateTo: "2026-07-15"},
		ReturnFrom:   "2026-07-22", ReturnTo: "2026-07-08",
	})
	if err == nil {
		t.Fatal("expected error for reversed inbound date range")
	}
	// Must be the inbound validation error, not a downstream network/decode error.
	if !strings.Contains(err.Error(), "inbound") {
		t.Errorf("error = %q, want the inbound date-range validation error", err)
	}
}

func TestNegativeMaxPriceRejected(t *testing.T) {
	client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := client.OneWayFares(context.Background(), ryanair.OneWayParams{
		Origin: "DUB", DateFrom: "2026-07-01", DateTo: "2026-07-02", MaxPrice: -1,
	})
	if err == nil {
		t.Fatal("expected error for negative max_price")
	}
	if !strings.Contains(err.Error(), "max price") {
		t.Errorf("error = %q, want the max-price validation error", err)
	}
}

func TestPreviousPriceMapped(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/farfnd/v4/oneWayFares":    "one_way_fares.json",
		"/farfnd/v4/roundTripFares": "round_trip_fares.json",
	}))
	ctx := context.Background()

	flights, err := client.OneWayFares(ctx, ryanair.OneWayParams{
		Origin: "DUB", DateFrom: "2026-07-01", DateTo: "2026-07-31",
	})
	if err != nil {
		t.Fatalf("OneWayFares: %v", err)
	}
	if flights[0].PreviousPrice == nil || *flights[0].PreviousPrice != 19.99 {
		t.Errorf("previous price = %v, want 19.99", flights[0].PreviousPrice)
	}
	wantUpdated := time.UnixMilli(1781642999000)
	if flights[0].PriceUpdated == nil || !flights[0].PriceUpdated.Equal(wantUpdated) {
		t.Errorf("price_updated = %v, want %v", flights[0].PriceUpdated, wantUpdated)
	}

	trips, err := client.RoundTripFares(ctx, ryanair.ReturnParams{
		OneWayParams: ryanair.OneWayParams{Origin: "DUB", DateFrom: "2026-07-01", DateTo: "2026-07-15"},
		ReturnFrom:   "2026-07-08", ReturnTo: "2026-07-22",
	})
	if err != nil {
		t.Fatalf("RoundTripFares: %v", err)
	}
	if trips[0].PreviousPrice == nil || *trips[0].PreviousPrice != 59.99 {
		t.Errorf("trip previous price = %v, want 59.99", trips[0].PreviousPrice)
	}
	if !trips[0].NewRoute {
		t.Error("expected new_route to be true")
	}
}

func TestReversedDateRangeRejected(t *testing.T) {
	client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := client.OneWayFares(context.Background(), ryanair.OneWayParams{
		Origin: "DUB", DateFrom: "2026-07-31", DateTo: "2026-07-01",
	})
	if err == nil {
		t.Fatal("expected error for reversed date range (from after to)")
	}
}

func TestMalformedDateErrors(t *testing.T) {
	client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body := `{"fares":[{"outbound":{"departureDate":"not-a-date","arrivalDate":"2026-07-01T10:00:00","price":{"value":10,"currencyCode":"EUR"},"flightNumber":"FR1"}}]}`
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	_, err := client.OneWayFares(context.Background(), ryanair.OneWayParams{
		Origin: "DUB", DateFrom: "2026-07-01", DateTo: "2026-07-02",
	})
	if err == nil {
		t.Fatal("expected error for malformed departure date")
	}
}

func TestRouteActiveDates(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/farfnd/v4/oneWayFares/DUB/STN/availabilities": "availabilities.json",
	}))
	dates, err := client.RouteActiveDates(context.Background(), "dub", "stn")
	if err != nil {
		t.Fatalf("RouteActiveDates: %v", err)
	}
	if len(dates) != 3 {
		t.Fatalf("dates = %d, want 3", len(dates))
	}
	if dates[0] != "2026-07-01" {
		t.Errorf("dates[0] = %q, want 2026-07-01", dates[0])
	}
	if _, err := client.RouteActiveDates(context.Background(), "XX", "STN"); err == nil {
		t.Error("expected error for invalid origin IATA")
	}
}

func TestCheapestReturnPerDay(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/farfnd/v4/roundTripFares/DUB/STN/cheapestPerDay": "return_cheapest_per_day.json",
	}))
	// Empty inbound month must default to the outbound month (asserted below).
	res, err := client.CheapestReturnPerDay(context.Background(), ryanair.ReturnCalendarParams{Origin: "dub", Destination: "stn", OutboundMonth: "2026-07-01", MinTripDays: 2, MaxTripDays: 3, Currency: "EUR"})
	if err != nil {
		t.Fatalf("CheapestReturnPerDay: %v", err)
	}
	if len(res.Outbound) != 3 || len(res.Inbound) != 3 {
		t.Fatalf("outbound=%d inbound=%d, want 3/3", len(res.Outbound), len(res.Inbound))
	}
	if res.Outbound[0].Day != "2026-07-03" || res.Outbound[0].Price == nil {
		t.Errorf("outbound[0] = %+v", res.Outbound[0])
	}
	q := fs.lastQuery.Load()
	for key, want := range map[string]string{
		"outboundMonthOfDate": "2026-07-01",
		"inboundMonthOfDate":  "2026-07-01",
		"durationFrom":        "2",
		"durationTo":          "3",
		"currency":            "EUR",
	} {
		if got := q.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if _, err := client.CheapestReturnPerDay(context.Background(), ryanair.ReturnCalendarParams{Origin: "XX", Destination: "STN", OutboundMonth: "2026-07-01"}); err == nil {
		t.Error("expected error for invalid origin IATA")
	}
	if _, err := client.CheapestReturnPerDay(context.Background(), ryanair.ReturnCalendarParams{Origin: "DUB", Destination: "STN", OutboundMonth: "2026-07-01", InboundMonth: "not-a-month"}); err == nil {
		t.Error("expected error for malformed inbound month")
	}
}

func TestCheapestWeekend(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/farfnd/v4/roundTripFares/DUB/STN/cheapestPerDay": "return_cheapest_per_day.json",
	}))
	trip, err := client.CheapestWeekend(context.Background(), ryanair.WeekendParams{Origin: "dub", Destination: "stn", MonthsAhead: 1, WeekendLength: 2})
	if err != nil {
		t.Fatalf("CheapestWeekend: %v", err)
	}
	if trip == nil {
		t.Fatal("expected a weekend trip")
	}
	// Cheapest Fri->Sun pair in the fixture: 2026-07-10 (15.00) + 2026-07-12 (18.00).
	if trip.Outbound.Day != "2026-07-10" || trip.Inbound.Day != "2026-07-12" {
		t.Errorf("trip days = %s -> %s, want 2026-07-10 -> 2026-07-12", trip.Outbound.Day, trip.Inbound.Day)
	}
	if trip.TotalPrice != 33.00 {
		t.Errorf("total = %v, want 33.00", trip.TotalPrice)
	}
	if _, err := client.CheapestWeekend(context.Background(), ryanair.WeekendParams{Origin: "DUB", Destination: "STN", MonthsAhead: 1, WeekendLength: 5}); err == nil {
		t.Error("expected error for invalid weekend_length")
	}
	if _, err := client.CheapestWeekend(context.Background(), ryanair.WeekendParams{Origin: "DUB", Destination: "STN", MonthsAhead: 0, WeekendLength: 2}); err == nil {
		t.Error("expected error for months_ahead < 1")
	}
}

func TestRoundTripNotTruncated(t *testing.T) {
	// Precondition: the fixture must carry a non-null nextPage to mirror the live
	// endpoint. If this drifts, the assertion below stops proving the exemption.
	var raw struct {
		NextPage *int `json:"nextPage"`
	}
	if err := json.Unmarshal(fixture(t, "round_trip_fares.json"), &raw); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if raw.NextPage == nil {
		t.Fatal("round_trip_fares.json must keep a non-null nextPage to exercise the guard exemption")
	}
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/farfnd/v4/roundTripFares": "round_trip_fares.json",
	}))
	if _, err := client.RoundTripFares(context.Background(), ryanair.ReturnParams{
		OneWayParams: ryanair.OneWayParams{Origin: "DUB", DateFrom: "2026-07-01", DateTo: "2026-07-15"},
		ReturnFrom:   "2026-07-08", ReturnTo: "2026-07-22",
	}); err != nil {
		t.Fatalf("RoundTripFares must not error on a nextPage:1 (complete) response: %v", err)
	}
}

// TestTruncatedFaresRejected guards oneWayFares against silent data loss: we
// always request the full result set (no limit), so a non-null nextPage means
// Ryanair began capping responses. Since the endpoint exposes no working cursor
// to fetch the rest, OneWayFares must fail loudly (returning a nil slice) rather
// than return a partial list. (roundTripFares is intentionally not guarded — it
// returns nextPage=1 on every response, so it has no usable truncation signal;
// TestRoundTripFares covers that its nextPage=1 fixture does NOT error.)
func TestTruncatedFaresRejected(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/farfnd/v4/oneWayFares": "one_way_fares_truncated.json",
	}))
	flights, err := client.OneWayFares(context.Background(), ryanair.OneWayParams{
		Origin: "DUB", DateFrom: "2026-07-01", DateTo: "2026-07-31",
	})
	if err == nil {
		t.Fatal("expected error for truncated (paginated) fares response")
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Errorf("error = %q, want it to mention truncation", err)
	}
	if flights != nil {
		t.Error("expected nil slice on truncation error")
	}
}
