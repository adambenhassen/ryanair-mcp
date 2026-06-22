package ryanair_test

import (
	"testing"

	"github.com/adambenhassen/ryanair-mcp/internal/ryanair"
)

func TestCheapestWeekendTripMatcher(t *testing.T) {
	price := func(v float64) *float64 { return &v }
	fare := func(day string, p *float64) ryanair.DailyFare {
		return ryanair.DailyFare{Day: day, Price: p, Currency: "EUR"}
	}
	cases := []struct {
		name            string
		outbounds       []ryanair.DailyFare
		inbounds        []ryanair.DailyFare
		length          int
		wantNil         bool
		wantOut, wantIn string
		wantTotal       float64
	}{
		{
			name: "picks cheapest priced Friday pair, ignoring non-Fridays (Thu and Sat)",
			outbounds: []ryanair.DailyFare{
				fare("2026-07-03", price(20)), // Fri
				fare("2026-07-10", price(15)), // Fri (cheaper)
				fare("2026-07-04", price(1)),  // Sat - must be ignored despite lowest price
				fare("2026-07-02", price(1)),  // Thu - must be ignored (pins Friday, not just "skip Sat")
			},
			inbounds: []ryanair.DailyFare{
				fare("2026-07-05", price(25)), // Sun -> pairs 07-03
				fare("2026-07-12", price(18)), // Sun -> pairs 07-10
				fare("2026-07-06", price(1)),  // Mon -> +2 from 07-04 (a Sat, never matched)
				fare("2026-07-04", price(1)),  // Sat -> +2 from 07-02 (a Thu, never matched)
			},
			length: 2, wantOut: "2026-07-10", wantIn: "2026-07-12", wantTotal: 33,
		},
		{
			name: "Fri->Mon (weekend_length 3) offset",
			outbounds: []ryanair.DailyFare{
				fare("2026-07-03", price(10)), // Fri
			},
			inbounds: []ryanair.DailyFare{
				fare("2026-07-05", price(99)), // Sun (+2) - wrong offset, must not match
				fare("2026-07-06", price(5)),  // Mon (+3) - the length-3 match
			},
			length: 3, wantOut: "2026-07-03", wantIn: "2026-07-06", wantTotal: 15,
		},
		{
			name: "skips nil-priced Friday in favor of a priced one",
			outbounds: []ryanair.DailyFare{
				fare("2026-07-10", nil),       // Fri, no price -> skipped
				fare("2026-07-03", price(40)), // Fri, priced
			},
			inbounds: []ryanair.DailyFare{
				fare("2026-07-05", price(10)), // pairs 07-03
				fare("2026-07-12", price(1)),  // would pair 07-10, but that outbound is unpriced
			},
			length: 2, wantOut: "2026-07-03", wantIn: "2026-07-05", wantTotal: 50,
		},
		{
			name: "skips Friday whose matching inbound is nil-priced",
			outbounds: []ryanair.DailyFare{
				fare("2026-07-10", price(1)),  // Fri (cheapest) but its inbound is unpriced
				fare("2026-07-03", price(40)), // Fri, fully priced pair
			},
			inbounds: []ryanair.DailyFare{
				fare("2026-07-12", nil),       // Sun -> would pair 07-10, but unpriced -> skipped
				fare("2026-07-05", price(10)), // Sun -> pairs 07-03
			},
			length: 2, wantOut: "2026-07-03", wantIn: "2026-07-05", wantTotal: 50,
		},
		{
			name:      "Friday with no matching inbound -> nil",
			outbounds: []ryanair.DailyFare{fare("2026-07-03", price(20))}, // Fri
			inbounds:  []ryanair.DailyFare{fare("2026-07-04", price(20))}, // not +2 days
			length:    2, wantNil: true,
		},
		{
			name:      "no Friday outbound -> nil",
			outbounds: []ryanair.DailyFare{fare("2026-07-04", price(10))}, // Sat
			inbounds:  []ryanair.DailyFare{fare("2026-07-06", price(10))},
			length:    2, wantNil: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trip, err := ryanair.CheapestWeekendTrip(tc.outbounds, tc.inbounds, tc.length)
			if err != nil {
				t.Fatalf("matcher: %v", err)
			}
			if tc.wantNil {
				if trip != nil {
					t.Fatalf("want nil trip, got %+v", trip)
				}
				return
			}
			if trip == nil {
				t.Fatal("want a trip, got nil")
			}
			if trip.Outbound.Day != tc.wantOut || trip.Inbound.Day != tc.wantIn {
				t.Errorf("pair = %s->%s, want %s->%s", trip.Outbound.Day, trip.Inbound.Day, tc.wantOut, tc.wantIn)
			}
			if trip.TotalPrice != tc.wantTotal {
				t.Errorf("total = %v, want %v", trip.TotalPrice, tc.wantTotal)
			}
			if trip.Currency != "EUR" {
				t.Errorf("currency = %q, want EUR", trip.Currency)
			}
		})
	}
}

func TestCheapestWeekendTripMalformedDay(t *testing.T) {
	v := 10.0
	out := []ryanair.DailyFare{{Day: "not-a-date", Price: &v, Currency: "EUR"}}
	if _, err := ryanair.CheapestWeekendTrip(out, nil, 2); err == nil {
		t.Fatal("expected error for malformed outbound day")
	}
}

// TestRoundTripNotTruncated locks in that roundTripFares is exempt from the
// oneWayFares truncation guard: the live endpoint returns nextPage=1 on every
// (even complete) response, so the guard must NOT fire there.
