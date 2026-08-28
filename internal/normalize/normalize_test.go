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
		{"María", "maria"},       // accent stripped
		{"José", "jose"},         // accent stripped
		{"François", "francois"}, // cedilla stripped
		{"Müller", "muller"},     // umlaut stripped
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
		{"15551234567", "5551234567"}, // strip leading 1
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

func TestNormalizeOrg(t *testing.T) {
	cases := map[string]string{
		"Kunkels Drive-In;":             "Kunkels Drive-In",
		"Kunkels Drive-In":              "Kunkels Drive-In",
		"Independent Insurance Agent;":  "Independent Insurance Agent",
		";FRIEND":                       ";FRIEND", // department, no company: position kept
		"Acme;Sales":                    "Acme;Sales",
		"Acme;;Team":                    "Acme;;Team",
		"Acme; Sales ;":                 "Acme;Sales",
		"  Acme  ":                      "Acme",
		";":                             "",
		"":                              "",
		"https://example.com:8080/team": "https://example.com:8080/team",
	}
	for in, want := range cases {
		if got := Org(in); got != want {
			t.Errorf("Org(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeBirthday(t *testing.T) {
	cases := map[string]string{
		"1989-10-22":           "1989-10-22",
		"19891022":             "1989-10-22",
		"--1022":               "--10-22",
		"--10-22":              "--10-22",
		"1989-10-22T00:00:00Z": "1989-10-22",
		"1604-10-26":           "--10-26", // Apple placeholder year
		"16041026":             "--10-26",
		" 19891022 ":           "1989-10-22",
		"October 22":           "--10-22",
		"October 22, 1989":     "1989-10-22",
		"Oct 22 1989":          "1989-10-22",
		"22 October 1989":      "1989-10-22",
		"22nd Oct 1989":        "1989-10-22",
		"1989/10/22":           "1989-10-22",
		"10/22/1989":           "1989-10-22", // US, month first
		"22/10/1989":           "1989-10-22", // day first, unambiguous
		"3/4/1989":             "1989-03-04", // ambiguous: read month first
		"22.10.1989":           "1989-10-22", // European, day first
		"10.22.1989":           "1989-10-22", // dotted but month first
		"10/22":                "--10-22",
		"1989":                 "1989",       // bare year: unreadable, untouched
		"1989-10":              "1989-10",    // partial ISO: unreadable
		"10/22/89":             "10/22/89",   // two-digit year: unreadable
		"1989-13-45":           "1989-13-45", // out of range: unreadable
		"0000-00-00":           "0000-00-00", // placeholder: unreadable
		"Smarch 3":             "Smarch 3",   // not a month
		"unknown":              "unknown",
		"":                     "",
	}
	for in, want := range cases {
		if got := Birthday(in); got != want {
			t.Errorf("Birthday(%q) = %q, want %q", in, got, want)
		}
	}
	if got := BirthdayWithoutYear("1604-10-26"); got != "--10-26" {
		t.Errorf("BirthdayWithoutYear = %q, want --10-26", got)
	}
	if got := BirthdayWithoutYear("--10-26"); got != "--10-26" {
		t.Errorf("BirthdayWithoutYear(no year) = %q, want unchanged", got)
	}
}

func TestGenerationalSuffix(t *testing.T) {
	cases := []struct {
		c    model.ParsedContact
		want string
	}{
		{model.ParsedContact{Suffix: "Jr."}, "jr"},
		{model.ParsedContact{Suffix: "Junior"}, "jr"},
		{model.ParsedContact{FamilyName: "Smith Sr."}, "sr"},
		{model.ParsedContact{GivenName: "John III"}, "iii"},
		{model.ParsedContact{Suffix: "MD"}, ""},
		{model.ParsedContact{Suffix: "PhD", FamilyName: "Smith"}, ""},
		{model.ParsedContact{}, ""},
	}
	for _, tc := range cases {
		if got := GenerationalSuffix(tc.c); got != tc.want {
			t.Errorf("GenerationalSuffix(%+v) = %q, want %q", tc.c, got, tc.want)
		}
	}
}
