package ryanair_test

import (
	"testing"

	"github.com/adambenhassen/ryanair-mcp/internal/ryanair"
)

func TestNormIATA(t *testing.T) {
	cases := []struct{ in, want string }{
		{"dub", "DUB"},
		{" stn ", "STN"},
		{"Bcn", "BCN"},
		{"", ""},
	}
	for _, c := range cases {
		if got := ryanair.NormIATA(c.in); got != c.want {
			t.Errorf("NormIATA(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidIATA(t *testing.T) {
	valid := []string{"DUB", "STN", "BCN"}
	for _, c := range valid {
		if !ryanair.ValidIATA(c) {
			t.Errorf("ValidIATA(%q) = false, want true", c)
		}
	}
	invalid := []string{"", "DU", "DUBB", "du1", "D B", "123"}
	for _, c := range invalid {
		if ryanair.ValidIATA(c) {
			t.Errorf("ValidIATA(%q) = true, want false", c)
		}
	}
}

func TestNormCountry(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ES", "es"},
		{" Gb ", "gb"},
		{"ie", "ie"},
	}
	for _, c := range cases {
		if got := ryanair.NormCountry(c.in); got != c.want {
			t.Errorf("NormCountry(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormRoute(t *testing.T) {
	o, d, err := ryanair.NormRoute("dub", " stn ")
	if err != nil {
		t.Fatalf("NormRoute valid: %v", err)
	}
	if o != "DUB" || d != "STN" {
		t.Errorf("NormRoute = %q-%q, want DUB-STN", o, d)
	}
	if _, _, err := ryanair.NormRoute("XX", "STN"); err == nil {
		t.Error("expected error for invalid origin")
	}
	if _, _, err := ryanair.NormRoute("DUB", "ZZZZ"); err == nil {
		t.Error("expected error for invalid destination")
	}
}

func TestTimeOr(t *testing.T) {
	if got := ryanair.TimeOr("", "00:00"); got != "00:00" {
		t.Errorf("TimeOr empty = %q, want fallback 00:00", got)
	}
	if got := ryanair.TimeOr("09:30", "00:00"); got != "09:30" {
		t.Errorf("TimeOr value = %q, want 09:30", got)
	}
	if got := ryanair.TimeOr("   ", "23:59"); got != "23:59" {
		t.Errorf("TimeOr blank = %q, want fallback 23:59", got)
	}
}

func TestValidateDateRange(t *testing.T) {
	if err := ryanair.ValidateDateRange("outbound", "2026-07-01", "2026-07-15"); err != nil {
		t.Errorf("valid range: %v", err)
	}
	if err := ryanair.ValidateDateRange("outbound", "2026-07-15", "2026-07-01"); err == nil {
		t.Error("expected error for reversed range")
	}
	if err := ryanair.ValidateDateRange("outbound", "not-a-date", "2026-07-15"); err == nil {
		t.Error("expected error for malformed from date")
	}
	if err := ryanair.ValidateDateRange("outbound", "2026-07-01", "nope"); err == nil {
		t.Error("expected error for malformed to date")
	}
}
