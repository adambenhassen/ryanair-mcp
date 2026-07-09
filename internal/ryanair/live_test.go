//go:build live

// These live smoke tests hit the real Ryanair endpoints to catch wire-format
// or endpoint drift that fixture-based tests cannot. They are excluded from
// normal builds and CI; run them explicitly with:
//
//	go test -tags live ./internal/ryanair/ -v
package ryanair_test

import (
	"context"
	"testing"
	"time"

	"github.com/adambenhassen/ryanair-mcp/internal/ryanair"
)

const (
	liveOrigin = "DUB"
	liveDest   = "STN"
)

func liveCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// firstOfNextMonth returns the first day of a month roughly one month out, so
// fares/schedules are within the sellable window.
func firstOfNextMonth() time.Time {
	n := time.Now().AddDate(0, 1, 0)
	return time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func TestLiveSmoke(t *testing.T) {
	client, err := ryanair.NewClient()
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	month := firstOfNextMonth()
	monthStr := month.Format("2006-01-02")
	outFrom := month.Format("2006-01-02")
	outTo := month.AddDate(0, 0, 14).Format("2006-01-02")
	retFrom := month.AddDate(0, 0, 21).Format("2006-01-02")
	retTo := month.AddDate(0, 0, 35).Format("2006-01-02")

	t.Run("OneWayFares", func(t *testing.T) {
		flights, err := client.OneWayFares(liveCtx(t), ryanair.OneWayParams{
			Origin: liveOrigin, DateFrom: outFrom, DateTo: outTo,
		})
		if err != nil {
			t.Fatalf("OneWayFares: %v", err)
		}
		t.Logf("one-way fares from %s: %d", liveOrigin, len(flights))
		if len(flights) == 0 {
			t.Fatal("expected at least one one-way fare (possible endpoint drift)")
		}
		f := flights[0]
		if f.Origin == "" || f.Destination == "" || f.Price <= 0 || f.Currency == "" {
			t.Errorf("malformed flight: %+v", f)
		}
		if f.DepartureTime.IsZero() {
			t.Error("departure time not parsed (datetime format may have changed)")
		}
	})

	t.Run("RoundTripFares", func(t *testing.T) {
		trips, err := client.RoundTripFares(liveCtx(t), ryanair.ReturnParams{
			OneWayParams: ryanair.OneWayParams{Origin: liveOrigin, DateFrom: outFrom, DateTo: outTo},
			ReturnFrom:   retFrom, ReturnTo: retTo,
		})
		if err != nil {
			t.Fatalf("RoundTripFares: %v", err)
		}
		t.Logf("return trips from %s: %d", liveOrigin, len(trips))
		if len(trips) == 0 {
			t.Fatal("expected at least one return trip (possible endpoint drift)")
		}
		if trips[0].TotalPrice <= 0 || trips[0].Inbound.FlightNumber == "" {
			t.Errorf("malformed trip: %+v", trips[0])
		}
	})

	t.Run("CheapestPerDay", func(t *testing.T) {
		days, err := client.CheapestPerDay(liveCtx(t), ryanair.CalendarParams{Origin: liveOrigin, Destination: liveDest, Month: monthStr})
		if err != nil {
			t.Fatalf("CheapestPerDay: %v", err)
		}
		t.Logf("calendar days for %s-%s %s: %d", liveOrigin, liveDest, monthStr, len(days))
		if len(days) == 0 {
			t.Fatal("expected calendar days (possible endpoint drift)")
		}
		if days[0].Day == "" {
			t.Error("daily fare missing day")
		}
	})

	t.Run("RouteActiveDates", func(t *testing.T) {
		dates, err := client.RouteActiveDates(liveCtx(t), liveOrigin, liveDest)
		if err != nil {
			t.Fatalf("RouteActiveDates: %v", err)
		}
		t.Logf("active dates for %s-%s: %d", liveOrigin, liveDest, len(dates))
		if len(dates) == 0 {
			t.Fatal("expected active dates (possible endpoint drift)")
		}
	})

	t.Run("CheapestReturnPerDay", func(t *testing.T) {
		res, err := client.CheapestReturnPerDay(liveCtx(t), ryanair.ReturnCalendarParams{Origin: liveOrigin, Destination: liveDest, OutboundMonth: monthStr, InboundMonth: monthStr, MinTripDays: 2, MaxTripDays: 7})
		if err != nil {
			t.Fatalf("CheapestReturnPerDay: %v", err)
		}
		t.Logf("return calendar %s-%s: out=%d in=%d", liveOrigin, liveDest, len(res.Outbound), len(res.Inbound))
		if len(res.Outbound) == 0 || len(res.Inbound) == 0 {
			t.Fatal("expected outbound and inbound days (possible endpoint drift)")
		}
	})

	t.Run("CheapestWeekend", func(t *testing.T) {
		// May legitimately be nil if no priced weekend exists; we only assert the
		// multi-month composition runs without error against the live endpoint.
		trip, err := client.CheapestWeekend(liveCtx(t), ryanair.WeekendParams{Origin: liveOrigin, Destination: liveDest, MonthsAhead: 2, WeekendLength: 2})
		if err != nil {
			t.Fatalf("CheapestWeekend: %v", err)
		}
		if trip != nil {
			t.Logf("cheapest weekend %s-%s: %s -> %s total %.2f",
				liveOrigin, liveDest, trip.Outbound.Day, trip.Inbound.Day, trip.TotalPrice)
		}
	})

	t.Run("Schedules", func(t *testing.T) {
		flights, err := client.Schedules(liveCtx(t), ryanair.ScheduleParams{Origin: liveOrigin, Destination: liveDest, Year: month.Year(), Month: int(month.Month())})
		if err != nil {
			t.Fatalf("Schedules: %v", err)
		}
		t.Logf("timetable flights for %s-%s: %d", liveOrigin, liveDest, len(flights))
		if len(flights) > 0 && flights[0].FlightNumber == "" {
			t.Error("timetable flight missing number")
		}
	})

	t.Run("ListAirports", func(t *testing.T) {
		airports, err := client.ListAirports(liveCtx(t), "")
		if err != nil {
			t.Fatalf("ListAirports: %v", err)
		}
		t.Logf("airports in network: %d", len(airports))
		if len(airports) == 0 {
			t.Fatal("expected airports (possible endpoint drift)")
		}
		if airports[0].IataCode == "" || airports[0].Name == "" {
			t.Errorf("malformed airport: %+v", airports[0])
		}
	})

	t.Run("ExploreDestinations", func(t *testing.T) {
		dests, err := client.ExploreDestinations(liveCtx(t), ryanair.ExploreParams{Origin: liveOrigin})
		if err != nil {
			t.Fatalf("ExploreDestinations: %v", err)
		}
		t.Logf("destinations from %s: %d", liveOrigin, len(dests))
		if len(dests) == 0 {
			t.Fatal("expected destinations (possible endpoint drift)")
		}
	})

	t.Run("AirportInfo", func(t *testing.T) {
		a, err := client.AirportInfo(liveCtx(t), liveOrigin)
		if err != nil {
			t.Fatalf("AirportInfo: %v", err)
		}
		if a.IataCode != liveOrigin || a.Name == "" || a.TimeZone == "" {
			t.Errorf("malformed airport info: %+v", a)
		}
	})

	t.Run("AirportDestinations", func(t *testing.T) {
		dests, err := client.AirportDestinations(liveCtx(t), liveOrigin)
		if err != nil {
			t.Fatalf("AirportDestinations: %v", err)
		}
		t.Logf("route destinations from %s: %d", liveOrigin, len(dests))
		if len(dests) == 0 {
			t.Fatal("expected route destinations (possible endpoint drift)")
		}
		if dests[0].IataCode == "" || dests[0].Operator == "" {
			t.Errorf("malformed route destination: %+v", dests[0])
		}
	})
}
