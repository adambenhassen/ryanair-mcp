package ryanair_test

import (
	"context"
	"testing"
)

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

func TestAirportDestinationsRejectsBadIATA(t *testing.T) {
	client := newClient(t, routeFixtures(t, &fakeServer{}, map[string]string{}))
	if _, err := client.AirportDestinations(context.Background(), "XX"); err == nil {
		t.Fatal("expected error for invalid origin IATA")
	}
}
