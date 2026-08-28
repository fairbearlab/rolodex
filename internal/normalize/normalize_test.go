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
		`Acme\; Inc.`:                   `Acme\; Inc.`, // escaped ';' is not a separator
		`Acme\; Inc.;`:                  `Acme\; Inc.`,
		`Acme\; Inc.; Sales `:           `Acme\; Inc.;Sales`,
		`Acme\\;Sales`:                  `Acme\\;Sales`, // escaped backslash, then a real separator
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
		"1989":                 "1989",          // bare year: unreadable, untouched
		"1989-10":              "1989-10",       // partial ISO: unreadable
		"10/22/89":             "10/22/89",      // two-digit year: unreadable
		"1989-13-45":           "1989-13-45",    // out of range: unreadable
		"0000-00-00":           "0000-00-00",    // placeholder: unreadable
		"Smarch 3":             "Smarch 3",      // not a month (Month Day form)
		"3 Smarch 1989":        "3 Smarch 1989", // not a month (Day Month form)
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

// TestAtoi covers the parse-failure branch directly: every caller in this
// file feeds atoi a regex-captured digit group, so the error path is
// defensive rather than reachable through Birthday(), and is pinned here so
// it stays correct if a future caller ever passes it free-form text.
func TestAtoi(t *testing.T) {
	cases := map[string]int{
		"5": 5, "05": 5, "": -1, "abc": -1, "-1": -1, "12": 12,
	}
	for in, want := range cases {
		if got := atoi(in); got != want {
			t.Errorf("atoi(%q) = %d, want %d", in, got, want)
		}
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

// A birthday is confirming evidence for the exact-name rule, so a placeholder
// two contacts happen to share would auto-merge them on the name alone. Month
// and day bounds are not enough: February 31 and year zero pass 1-12 / 1-31
// and are not dates.
func TestBirthdayRejectsImpossibleDates(t *testing.T) {
	for _, raw := range []string{
		"1989-02-31", "1989-04-31", "1999-02-29", "0000-01-01", "0000-00-00",
		"1989-13-45", "--02-31",
	} {
		if got := Birthday(raw); got != raw {
			t.Errorf("Birthday(%q) = %q, want it left raw so it cannot count as evidence", raw, got)
		}
	}
	// Real dates, including a leap day and a no-year February 29, still parse.
	for _, tc := range []struct{ in, want string }{
		{"1989-10-22", "1989-10-22"},
		{"2000-02-29", "2000-02-29"},
		{"1989-02-28", "1989-02-28"},
		{"--02-29", "--02-29"},
	} {
		if got := Birthday(tc.in); got != tc.want {
			t.Errorf("Birthday(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNameStrictKeepsMarksAndFoldsCompatibility(t *testing.T) {
	cases := []struct{ in, want string }{
		// Combining marks are what this form exists to see.
		{"Nguyên", "nguyên"},
		{"Nguyễn", "nguyễn"},
		{"José", "josé"},
		// Compatibility variants are the same name written two ways.
		{"Ｓmith", "smith"},
		{"ﾀﾅｶ", "タナカ"},
		// Titles and multi-letter suffixes are still stripped; a single letter
		// is an initial, not a suffix.
		{"Dr. John", "john"},
		{"John Jr.", "john"},
		{"John V", "john v"},
		// A field that is nothing but a title keeps it rather than emptying.
		{"Dr.", "dr."},
		{"", ""},
	}
	for _, tc := range cases {
		if got := NameStrict(tc.in); got != tc.want {
			t.Errorf("NameStrict(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDisplayComponentsUnescapesForHumans(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`Acme\; Inc.`, []string{"Acme; Inc."}},
		{`Acme;Engineering`, []string{"Acme", "Engineering"}},
		{`Acme\; Inc.;R&D`, []string{"Acme; Inc.", "R&D"}},
		{`plain`, []string{"plain"}},
	}
	for _, tc := range cases {
		got := DisplayComponents(tc.in, ';')
		if len(got) != len(tc.want) {
			t.Errorf("DisplayComponents(%q) = %q, want %q", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("DisplayComponents(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

func TestUnescape(t *testing.T) {
	cases := map[string]string{
		`no escapes`:  "no escapes",
		`a\;b`:        "a;b",
		`a\\b`:        `a\b`,
		`line\nbreak`: "line\nbreak",
		`line\Nbreak`: "line\nbreak",
		`a\,b`:        "a,b",
		`trailing\`:   `trailing\`, // a dangling escape stands for itself
	}
	for in, want := range cases {
		if got := Unescape(in); got != want {
			t.Errorf("Unescape(%q) = %q, want %q", in, got, want)
		}
	}
}

// The ISO tail is a time, not "anything at all". The optional group matched
// arbitrary trailing text, so "1989-10-22 or 23" yielded a date the caller
// then trusted as confirming evidence.
func TestBirthdayRejectsTrailingText(t *testing.T) {
	for _, raw := range []string{
		"1989-10-22 or 23", "1989-10-22 maybe", "1989-10-22 approx", "1989-10-22x",
	} {
		if got := Birthday(raw); got != raw {
			t.Errorf("Birthday(%q) = %q, want it left raw", raw, got)
		}
	}
	// Real ISO datetimes still normalize to the date.
	for _, tc := range []struct{ in, want string }{
		{"1989-10-22T00:00:00", "1989-10-22"},
		{"1989-10-22T00:00:00Z", "1989-10-22"},
		{"1989-10-22T12:30", "1989-10-22"},
		{"1989-10-22 00:00:00", "1989-10-22"},
		{"1989-10-22T00:00:00.500Z", "1989-10-22"},
		{"1989-10-22T00:00:00+02:00", "1989-10-22"},
		{"1989-10-22", "1989-10-22"},
	} {
		if got := Birthday(tc.in); got != tc.want {
			t.Errorf("Birthday(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseCanonicalBirthday(t *testing.T) {
	cases := []struct {
		in       string
		year, md string
		ok       bool
	}{
		{"1989-10-22", "1989", "10-22", true},
		{"2000-02-29", "2000", "02-29", true},
		{"--02-29", "", "02-29", true}, // no year: judged against a leap year
		{"--10-22", "", "10-22", true},
		// Look canonical, are not dates.
		{"1989-02-31", "", "", false},
		{"1999-02-29", "", "", false},
		{"0000-01-01", "", "", false},
		{"0000-00-00", "", "", false},
		{"1989-13-45", "", "", false},
		{"--02-31", "", "", false},
		// Not canonical at all.
		{"1989", "", "", false},
		{"unknown", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
		year, md, ok := ParseCanonicalBirthday(tc.in)
		if ok != tc.ok || year != tc.year || md != tc.md {
			t.Errorf("ParseCanonicalBirthday(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.in, year, md, ok, tc.year, tc.md, tc.ok)
		}
	}
}

func TestPlausibleIdentifiers(t *testing.T) {
	phones := map[string]bool{
		"2125550199": true, "5551234567": true, "1234567": true,
		"0": false, "12345": false, "000000000": false, "1111111": false, "": false,
	}
	for in, want := range phones {
		if got := PlausiblePhone(in); got != want {
			t.Errorf("PlausiblePhone(%q) = %v, want %v", in, got, want)
		}
	}
	emails := map[string]bool{
		"john@example.com": true, "a@b.co": true,
		"unknown": false, "user@localhost": false, "@example.com": false,
		"user@": false, "user@domain.": false, "": false,
	}
	for in, want := range emails {
		if got := PlausibleEmail(in); got != want {
			t.Errorf("PlausibleEmail(%q) = %v, want %v", in, got, want)
		}
	}
}
