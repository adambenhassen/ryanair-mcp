package ryanair_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adambenhassen/ryanair-mcp/internal/ryanair"
)

// redirectTransport rewrites every request to the test server while preserving
// path and query, so the client's hard-coded hosts hit our fixtures.
type redirectTransport struct {
	base *url.URL
}

func (rt redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = rt.base.Scheme
	req.URL.Host = rt.base.Host
	return http.DefaultTransport.RoundTrip(req)
}

// fakeServer records prime hits and the last request query for assertions.
type fakeServer struct {
	primeHits atomic.Int32
	lastQuery atomic.Pointer[url.Values]
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func newClient(t *testing.T, handler http.Handler) *ryanair.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	httpClient := &http.Client{Transport: redirectTransport{base: base}}
	client, err := ryanair.NewClient(ryanair.WithHTTPClient(httpClient), ryanair.WithNetworkTTL(time.Minute))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

// routeFixtures builds a handler serving the given path-prefix -> fixture map,
// recording prime hits and the last query string.
func routeFixtures(t *testing.T, fs *fakeServer, routes map[string]string) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fs.primeHits.Add(1)
			w.WriteHeader(http.StatusOK)
			return
		}
		q := r.URL.Query()
		fs.lastQuery.Store(&q)
		for prefix, name := range routes {
			if strings.HasPrefix(r.URL.Path, prefix) {
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write(fixture(t, name)); err != nil {
					t.Errorf("write fixture: %v", err)
				}
				return
			}
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	return mux
}

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

	days, err := client.CheapestPerDay(context.Background(), "DUB", "STN", "2026-07-01", "")
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

func TestSchedulesValidation(t *testing.T) {
	client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ctx := context.Background()
	cases := []struct {
		name         string
		origin, dest string
		year, month  int
		wantErr      string // substring proving validation rejected it (not a network decode error)
	}{
		{"bad month low", "DUB", "STN", 2026, 0, "invalid month"},
		{"bad month high", "DUB", "STN", 2026, 13, "invalid month"},
		{"bad year low", "DUB", "STN", 1999, 7, "invalid year"},
		{"bad year high", "DUB", "STN", 2101, 7, "invalid year"},
		{"bad origin", "XX", "STN", 2026, 7, "invalid route"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.Schedules(ctx, tc.origin, tc.dest, tc.year, tc.month)
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q (a validation error, not a network failure)", err, tc.wantErr)
			}
		})
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

func TestSchedules(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/timtbl/3/schedules": "schedules.json",
	}))

	flights, err := client.Schedules(context.Background(), "DUB", "STN", 2026, 7)
	if err != nil {
		t.Fatalf("Schedules: %v", err)
	}
	if len(flights) == 0 {
		t.Fatal("expected timetable flights")
	}
	if !strings.HasPrefix(flights[0].FlightNumber, "FR") {
		t.Errorf("flight number = %q, want FR prefix", flights[0].FlightNumber)
	}
}

func TestListAirportsAndRoutes(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/api/views/locate/3/aggregate/all/en": "network.json",
	}))
	ctx := context.Background()

	all, err := client.ListAirports(ctx, "")
	if err != nil {
		t.Fatalf("ListAirports: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("airports = %d, want 4", len(all))
	}

	ie, err := client.ListAirports(ctx, "IE")
	if err != nil {
		t.Fatalf("ListAirports(IE): %v", err)
	}
	if len(ie) != 1 || ie[0].IataCode != "DUB" {
		t.Errorf("IE airports = %+v, want [DUB]", ie)
	}

	ok, err := client.ValidateRoute(ctx, "DUB", "STN")
	if err != nil {
		t.Fatalf("ValidateRoute: %v", err)
	}
	if !ok {
		t.Error("DUB-STN should be a valid route")
	}

	// Seasonal route should also count.
	seasonal, err := client.ValidateRoute(ctx, "DUB", "AGA")
	if err != nil {
		t.Fatalf("ValidateRoute seasonal: %v", err)
	}
	if !seasonal {
		t.Error("DUB-AGA seasonal route should validate")
	}

	none, err := client.ValidateRoute(ctx, "STN", "BCN")
	if err != nil {
		t.Fatalf("ValidateRoute none: %v", err)
	}
	if none {
		t.Error("STN-BCN should not be a route")
	}
}

