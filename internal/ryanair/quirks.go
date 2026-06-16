// Package ryanair is a typed client for Ryanair's undocumented but publicly
// reachable read APIs (fare search, price calendars, timetables, and network
// data). It is the only package that knows Ryanair's wire format and quirks.
package ryanair

import (
	"fmt"
	"strings"
	"time"
)

// defaultTimeFrom and defaultTimeTo are the departure-time window bounds the
// fare endpoints require even when the caller does not care about time of day.
const (
	defaultTimeFrom = "00:00"
	defaultTimeTo   = "23:59"
)

// normCountry lowercases an ISO-3166 alpha-2 country code. The fare API
// silently returns zero fares for an uppercase arrivalCountryCode, so callers
// must always pass it lowercase.
func normCountry(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

// normIATA uppercases and trims an IATA airport code.
func normIATA(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// timeOr returns the supplied HH:MM value, or fallback when it is empty.
func timeOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// validIATA reports whether code is a plausible 3-letter IATA code.
func validIATA(code string) bool {
	if len(code) != 3 {
		return false
	}
	for _, r := range code {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// validateDateRange ensures from is not after to. Both are inclusive ISO dates.
func validateDateRange(label, from, to string) error {
	f, err := time.Parse(dateLayout, from)
	if err != nil {
		return fmt.Errorf("%s from date %q: %w", label, from, err)
	}
	t, err := time.Parse(dateLayout, to)
	if err != nil {
		return fmt.Errorf("%s to date %q: %w", label, to, err)
	}
	if f.After(t) {
		return fmt.Errorf("%s from date %q is after to date %q", label, from, to)
	}
	return nil
}

// dateLayout is the ISO date format Ryanair expects for date-window params.
const dateLayout = "2006-01-02"
