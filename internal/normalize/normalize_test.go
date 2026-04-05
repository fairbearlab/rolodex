package normalize

import (
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Robert", "robert"},
		{"  Robert  ", "robert"},
		{"Dr. Smith", "smith"},
		{"Smith Jr.", "smith"},
		{"Smith III", "smith"},
		{"UPPERCASE", "uppercase"},
		{"María", "maria"},        // accent stripped
		{"José", "jose"},          // accent stripped
		{"François", "francois"},  // cedilla stripped
		{"Müller", "muller"},      // umlaut stripped
		{"  Multiple   Spaces  ", "multiple spaces"},
		{"", ""},
		{"Prof. Dr. Jane Smith Esq.", "jane smith"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Name(tt.input)
			if got != tt.want {
				t.Errorf("Name(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"+1 (555) 123-4567", "5551234567"},
		{"15551234567", "5551234567"},     // strip leading 1
		{"555-123-4567", "5551234567"},
		{"(555) 123 4567", "5551234567"},
		{"5551234567", "5551234567"},
		{"+44 20 7946 0958", "442079460958"}, // non-US keeps all digits
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Phone(tt.input)
			if got != tt.want {
				t.Errorf("Phone(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Bob@Gmail.com", "bob@gmail.com"},
		{"  alice@example.com  ", "alice@example.com"},
		{"USER@DOMAIN.COM", "user@domain.com"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Email(tt.input)
			if got != tt.want {
				t.Errorf("Email(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeContact(t *testing.T) {
	c := model.ParsedContact{
		FamilyName: "Smith",
		GivenName:  "Robert",
		Emails: []model.Email{
			{Address: "Bob@Gmail.com", Type: "HOME"},
			{Address: "Bob@Gmail.com", Type: "WORK"}, // duplicate
		},
		Phones: []model.Phone{
			{Number: "+1 (555) 123-4567", Type: "CELL"},
			{Number: "15551234567", Type: "HOME"}, // same number, different format
		},
	}

	nc := Contact(c)
	if nc.NormalizedFamilyName != "smith" {
		t.Errorf("family = %q", nc.NormalizedFamilyName)
	}
	if nc.NormalizedGivenName != "robert" {
		t.Errorf("given = %q", nc.NormalizedGivenName)
	}
	if len(nc.NormalizedEmails) != 1 {
		t.Errorf("expected 1 unique email, got %d: %v", len(nc.NormalizedEmails), nc.NormalizedEmails)
	}
	if len(nc.NormalizedPhones) != 1 {
		t.Errorf("expected 1 unique phone, got %d: %v", len(nc.NormalizedPhones), nc.NormalizedPhones)
	}
}
