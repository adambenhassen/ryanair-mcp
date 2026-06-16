package tools_test

import (
	"testing"

	"github.com/adambenhassen/ryanair-mcp/internal/ryanair"
	"github.com/adambenhassen/ryanair-mcp/internal/tools"
)

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
