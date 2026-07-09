package tools_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adambenhassen/ryanair-mcp/internal/ryanair"
	"github.com/adambenhassen/ryanair-mcp/internal/tools"
)

// rewriteHost redirects the client's hard-coded hosts to the test server.
type rewriteHost struct{ base *url.URL }

func (rt rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = rt.base.Scheme
	req.URL.Host = rt.base.Host
	return http.DefaultTransport.RoundTrip(req)
}

// capturedRequest records the last non-priming request a handler issued, so
// tests can assert input fields were mapped onto the right query/path segments.
type capturedRequest struct {
	path  string
	query url.Values
}

// exploreClient builds a ryanair.Client backed by the ryanair package fixtures.
func exploreClient(t *testing.T) *ryanair.Client {
	t.Helper()
	return fixtureClient(t, nil)
}

// fixtureClient builds a ryanair.Client whose test server replays the ryanair
// package fixtures for every supported endpoint. When capture is non-nil, the
// last non-priming request's path and query are recorded into it.
func fixtureClient(t *testing.T, capture *capturedRequest) *ryanair.Client {
	t.Helper()
	serve := func(w http.ResponseWriter, name string) {
		b, err := os.ReadFile(filepath.Join("..", "ryanair", "testdata", name))
		if err != nil {
			t.Errorf("read fixture %s: %v", name, err)
			http.Error(w, "fixture", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(b); err != nil {
			t.Errorf("write fixture: %v", err)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if capture != nil && r.URL.Path != "/" {
			capture.path = r.URL.Path
			capture.query = r.URL.Query()
		}
		switch {
		case r.URL.Path == "/":
			w.WriteHeader(http.StatusOK)
		case strings.HasPrefix(r.URL.Path, "/api/views/locate/3/aggregate/all/en"):
			serve(w, "network.json")
		case strings.HasPrefix(r.URL.Path, "/api/views/locate/5/airports/en/"):
			serve(w, "airport_info.json")
		case strings.HasPrefix(r.URL.Path, "/api/views/locate/searchWidget/routes/en/airport/"):
			serve(w, "explore_route_metadata.json")
		case strings.HasSuffix(r.URL.Path, "/availabilities"):
			serve(w, "availabilities.json")
		case strings.Contains(r.URL.Path, "/oneWayFares/") && strings.HasSuffix(r.URL.Path, "/cheapestPerDay"):
			serve(w, "cheapest_per_day.json")
		case strings.Contains(r.URL.Path, "/roundTripFares/") && strings.HasSuffix(r.URL.Path, "/cheapestPerDay"):
			serve(w, "return_cheapest_per_day.json")
		case strings.HasPrefix(r.URL.Path, "/farfnd/v4/roundTripFares"):
			serve(w, "round_trip_fares.json")
		case strings.HasPrefix(r.URL.Path, "/farfnd/v4/oneWayFares"):
			serve(w, "one_way_fares.json")
		case strings.HasPrefix(r.URL.Path, "/timtbl/3/schedules"):
			serve(w, "schedules.json")
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	client, err := ryanair.NewClient(
		ryanair.WithHTTPClient(&http.Client{Transport: rewriteHost{base: base}}),
		ryanair.WithNetworkTTL(time.Minute),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func TestExploreHandlerFlatVsGrouped(t *testing.T) {
	c := exploreClient(t)

	flat, err := tools.RunExplore(c, tools.ExploreArgs{Origin: "DUB"})
	if err != nil {
		t.Fatalf("flat: %v", err)
	}
	if len(flat.Destinations) == 0 || flat.Groups != nil {
		t.Errorf("flat shape wrong: dests=%d groups=%v", len(flat.Destinations), flat.Groups)
	}

	grouped, err := tools.RunExplore(c, tools.ExploreArgs{Origin: "DUB", GroupBy: "country"})
	if err != nil {
		t.Fatalf("grouped: %v", err)
	}
	if len(grouped.Groups) == 0 || grouped.Destinations != nil {
		t.Errorf("grouped shape wrong: dests=%v groups=%d", grouped.Destinations, len(grouped.Groups))
	}
	for _, g := range grouped.Groups {
		if g.Key == "" || g.Name == "" {
			t.Errorf("group missing key/name: %+v", g)
		}
	}
}

func TestExploreHandlerInvalidGroupBy(t *testing.T) {
	// Invalid group_by must fail before any network call; a client with no
	// server backing would otherwise error differently.
	c := exploreClient(t)
	if _, err := tools.RunExplore(c, tools.ExploreArgs{Origin: "DUB", GroupBy: "city"}); err == nil {
		t.Fatal("expected error for invalid group_by")
	}
}

func TestExploreHandlerCityFilterMapping(t *testing.T) {
	c := exploreClient(t)
	res, err := tools.RunExplore(c, tools.ExploreArgs{Origin: "DUB", City: "LONDON"})
	if err != nil {
		t.Fatalf("city filter: %v", err)
	}
	if len(res.Destinations) != 1 || res.Destinations[0].IataCode != "STN" {
		t.Errorf("city filter mapping = %+v, want [STN]", res.Destinations)
	}
}

func TestActiveDatesHandler(t *testing.T) {
	c := exploreClient(t)
	dates, err := tools.RunActiveDates(c, "DUB", "STN")
	if err != nil {
		t.Fatalf("active dates: %v", err)
	}
	if len(dates) != 3 || dates[0] != "2026-07-01" {
		t.Errorf("dates = %v, want 3 starting 2026-07-01", dates)
	}
}

func TestCheapestReturnPerDayHandler(t *testing.T) {
	c := exploreClient(t)
	out, in, err := tools.RunCheapestReturnPerDay(c, "DUB", "STN", "2026-07-01", "", 2, 3, "EUR")
	if err != nil {
		t.Fatalf("return cpd: %v", err)
	}
	if len(out) != 3 || len(in) != 3 {
		t.Errorf("out=%d in=%d, want 3/3", len(out), len(in))
	}
}

func TestCheapestWeekendHandler(t *testing.T) {
	c := exploreClient(t)
	trip, err := tools.RunCheapestWeekend(c, "DUB", "STN", 1, 2)
	if err != nil {
		t.Fatalf("weekend: %v", err)
	}
	if trip == nil || trip.TotalPrice != 33.00 {
		t.Errorf("trip = %+v, want total 33.00", trip)
	}
	// Omitted weekend_length (0) must default to a valid value (Fri->Sun).
	if _, err := tools.RunCheapestWeekend(c, "DUB", "STN", 1, 0); err != nil {
		t.Errorf("default weekend_length: %v", err)
	}
}

func TestListAirportsHandlerCodeLookup(t *testing.T) {
	c := exploreClient(t)
	got, err := tools.RunListAirports(c, "", "DUB")
	if err != nil {
		t.Fatalf("airport lookup: %v", err)
	}
	if len(got) != 1 || got[0].IataCode != "DUB" || got[0].RegionName != "Leinster" {
		t.Errorf("airport = %+v, want single DUB with Leinster region", got)
	}
	if _, err := tools.RunListAirports(c, "IE", "DUB"); err == nil {
		t.Error("expected error for code+country together")
	}
}

func TestExploreHandlerWithRouteDetails(t *testing.T) {
	c := exploreClient(t)
	res, err := tools.RunExplore(c, tools.ExploreArgs{Origin: "DUB", WithRouteDetails: true})
	if err != nil {
		t.Fatalf("explore with metadata: %v", err)
	}
	var stn ryanair.Destination
	for _, d := range res.Destinations {
		if d.IataCode == "STN" {
			stn = d
		}
	}
	if stn.IataCode != "STN" || stn.Operator != "FR" || !stn.Recent {
		t.Errorf("STN should be enriched with route metadata, got %+v", stn)
	}
}

func dest(iata, country, countryName, region, regionName string) ryanair.Destination {
	return ryanair.Destination{Airport: ryanair.Airport{
		IataCode:    iata,
		CountryCode: country,
		CountryName: countryName,
		RegionCode:  region,
		RegionName:  regionName,
	}}
}

func TestGroupDestinationsByCountry(t *testing.T) {
	dests := []ryanair.Destination{
		dest("AGP", "es", "Spain", "COSTA_DE_SOL", "Costa del Sol"),
		dest("STN", "gb", "United Kingdom", "ENGLAND", "England"),
		dest("BCN", "es", "Spain", "CATALONIA", "Catalonia"),
	}
	groups, err := tools.GroupDestinations(dests, "country")
	if err != nil {
		t.Fatalf("GroupDestinations: %v", err)
	}
	// First-seen order: Spain, then UK.
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	if groups[0].Key != "es" || groups[0].Name != "Spain" {
		t.Errorf("group[0] = %+v, want es/Spain", groups[0])
	}
	if len(groups[0].Destinations) != 2 {
		t.Errorf("Spain destinations = %d, want 2", len(groups[0].Destinations))
	}
	// Members preserve first-seen order: AGP before BCN.
	if groups[0].Destinations[0].IataCode != "AGP" || groups[0].Destinations[1].IataCode != "BCN" {
		t.Errorf("Spain member order = [%s %s], want [AGP BCN]",
			groups[0].Destinations[0].IataCode, groups[0].Destinations[1].IataCode)
	}
	if groups[1].Key != "gb" {
		t.Errorf("group[1] key = %q, want gb", groups[1].Key)
	}
}

func TestGroupDestinationsByRegion(t *testing.T) {
	dests := []ryanair.Destination{
		dest("STN", "gb", "United Kingdom", "ENGLAND", "England"),
		dest("BCN", "es", "Spain", "CATALONIA", "Catalonia"),
	}
	groups, err := tools.GroupDestinations(dests, "region")
	if err != nil {
		t.Fatalf("GroupDestinations: %v", err)
	}
	if len(groups) != 2 || groups[0].Name != "England" {
		t.Errorf("region groups = %+v", groups)
	}
}

func TestGroupDestinationsInvalid(t *testing.T) {
	if _, err := tools.GroupDestinations(nil, "city"); err == nil {
		t.Fatal("expected error for invalid group_by")
	}
}

// The handler-wiring tests below exercise the thin tool handlers end-to-end and
// assert each maps its input fields onto the right request — a swapped or
// mistyped field (e.g. origin/destination) would otherwise compile and pass.

func TestSearchOneWayHandlerWiring(t *testing.T) {
	rec := &capturedRequest{}
	c := fixtureClient(t, rec)
	flights, err := tools.RunSearchOneWay(c, tools.OneWayArgs{
		Origin: "DUB", Destination: "STN", DateFrom: "2026-07-01", DateTo: "2026-07-31",
		MaxPrice: 100, Currency: "EUR",
	})
	if err != nil {
		t.Fatalf("search_one_way: %v", err)
	}
	if len(flights) == 0 {
		t.Fatal("expected flights")
	}
	for field, want := range map[string]string{
		"departureAirportIataCode": "DUB",
		"arrivalAirportIataCode":   "STN",
		"priceValueTo":             "100",
		"currency":                 "EUR",
	} {
		if got := rec.query.Get(field); got != want {
			t.Errorf("query[%s] = %q, want %q (field mapping wrong)", field, got, want)
		}
	}
}

func TestSearchReturnHandlerWiring(t *testing.T) {
	rec := &capturedRequest{}
	c := fixtureClient(t, rec)
	trips, err := tools.RunSearchReturn(c, tools.ReturnArgs{
		Origin: "DUB", Destination: "STN", DateFrom: "2026-07-01", DateTo: "2026-07-15",
		ReturnFrom: "2026-07-20", ReturnTo: "2026-07-31", MinTripDays: 3, MaxTripDays: 7,
	})
	if err != nil {
		t.Fatalf("search_return: %v", err)
	}
	if len(trips) == 0 {
		t.Fatal("expected trips")
	}
	for field, want := range map[string]string{
		"departureAirportIataCode": "DUB",
		"arrivalAirportIataCode":   "STN",
		"inboundDepartureDateFrom": "2026-07-20",
		"durationFrom":             "3",
		"durationTo":               "7",
	} {
		if got := rec.query.Get(field); got != want {
			t.Errorf("query[%s] = %q, want %q (field mapping wrong)", field, got, want)
		}
	}
}

func TestCheapestPerDayHandlerWiring(t *testing.T) {
	rec := &capturedRequest{}
	c := fixtureClient(t, rec)
	days, err := tools.RunCheapestPerDay(c, "DUB", "STN", "2026-07-01", "EUR")
	if err != nil {
		t.Fatalf("cheapest_per_day: %v", err)
	}
	if len(days) == 0 {
		t.Fatal("expected daily fares")
	}
	if !strings.HasSuffix(rec.path, "/oneWayFares/DUB/STN/cheapestPerDay") {
		t.Errorf("path = %q, want origin/dest DUB/STN in route", rec.path)
	}
	if got := rec.query.Get("outboundMonthOfDate"); got != "2026-07-01" {
		t.Errorf("outboundMonthOfDate = %q, want 2026-07-01", got)
	}
}

func TestGetSchedulesHandlerWiring(t *testing.T) {
	rec := &capturedRequest{}
	c := fixtureClient(t, rec)
	flights, err := tools.RunGetSchedules(c, "DUB", "STN", 2026, 7)
	if err != nil {
		t.Fatalf("get_schedules: %v", err)
	}
	if len(flights) == 0 {
		t.Fatal("expected timetable flights")
	}
	if !strings.HasSuffix(rec.path, "/schedules/DUB/STN/years/2026/months/7") {
		t.Errorf("path = %q, want DUB/STN/years/2026/months/7", rec.path)
	}
}

func TestListAirportsHandlerWiring(t *testing.T) {
	c := exploreClient(t)
	all, err := tools.RunListAirports(c, "", "")
	if err != nil {
		t.Fatalf("list_airports: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("airports = %d, want 4", len(all))
	}
	ie, err := tools.RunListAirports(c, "IE", "")
	if err != nil {
		t.Fatalf("list_airports(IE): %v", err)
	}
	if len(ie) != 1 || ie[0].IataCode != "DUB" {
		t.Errorf("IE airports = %+v, want [DUB] (country filter mapping)", ie)
	}
}
