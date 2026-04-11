package review

import (
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

func TestContactDisplayNameFull(t *testing.T) {
	c := model.ParsedContact{GivenName: "Alice", FamilyName: "Smith"}
	got := contactDisplayName(c)
	if got != "Alice Smith" {
		t.Errorf("contactDisplayName = %q, want %q", got, "Alice Smith")
	}
}

func TestContactDisplayNameFormattedName(t *testing.T) {
	c := model.ParsedContact{FormattedName: "Dr. Bob Jones", GivenName: "Bob", FamilyName: "Jones"}
	got := contactDisplayName(c)
	if got != "Dr. Bob Jones" {
		t.Errorf("contactDisplayName = %q, want %q", got, "Dr. Bob Jones")
	}
}

func TestContactDisplayNameEmailFallback(t *testing.T) {
	c := model.ParsedContact{
		Emails: []model.Email{{Address: "test@example.com"}},
	}
	got := contactDisplayName(c)
	if got != "test@example.com" {
		t.Errorf("contactDisplayName = %q, want %q", got, "test@example.com")
	}
}

func TestContactDisplayNamePhoneFallback(t *testing.T) {
	c := model.ParsedContact{
		Phones: []model.Phone{{Number: "+15551234567"}},
	}
	got := contactDisplayName(c)
	if got != "+15551234567" {
		t.Errorf("contactDisplayName = %q, want %q", got, "+15551234567")
	}
}

func TestContactDisplayNameUnknown(t *testing.T) {
	c := model.ParsedContact{}
	got := contactDisplayName(c)
	if got != "(unknown)" {
		t.Errorf("contactDisplayName = %q, want %q", got, "(unknown)")
	}
}

func TestFormatAddress(t *testing.T) {
	a := model.Address{Street: "123 Main St", City: "Springfield", Region: "IL"}
	got := formatAddress(a)
	if got != "123 Main St, Springfield, IL" {
		t.Errorf("formatAddress = %q, want %q", got, "123 Main St, Springfield, IL")
	}
}

func TestFormatAddressEmpty(t *testing.T) {
	got := formatAddress(model.Address{})
	if got != "(empty)" {
		t.Errorf("formatAddress = %q, want %q", got, "(empty)")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate short = %q, want %q", got, "short")
	}
	if got := truncate("this is a very long string", 10); got != "this is..." {
		t.Errorf("truncate long = %q, want %q", got, "this is...")
	}
}
