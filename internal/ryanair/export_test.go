package ryanair

import (
	"encoding/json"
	"time"
)

// SetBaseBackoff overrides the retry backoff base and returns a function that
// restores the original, so retry tests exercise the backoff path without
// sleeping for real.
func SetBaseBackoff(d time.Duration) (restore func()) {
	prev := baseBackoff
	baseBackoff = d
	return func() { baseBackoff = prev }
}

// Exported for white-box testing of the pure weekend-matching helper, which
// carries the Friday-matching and cheapest-pair selection logic.
var CheapestWeekendTrip = cheapestWeekendTrip

// Exported for co-located unit testing of the wire-quirk helpers from the
// external test package.
var (
	NormIATA          = normIATA
	ValidIATA         = validIATA
	NormCountry       = normCountry
	NormRoute         = normRoute
	TimeOr            = timeOr
	ValidateDateRange = validateDateRange
)

// DecodeWireAirport unmarshals a views/locate airport JSON object and runs the
// wire->domain conversion, exposing toAirport for co-located testing.
func DecodeWireAirport(data []byte) (Airport, error) {
	var w wireLocAirport
	if err := json.Unmarshal(data, &w); err != nil {
		return Airport{}, err
	}
	return w.toAirport(), nil
}