func TestNetworkMetadataDepth(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/api/views/locate/3/aggregate/all/en": "network.json",
	}))
	airports, err := client.ListAirports(context.Background(), "IE")
	if err != nil {
		t.Fatalf("ListAirports: %v", err)
	}
	dub := airports[0]
	if dub.TimeZone != "Europe/Dublin" {
		t.Errorf("timezone = %q, want Europe/Dublin", dub.TimeZone)
	}
	if dub.CityCode != "DUBLIN" || dub.CurrencyCode != "EUR" {
		t.Errorf("city/currency = %q/%q", dub.CityCode, dub.CurrencyCode)
	}
	if dub.CityName != "Dublin" {
		t.Errorf("city name = %q, want Dublin", dub.CityName)
	}
	if dub.RegionCode != "LEINSTER" || dub.RegionName != "Leinster" {
		t.Errorf("region = %q/%q, want LEINSTER/Leinster", dub.RegionCode, dub.RegionName)
	}
	if dub.CountryName != "Ireland" {
		t.Errorf("country name = %q, want Ireland", dub.CountryName)
	}
	if len(dub.Aliases) == 0 {
		t.Error("expected aliases")
	}
}

func TestExploreWithFares(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/api/views/locate/3/aggregate/all/en": "network.json",
		"/farfnd/v4/oneWayFares":               "one_way_fares.json",
	}))

	dests, err := client.ExploreDestinations(context.Background(), ryanair.ExploreParams{
		Origin:    "DUB",
		WithFares: true,
		Fare:      ryanair.FareWindow{DateFrom: "2026-07-01", DateTo: "2026-07-31"},
	})
	if err != nil {
		t.Fatalf("ExploreDestinations: %v", err)
	}
	if len(dests) == 0 {
		t.Fatal("expected destinations from DUB")
	}
	var annotated int
	for _, d := range dests {
		if d.Fare != nil {
			annotated++
		}
	}
	if annotated == 0 {
		t.Error("expected at least one destination annotated with a fare")
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

func TestExploreSeasonalAndFilter(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/api/views/locate/3/aggregate/all/en": "network.json",
	}))
	ctx := context.Background()

	dests, err := client.ExploreDestinations(ctx, ryanair.ExploreParams{Origin: "DUB"})
	if err != nil {
		t.Fatalf("ExploreDestinations: %v", err)
	}
	byCode := map[string]ryanair.Destination{}
	for _, d := range dests {
		byCode[d.IataCode] = d
	}
	if aga, ok := byCode["AGA"]; !ok || !aga.Seasonal {
		t.Errorf("AGA should be present and seasonal, got %+v (ok=%v)", aga, ok)
	}
	// BCN is in both regular and seasonal routes; regular wins, so non-seasonal.
	if bcn, ok := byCode["BCN"]; !ok || bcn.Seasonal {
		t.Errorf("BCN served both ways should be non-seasonal, got %+v (ok=%v)", bcn, ok)
	}
	// BCN must appear exactly once despite being in both route sets.
	var bcnCount int
	for _, d := range dests {
		if d.IataCode == "BCN" {
			bcnCount++
		}
	}
	if bcnCount != 1 {
		t.Errorf("BCN appears %d times, want 1 (dedup across regular/seasonal)", bcnCount)
	}

	es, err := client.ExploreDestinations(ctx, ryanair.ExploreParams{Origin: "DUB", Country: "ES"})
	if err != nil {
		t.Fatalf("explore ES: %v", err)
	}
	if len(es) != 1 || es[0].IataCode != "BCN" {
		t.Errorf("country filter = %+v, want [BCN]", es)
	}

	region, err := client.ExploreDestinations(ctx, ryanair.ExploreParams{Origin: "DUB", Region: "ENGLAND"})
	if err != nil {
		t.Fatalf("explore region: %v", err)
	}
	if len(region) != 1 || region[0].IataCode != "STN" {
		t.Errorf("region filter = %+v, want [STN]", region)
	}

	city, err := client.ExploreDestinations(ctx, ryanair.ExploreParams{Origin: "DUB", City: "LONDON"})
	if err != nil {
		t.Fatalf("explore city: %v", err)
	}
	if len(city) != 1 || city[0].IataCode != "STN" {
		t.Errorf("city filter = %+v, want [STN]", city)
	}

	if _, err := client.ExploreDestinations(ctx, ryanair.ExploreParams{Origin: "XX"}); err == nil {
		t.Error("expected error for invalid origin IATA")
	}
}

