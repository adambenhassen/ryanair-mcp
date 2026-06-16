package ryanair_test

import (
	"context"
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
