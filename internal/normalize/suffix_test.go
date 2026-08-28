package normalize

import (
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

func TestGenerationalSuffixTrailingTokenOnly(t *testing.T) {
	tests := []struct {
		name string
		in   model.ParsedContact
		want string
	}{
		// The dedicated N suffix component.
		{"suffix component jr", model.ParsedContact{Suffix: "Jr."}, "jr"},
		{"suffix component sr", model.ParsedContact{Suffix: "Sr"}, "sr"},
		{"suffix component roman", model.ParsedContact{Suffix: "III"}, "iii"},
		{"suffix component junior spelled out", model.ParsedContact{Suffix: "Junior"}, "jr"},
		{"suffix component v", model.ParsedContact{Suffix: "V"}, "v"},
		{"credential is not generational", model.ParsedContact{Suffix: "MD"}, ""},
		{"credential beside generational", model.ParsedContact{Suffix: "MD, Jr."}, "jr"},

		// Folded into a name field: only a trailing token, and only when a
		// name precedes it.
		{"folded into family name", model.ParsedContact{FamilyName: "Smith Jr."}, "jr"},
		{"folded into given name", model.ParsedContact{GivenName: "John Jr"}, "jr"},
		{"folded roman numeral", model.ParsedContact{FamilyName: "Vanderbilt IV"}, "iv"},

		// Regression: a lone token IS the name, not a folded suffix. A given
		// name of "V" previously normalized to suffix "v", which then blocked
		// every match against the same person recorded with a full name.
		{"lone V given name is not a suffix", model.ParsedContact{GivenName: "V", FamilyName: "Smith"}, ""},
		{"lone V family name is not a suffix", model.ParsedContact{GivenName: "Victor", FamilyName: "V"}, ""},
		{"lone roman numeral given name", model.ParsedContact{GivenName: "Ii"}, ""},
		{"lone jr given name", model.ParsedContact{GivenName: "Jr"}, ""},

		// A leading or interior match is not a generational suffix either.
		{"leading token not a suffix", model.ParsedContact{FamilyName: "Jr Smith"}, ""},

		{"no suffix anywhere", model.ParsedContact{GivenName: "John", FamilyName: "Smith"}, ""},
		{"empty contact", model.ParsedContact{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GenerationalSuffix(tt.in); got != tt.want {
				t.Errorf("GenerationalSuffix(%+v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestGenerationalSuffixInitialMatchesFullName is the end-to-end shape of the
// regression: "V Smith" and "Victor Smith" must not be pushed apart by a
// phantom generational suffix. They still differ by given name, so this
// asserts only that the suffix does not become the reason.
func TestGenerationalSuffixInitialMatchesFullName(t *testing.T) {
	initial := Contact(model.ParsedContact{GivenName: "V", FamilyName: "Smith"})
	full := Contact(model.ParsedContact{GivenName: "Victor", FamilyName: "Smith"})

	if initial.NormalizedSuffix != "" {
		t.Errorf("initial-only given name produced suffix %q, want %q", initial.NormalizedSuffix, "")
	}
	if initial.NormalizedSuffix != full.NormalizedSuffix {
		t.Errorf("suffix mismatch blocks the match: %q vs %q",
			initial.NormalizedSuffix, full.NormalizedSuffix)
	}
}

// TestGenerationalSuffixStillSeparatesGenerations guards the property the
// suffix rule exists for: a father and son on one household phone.
func TestGenerationalSuffixStillSeparatesGenerations(t *testing.T) {
	junior := Contact(model.ParsedContact{GivenName: "John", FamilyName: "Smith", Suffix: "Jr."})
	senior := Contact(model.ParsedContact{GivenName: "John", FamilyName: "Smith", Suffix: "Sr."})
	plain := Contact(model.ParsedContact{GivenName: "John", FamilyName: "Smith"})

	if junior.NormalizedSuffix != "jr" {
		t.Errorf("junior suffix = %q, want %q", junior.NormalizedSuffix, "jr")
	}
	if senior.NormalizedSuffix != "sr" {
		t.Errorf("senior suffix = %q, want %q", senior.NormalizedSuffix, "sr")
	}
	if junior.NormalizedSuffix == senior.NormalizedSuffix {
		t.Error("Jr. and Sr. must not share a suffix")
	}
	if junior.NormalizedSuffix == plain.NormalizedSuffix {
		t.Error("Jr. and an unsuffixed contact must not share a suffix")
	}
}