func TestExploreWithFaresAndFilter(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/api/views/locate/3/aggregate/all/en": "network.json",
		"/farfnd/v4/oneWayFares":               "one_way_fares.json",
	}))
	dests, err := client.ExploreDestinations(context.Background(), ryanair.ExploreParams{
		Origin: "DUB", Country: "ma", WithFares: true,
		Fare: ryanair.FareWindow{DateFrom: "2026-07-01", DateTo: "2026-07-31"},
	})
	if err != nil {
		t.Fatalf("explore with fares+filter: %v", err)
	}
	// Country filter narrows to AGA; the surviving destination still gets its
	// cheapest fare (63.59, the cheaper of the two AGA fares in the fixture).
	if len(dests) != 1 || dests[0].IataCode != "AGA" {
		t.Fatalf("filtered dests = %+v, want [AGA]", dests)
	}
	if dests[0].Fare == nil || *dests[0].Fare != 63.59 {
		t.Errorf("AGA fare = %v, want cheapest 63.59", dests[0].Fare)
	}
	// ACE is in the fares fixture but is not a DUB network destination — it must
	// never leak into the output.
	for _, d := range dests {
		if d.IataCode == "ACE" {
			t.Error("ACE should not appear (not a network destination)")
		}
	}
}

func TestExploreNetworkError(t *testing.T) {
	client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	if _, err := client.ExploreDestinations(context.Background(), ryanair.ExploreParams{Origin: "DUB"}); err == nil {
		t.Fatal("expected error when the network bundle fails to load")
	}
}

func TestExploreUnknownFilterErrors(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/api/views/locate/3/aggregate/all/en": "network.json",
	}))
	if _, err := client.ExploreDestinations(context.Background(), ryanair.ExploreParams{
		Origin: "DUB", Region: "ATLANTIS",
	}); err == nil {
		t.Error("expected error for unknown region code")
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

func TestAnywhereUnder(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/farfnd/v4/oneWayFares": "one_way_fares.json",
	}))
	flights, err := client.AnywhereUnder(context.Background(), ryanair.OneWayParams{
		Origin: "DUB", DateFrom: "2026-07-01", DateTo: "2026-07-31", MaxPrice: 100,
	})
	if err != nil {
		t.Fatalf("AnywhereUnder: %v", err)
	}
	seen := map[string]bool{}
	for i, f := range flights {
		if seen[f.Destination] {
			t.Errorf("duplicate destination %q", f.Destination)
		}
		seen[f.Destination] = true
		if i > 0 && flights[i-1].Price > f.Price {
			t.Error("results not sorted ascending by price")
		}
	}
	// AGA appears twice (63.59 and 89.00); cheapest must win.
	for _, f := range flights {
		if f.Destination == "AGA" && f.Price != 63.59 {
			t.Errorf("AGA price = %v, want cheapest 63.59", f.Price)
		}
	}

	if _, err := client.AnywhereUnder(context.Background(), ryanair.OneWayParams{
		Origin: "DUB", DateFrom: "2026-07-01", DateTo: "2026-07-31",
	}); err == nil {
		t.Error("expected error when max_price is missing")
	}

	// Caller-supplied Destination/Country must be stripped (network-wide probe).
	if _, err := client.AnywhereUnder(context.Background(), ryanair.OneWayParams{
		Origin: "DUB", DateFrom: "2026-07-01", DateTo: "2026-07-31", MaxPrice: 100,
		Destination: "STN", Country: "ES",
	}); err != nil {
		t.Fatalf("AnywhereUnder with dest/country: %v", err)
	}
	q := fs.lastQuery.Load()
	if got := q.Get("arrivalAirportIataCode"); got != "" {
		t.Errorf("arrivalAirportIataCode = %q, want empty (stripped)", got)
	}
	if got := q.Get("arrivalCountryCode"); got != "" {
		t.Errorf("arrivalCountryCode = %q, want empty (stripped)", got)
	}
}

