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

func TestExploreWithFares(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/api/views/locate/3/aggregate/all/en": "network.json",
		"/farfnd/v4/oneWayFares":               "one_way_fares.json",
	}))

	dests, err := client.ExploreDestinations(context.Background(), "DUB", true, ryanair.OneWayParams{
		DateFrom: "2026-07-01",
		DateTo:   "2026-07-31",
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
	if flights[0].PriceUpdated == nil {
		t.Error("expected price_updated to be set")
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
