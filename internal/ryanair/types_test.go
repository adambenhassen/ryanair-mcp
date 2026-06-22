package ryanair_test

import (
	"testing"

	"github.com/adambenhassen/ryanair-mcp/internal/ryanair"
)

// TestDecodeWireAirport verifies the wire->domain mapping flattens the nested
// city/region/country/coordinates shape onto the flat Airport struct.
func TestDecodeWireAirport(t *testing.T) {
	const data = `{
		"code": "DUB",
		"name": "Dublin",
		"aliases": ["Dublin Intl"],
		"base": true,
		"timeZone": "Europe/Dublin",
		"city": {"code": "DUBLIN", "name": "Dublin"},
		"region": {"code": "LEINSTER", "name": "Leinster"},
		"country": {"code": "ie", "name": "Ireland", "currency": "EUR"},
		"coordinates": {"latitude": 53.42, "longitude": -6.27}
	}`
	a, err := ryanair.DecodeWireAirport([]byte(data))
	if err != nil {
		t.Fatalf("DecodeWireAirport: %v", err)
	}
	want := ryanair.Airport{
		IataCode:     "DUB",
		Name:         "Dublin",
		CityCode:     "DUBLIN",
		CityName:     "Dublin",
		CountryCode:  "ie",
		CountryName:  "Ireland",
		RegionCode:   "LEINSTER",
		RegionName:   "Leinster",
		CurrencyCode: "EUR",
		TimeZone:     "Europe/Dublin",
		Aliases:      []string{"Dublin Intl"},
		Latitude:     53.42,
		Longitude:    -6.27,
		Base:         true,
	}
	if a.IataCode != want.IataCode || a.Name != want.Name || a.CityCode != want.CityCode ||
		a.CityName != want.CityName || a.CountryCode != want.CountryCode || a.CountryName != want.CountryName ||
		a.RegionCode != want.RegionCode || a.RegionName != want.RegionName || a.CurrencyCode != want.CurrencyCode ||
		a.TimeZone != want.TimeZone || a.Latitude != want.Latitude || a.Longitude != want.Longitude || a.Base != want.Base {
		t.Errorf("DecodeWireAirport() = %+v, want %+v", a, want)
	}
	if len(a.Aliases) != 1 || a.Aliases[0] != "Dublin Intl" {
		t.Errorf("aliases = %v, want [Dublin Intl]", a.Aliases)
	}
}

// TestDecodeWireAirportLean covers the leaner geoloc shape where region,
// timezone, base, and currency are absent: they must map to zero values.
func TestDecodeWireAirportLean(t *testing.T) {
	const data = `{
		"code": "STN",
		"name": "London Stansted",
		"city": {"code": "LONDON", "name": "London"},
		"country": {"code": "gb", "name": "United Kingdom"}
	}`
	a, err := ryanair.DecodeWireAirport([]byte(data))
	if err != nil {
		t.Fatalf("DecodeWireAirport lean: %v", err)
	}
	if a.RegionCode != "" || a.RegionName != "" || a.TimeZone != "" || a.CurrencyCode != "" || a.Base {
		t.Errorf("lean airport should leave optional fields zero, got %+v", a)
	}
	if a.IataCode != "STN" || a.CityName != "London" || a.CountryCode != "gb" {
		t.Errorf("DecodeWireAirport lean = %+v, want STN/London/gb core fields", a)
	}
}