func TestRetryClassification(t *testing.T) {
	t.Run("429 is retried then succeeds", func(t *testing.T) {
		var calls atomic.Int32
		client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				w.WriteHeader(http.StatusOK)
				return
			}
			if calls.Add(1) == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write(fixture(t, "one_way_fares.json")); err != nil {
				t.Errorf("write: %v", err)
			}
		}))
		_, err := client.OneWayFares(context.Background(), ryanair.OneWayParams{
			Origin: "DUB", DateFrom: "2026-07-01", DateTo: "2026-07-02",
		})
		if err != nil {
			t.Fatalf("expected success after retry, got %v", err)
		}
		if calls.Load() != 2 {
			t.Errorf("calls = %d, want 2 (one retry)", calls.Load())
		}
	})

	t.Run("409 is not retried", func(t *testing.T) {
		var calls atomic.Int32
		client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				w.WriteHeader(http.StatusOK)
				return
			}
			calls.Add(1)
			http.Error(w, `{"message":"Availability declined"}`, http.StatusConflict)
		}))
		_, err := client.OneWayFares(context.Background(), ryanair.OneWayParams{
			Origin: "DUB", DateFrom: "2026-07-01", DateTo: "2026-07-02",
		})
		var apiErr *ryanair.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected *ryanair.APIError, got %v", err)
		}
		if apiErr.Status != http.StatusConflict {
			t.Errorf("status = %d, want 409", apiErr.Status)
		}
		if calls.Load() != 1 {
			t.Errorf("calls = %d, want 1 (no retry on 409)", calls.Load())
		}
	})
}

func TestValidationRejectsBadInput(t *testing.T) {
	client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := client.OneWayFares(context.Background(), ryanair.OneWayParams{
		Origin: "XX", DateFrom: "2026-07-01", DateTo: "2026-07-02",
	})
	if err == nil {
		t.Fatal("expected error for invalid origin IATA")
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
	res, err := client.CheapestReturnPerDay(context.Background(), "dub", "stn", "2026-07-01", "", 2, 3, "EUR")
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
	if _, err := client.CheapestReturnPerDay(context.Background(), "XX", "STN", "2026-07-01", "", 0, 0, ""); err == nil {
		t.Error("expected error for invalid origin IATA")
	}
	if _, err := client.CheapestReturnPerDay(context.Background(), "DUB", "STN", "2026-07-01", "not-a-month", 0, 0, ""); err == nil {
		t.Error("expected error for malformed inbound month")
	}
}

func TestCheapestWeekend(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/farfnd/v4/roundTripFares/DUB/STN/cheapestPerDay": "return_cheapest_per_day.json",
	}))
	trip, err := client.CheapestWeekend(context.Background(), "dub", "stn", 1, 2)
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
	if _, err := client.CheapestWeekend(context.Background(), "DUB", "STN", 1, 5); err == nil {
		t.Error("expected error for invalid weekend_length")
	}
	if _, err := client.CheapestWeekend(context.Background(), "DUB", "STN", 0, 2); err == nil {
		t.Error("expected error for months_ahead < 1")
	}
}

