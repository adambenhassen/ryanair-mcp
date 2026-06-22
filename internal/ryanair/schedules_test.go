package ryanair_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/adambenhassen/ryanair-mcp/internal/ryanair"
)

func TestSchedulesValidation(t *testing.T) {
	client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ctx := context.Background()
	cases := []struct {
		name         string
		origin, dest string
		year, month  int
		wantErr      string // substring proving validation rejected it (not a network decode error)
	}{
		{"bad month low", "DUB", "STN", 2026, 0, "invalid month"},
		{"bad month high", "DUB", "STN", 2026, 13, "invalid month"},
		{"bad year low", "DUB", "STN", 1999, 7, "invalid year"},
		{"bad year high", "DUB", "STN", 2101, 7, "invalid year"},
		{"bad origin", "XX", "STN", 2026, 7, "invalid route"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.Schedules(ctx, ryanair.ScheduleParams{Origin: tc.origin, Destination: tc.dest, Year: tc.year, Month: tc.month})
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q (a validation error, not a network failure)", err, tc.wantErr)
			}
		})
	}
}

func TestSchedules(t *testing.T) {
	fs := &fakeServer{}
	client := newClient(t, routeFixtures(t, fs, map[string]string{
		"/timtbl/3/schedules": "schedules.json",
	}))

	flights, err := client.Schedules(context.Background(), ryanair.ScheduleParams{Origin: "DUB", Destination: "STN", Year: 2026, Month: 7})
	if err != nil {
		t.Fatalf("Schedules: %v", err)
	}
	if len(flights) == 0 {
		t.Fatal("expected timetable flights")
	}
	if !strings.HasPrefix(flights[0].FlightNumber, "FR") {
		t.Errorf("flight number = %q, want FR prefix", flights[0].FlightNumber)
	}
}
