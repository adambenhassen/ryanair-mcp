package ryanair_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/adambenhassen/ryanair-mcp/internal/ryanair"
)

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

func TestExploreWithRouteDetails(t *testing.T) {
	client := newClient(t, routeFixtures(t, &fakeServer{}, map[string]string{
		"/api/views/locate/3/aggregate/all/en":                 "network.json",
		"/api/views/locate/searchWidget/routes/en/airport/DUB": "explore_route_metadata.json",
	}))
	dests, err := client.ExploreDestinations(context.Background(), ryanair.ExploreParams{
		Origin: "DUB", WithRouteDetails: true,
	})
	if err != nil {
		t.Fatalf("explore with metadata: %v", err)
	}
	byCode := map[string]ryanair.Destination{}
	for _, d := range dests {
		byCode[d.IataCode] = d
	}
	// STN is in both the network and the searchWidget fixture, so it is enriched.
	stn, ok := byCode["STN"]
	if !ok || stn.Operator != "FR" || !stn.Recent || len(stn.Tags) != 1 || stn.Tags[0] != "popular" {
		t.Errorf("STN metadata = %+v (ok=%v), want operator FR, recent, tags [popular]", stn, ok)
	}
	// AGA is a network destination absent from the searchWidget fixture, so its
	// metadata fields stay empty rather than being fabricated.
	if aga, ok := byCode["AGA"]; !ok || aga.Operator != "" || aga.Recent || len(aga.Tags) != 0 {
		t.Errorf("AGA should have no route metadata, got %+v (ok=%v)", aga, ok)
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