func TestCheapestWeekendTripMatcher(t *testing.T) {
	price := func(v float64) *float64 { return &v }
	fare := func(day string, p *float64) ryanair.DailyFare {
		return ryanair.DailyFare{Day: day, Price: p, Currency: "EUR"}
	}
	cases := []struct {
		name            string
		outbounds       []ryanair.DailyFare
		inbounds        []ryanair.DailyFare
		length          int
		wantNil         bool
		wantOut, wantIn string
		wantTotal       float64
	}{
		{
			name: "picks cheapest priced Friday pair, ignoring non-Fridays (Thu and Sat)",
			outbounds: []ryanair.DailyFare{
				fare("2026-07-03", price(20)), // Fri
				fare("2026-07-10", price(15)), // Fri (cheaper)
				fare("2026-07-04", price(1)),  // Sat - must be ignored despite lowest price
				fare("2026-07-02", price(1)),  // Thu - must be ignored (pins Friday, not just "skip Sat")
			},
			inbounds: []ryanair.DailyFare{
				fare("2026-07-05", price(25)), // Sun -> pairs 07-03
				fare("2026-07-12", price(18)), // Sun -> pairs 07-10
				fare("2026-07-06", price(1)),  // Mon -> +2 from 07-04 (a Sat, never matched)
				fare("2026-07-04", price(1)),  // Sat -> +2 from 07-02 (a Thu, never matched)
			},
			length: 2, wantOut: "2026-07-10", wantIn: "2026-07-12", wantTotal: 33,
		},
		{
			name: "Fri->Mon (weekend_length 3) offset",
			outbounds: []ryanair.DailyFare{
				fare("2026-07-03", price(10)), // Fri
			},
			inbounds: []ryanair.DailyFare{
				fare("2026-07-05", price(99)), // Sun (+2) - wrong offset, must not match
				fare("2026-07-06", price(5)),  // Mon (+3) - the length-3 match
			},
			length: 3, wantOut: "2026-07-03", wantIn: "2026-07-06", wantTotal: 15,
		},
		{
			name: "skips nil-priced Friday in favor of a priced one",
			outbounds: []ryanair.DailyFare{
				fare("2026-07-10", nil),       // Fri, no price -> skipped
				fare("2026-07-03", price(40)), // Fri, priced
			},
			inbounds: []ryanair.DailyFare{
				fare("2026-07-05", price(10)), // pairs 07-03
				fare("2026-07-12", price(1)),  // would pair 07-10, but that outbound is unpriced
			},
			length: 2, wantOut: "2026-07-03", wantIn: "2026-07-05", wantTotal: 50,
		},
		{
			name: "skips Friday whose matching inbound is nil-priced",
			outbounds: []ryanair.DailyFare{
				fare("2026-07-10", price(1)),  // Fri (cheapest) but its inbound is unpriced
				fare("2026-07-03", price(40)), // Fri, fully priced pair
			},
			inbounds: []ryanair.DailyFare{
				fare("2026-07-12", nil),       // Sun -> would pair 07-10, but unpriced -> skipped
				fare("2026-07-05", price(10)), // Sun -> pairs 07-03
			},
			length: 2, wantOut: "2026-07-03", wantIn: "2026-07-05", wantTotal: 50,
		},
		{
			name:      "Friday with no matching inbound -> nil",
			outbounds: []ryanair.DailyFare{fare("2026-07-03", price(20))}, // Fri
			inbounds:  []ryanair.DailyFare{fare("2026-07-04", price(20))}, // not +2 days
			length:    2, wantNil: true,
		},
		{
			name:      "no Friday outbound -> nil",
			outbounds: []ryanair.DailyFare{fare("2026-07-04", price(10))}, // Sat
			inbounds:  []ryanair.DailyFare{fare("2026-07-06", price(10))},
			length:    2, wantNil: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trip, err := ryanair.CheapestWeekendTrip(tc.outbounds, tc.inbounds, tc.length)
			if err != nil {
				t.Fatalf("matcher: %v", err)
			}
			if tc.wantNil {
				if trip != nil {
					t.Fatalf("want nil trip, got %+v", trip)
				}
				return
			}
			if trip == nil {
				t.Fatal("want a trip, got nil")
			}
			if trip.Outbound.Day != tc.wantOut || trip.Inbound.Day != tc.wantIn {
				t.Errorf("pair = %s->%s, want %s->%s", trip.Outbound.Day, trip.Inbound.Day, tc.wantOut, tc.wantIn)
			}
			if trip.TotalPrice != tc.wantTotal {
				t.Errorf("total = %v, want %v", trip.TotalPrice, tc.wantTotal)
			}
			if trip.Currency != "EUR" {
				t.Errorf("currency = %q, want EUR", trip.Currency)
			}
		})
	}
}

func TestCheapestWeekendTripMalformedDay(t *testing.T) {
	v := 10.0
	out := []ryanair.DailyFare{{Day: "not-a-date", Price: &v, Currency: "EUR"}}
	if _, err := ryanair.CheapestWeekendTrip(out, nil, 2); err == nil {
		t.Fatal("expected error for malformed outbound day")
	}
}

// TestRoundTripNotTruncated locks in that roundTripFares is exempt from the
// oneWayFares truncation guard: the live endpoint returns nextPage=1 on every
// (even complete) response, so the guard must NOT fire there.
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

func TestActiveAirports(t *testing.T) {
	client := newClient(t, routeFixtures(t, &fakeServer{}, map[string]string{
		"/api/views/locate/5/airports/en/active": "active_airports.json",
	}))
	airports, err := client.ActiveAirports(context.Background())
	if err != nil {
		t.Fatalf("ActiveAirports: %v", err)
	}
	if len(airports) != 2 {
		t.Fatalf("airports = %d, want 2", len(airports))
	}
	dub := airports[0]
	if dub.IataCode != "DUB" || dub.RegionName != "Leinster" || dub.CurrencyCode != "EUR" || dub.TimeZone != "Europe/Dublin" {
		t.Errorf("unexpected airport %+v", dub)
	}
	if !dub.Base {
		t.Error("DUB should be a base")
	}
}

