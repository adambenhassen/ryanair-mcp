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
