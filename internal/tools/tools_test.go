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

// exploreClient builds a ryanair.Client backed by the ryanair package fixtures.
func exploreClient(t *testing.T) *ryanair.Client {
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
		switch {
		case r.URL.Path == "/":
			w.WriteHeader(http.StatusOK)
		case strings.HasPrefix(r.URL.Path, "/api/views/locate/3/aggregate/all/en"):
			serve(w, "network.json")
		case strings.HasSuffix(r.URL.Path, "/api/views/locate/5/airports/en/active"):
			serve(w, "active_airports.json")
		case strings.HasPrefix(r.URL.Path, "/api/views/locate/5/airports/en/"):
			serve(w, "airport_info.json")
		case strings.HasPrefix(r.URL.Path, "/api/views/locate/searchWidget/routes/en/airport/"):
			serve(w, "airport_destinations.json")
		case strings.HasPrefix(r.URL.Path, "/api/geoloc/v5/nearbyAirports"):
			serve(w, "nearby_airports.json")
		case strings.HasPrefix(r.URL.Path, "/api/geoloc/v5/defaultAirport"):
			serve(w, "default_airport.json")
		case strings.HasSuffix(r.URL.Path, "/availabilities"):
			serve(w, "availabilities.json")
		case strings.Contains(r.URL.Path, "/roundTripFares/") && strings.HasSuffix(r.URL.Path, "/cheapestPerDay"):
			serve(w, "return_cheapest_per_day.json")
		case strings.HasPrefix(r.URL.Path, "/farfnd/v4/oneWayFares"):
			serve(w, "one_way_fares.json")
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

func TestAnywhereHandler(t *testing.T) {
	c := exploreClient(t)
	flights, err := tools.RunAnywhereUnder(c, "DUB", "2026-07-01", "2026-07-31", 100)
	if err != nil {
		t.Fatalf("anywhere: %v", err)
	}
	if len(flights) == 0 {
		t.Fatal("expected flights under cap")
	}
	if _, err := tools.RunAnywhereUnder(c, "DUB", "2026-07-01", "2026-07-31", 0); err == nil {
		t.Error("expected error for max_price = 0")
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

func TestActiveAirportsHandler(t *testing.T) {
	c := exploreClient(t)
	airports, err := tools.RunActiveAirports(c)
	if err != nil {
		t.Fatalf("active airports: %v", err)
	}
	if len(airports) != 2 || airports[0].IataCode != "DUB" {
		t.Errorf("airports = %+v, want 2 starting DUB", airports)
	}
}

func TestAirportInfoHandler(t *testing.T) {
	c := exploreClient(t)
	a, err := tools.RunAirportInfo(c, "DUB")
	if err != nil {
		t.Fatalf("airport info: %v", err)
	}
	if a.IataCode != "DUB" || a.RegionName != "Leinster" {
		t.Errorf("airport = %+v", a)
	}
}

func TestAirportDestinationsHandler(t *testing.T) {
	c := exploreClient(t)
	dests, err := tools.RunAirportDestinations(c, "DUB")
	if err != nil {
		t.Fatalf("airport destinations: %v", err)
	}
	if len(dests) != 2 || dests[0].Operator != "FR" {
		t.Errorf("dests = %+v", dests)
	}
}

func TestNearbyAirportsHandler(t *testing.T) {
	c := exploreClient(t)
	airports, err := tools.RunNearbyAirports(c, "")
	if err != nil {
		t.Fatalf("nearby: %v", err)
	}
	if len(airports) != 2 || airports[0].IataCode != "STN" {
		t.Errorf("airports = %+v", airports)
	}
}

func TestDefaultAirportHandler(t *testing.T) {
	c := exploreClient(t)
	a, err := tools.RunDefaultAirport(c)
	if err != nil {
		t.Fatalf("default airport: %v", err)
	}
	if a.IataCode != "DUB" {
		t.Errorf("airport = %+v", a)
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