func TestAirportInfo(t *testing.T) {
	client := newClient(t, routeFixtures(t, &fakeServer{}, map[string]string{
		"/api/views/locate/5/airports/en/DUB": "airport_info.json",
	}))
	a, err := client.AirportInfo(context.Background(), "dub")
	if err != nil {
		t.Fatalf("AirportInfo: %v", err)
	}
	if a.IataCode != "DUB" || a.CountryName != "Ireland" || a.CityCode != "DUBLIN" || a.RegionCode != "LEINSTER" {
		t.Errorf("unexpected airport %+v", a)
	}
}

func TestAirportInfoRejectsBadIATA(t *testing.T) {
	client := newClient(t, routeFixtures(t, &fakeServer{}, map[string]string{}))
	if _, err := client.AirportInfo(context.Background(), "XX"); err == nil {
		t.Fatal("expected error for invalid IATA")
	}
}

func TestAirportDestinations(t *testing.T) {
	client := newClient(t, routeFixtures(t, &fakeServer{}, map[string]string{
		"/api/views/locate/searchWidget/routes/en/airport/DUB": "airport_destinations.json",
	}))
	dests, err := client.AirportDestinations(context.Background(), "DUB")
	if err != nil {
		t.Fatalf("AirportDestinations: %v", err)
	}
	if len(dests) != 2 {
		t.Fatalf("dests = %d, want 2", len(dests))
	}
	if dests[0].IataCode != "ACE" || dests[0].Operator != "FR" || dests[0].Seasonal {
		t.Errorf("unexpected first dest %+v", dests[0])
	}
	agp := dests[1]
	if !agp.Seasonal || !agp.Recent || len(agp.Tags) != 1 || agp.Tags[0] != "popular" {
		t.Errorf("unexpected malaga dest %+v", agp)
	}
}

func TestNearbyAirports(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/api/geoloc/v5/nearbyAirports": "nearby_airports.json",
	}))
	airports, err := client.NearbyAirports(context.Background(), "")
	if err != nil {
		t.Fatalf("NearbyAirports: %v", err)
	}
	if len(airports) != 2 {
		t.Fatalf("airports = %d, want 2", len(airports))
	}
	stn := airports[0]
	if stn.IataCode != "STN" || stn.CountryCode != "gb" || stn.CityName != "London" {
		t.Errorf("unexpected airport %+v", stn)
	}
	// Lean geoloc shape: region/timezone/currency/base are absent and must map
	// to zero values, not be fabricated.
	if stn.RegionName != "" || stn.TimeZone != "" || stn.CurrencyCode != "" || stn.Base {
		t.Errorf("geoloc airport should have empty region/timezone/currency/base, got %+v", stn)
	}
	if q := fs.lastQuery.Load(); q == nil || q.Get("market") != "en-gb" {
		t.Errorf("market query = %v, want en-gb default", q)
	}

	// An explicit market must be forwarded, not overwritten by the default.
	if _, err := client.NearbyAirports(context.Background(), "fr-fr"); err != nil {
		t.Fatalf("NearbyAirports(fr-fr): %v", err)
	}
	if q := fs.lastQuery.Load(); q == nil || q.Get("market") != "fr-fr" {
		t.Errorf("market query = %v, want fr-fr", q)
	}
}

func TestDefaultAirport(t *testing.T) {
	client := newClient(t, routeFixtures(t, &fakeServer{}, map[string]string{
		"/api/geoloc/v5/defaultAirport": "default_airport.json",
	}))
	a, err := client.DefaultAirport(context.Background())
	if err != nil {
		t.Fatalf("DefaultAirport: %v", err)
	}
	if a.IataCode != "DUB" || a.CountryName != "Ireland" || a.CityCode != "DUBLIN" || a.Latitude == 0 {
		t.Errorf("unexpected airport %+v", a)
	}
	// Lean geoloc shape: these fields are absent upstream and must map to zero.
	if a.RegionName != "" || a.TimeZone != "" || a.CurrencyCode != "" || a.Base {
		t.Errorf("geoloc airport should have empty region/timezone/currency/base, got %+v", a)
	}
}

func TestActiveAirportsNetworkError(t *testing.T) {
	client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	if _, err := client.ActiveAirports(context.Background()); err == nil {
		t.Fatal("expected error when the endpoint returns 500")
	}
}

func TestAirportDestinationsRejectsBadIATA(t *testing.T) {
	client := newClient(t, routeFixtures(t, &fakeServer{}, map[string]string{}))
	if _, err := client.AirportDestinations(context.Background(), "XX"); err == nil {
		t.Fatal("expected error for invalid origin IATA")
	}
}
