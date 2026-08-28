package scorer

import (
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/normalize"
)

// TestExpandFullNameDegenerateInput covers the len(parts)==0 guard: a name
// that is empty or pure whitespace has no first word to expand.
func TestExpandFullNameDegenerateInput(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"   ", "   "},
		{"bob", "robert"},
		{"bob smith", "robert smith"},
		{"  bob   smith  ", "robert smith"},
		{"smith", "smith"},
	}
	for _, tc := range cases {
		if got := expandFullName(tc.in); got != tc.want {
			t.Errorf("expandFullName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestSameGivenNameMultiWord pins sameGivenName as literal equality of the
// normalized given name, compound names included. It once expanded the
// first word through the nickname table, which made "bob james" identical
// to "robert james"; a nickname is similarity, not identity.
func TestSameGivenNameMultiWord(t *testing.T) {
	cases := []struct {
		name   string
		ga, gb string
		want   bool
	}{
		{"identical single", "john", "john", true},
		{"differing word counts", "mary jane", "mary", false},
		{"mismatch after first word", "mary jane", "mary ann", false},
		{"nickname first, rest equal", "bob james", "robert james", false},
		{"two diminutives, rest equal", "ted james", "ned james", false},
		{"empty vs name", "", "john", false},
		{"name vs empty", "john", "", false},
		{"both empty are equal strings", "", "", true},
		{"unrelated names", "john", "peter", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameGivenName(tc.ga, tc.gb); got != tc.want {
				t.Errorf("sameGivenName(%q, %q) = %v, want %v", tc.ga, tc.gb, got, tc.want)
			}
		})
	}
}

// TestBirthdayConflictRequiresCanonicalDates: only the forms
// normalize.Birthday produces are trusted enough to call two people distinct.
// A free-text BDAY that survived normalization must never cap a pair.
func TestBirthdayConflictRequiresCanonicalDates(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1989-06-29", "1990-06-29", true},
		{"--06-29", "--06-30", true},
		{"1989-06-29", "--06-30", true},
		{"1989-06-29", "1989-06-29", false},
		{"--06-29", "1989-06-29", false},
		{"June 29, 1989", "1990-06-29", false},
		{"1989-06-29", "unknown", false},
		{"", "1989-06-29", false},
		{"", "", false},
	}
	for _, tc := range cases {
		a := withBirthday(makeContact("a", "b", nil, nil, ""), tc.a)
		b := withBirthday(makeContact("a", "b", nil, nil, ""), tc.b)
		if got := birthdayConflict(a, b); got != tc.want {
			t.Errorf("birthdayConflict(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestBirthdayConflictCapsAtReview: a well-formed disagreement holds an
// otherwise auto-merge pair (identical name + shared phone) at review.
func TestBirthdayConflictCapsAtReview(t *testing.T) {
	mk := func(bday string) model.NormalizedContact {
		return normalize.Contact(model.ParsedContact{
			GivenName: "John", FamilyName: "Smith", Birthday: bday,
			Phones: []model.Phone{{Number: "5558675309"}},
		})
	}
	got := Score([]model.NormalizedContact{mk("1989-06-29"), mk("1962-01-04")}, [][2]int{{0, 1}})[0]
	if !got.Features.BirthdayConflict {
		t.Fatalf("BirthdayConflict = false, want true (features: %+v)", got.Features)
	}
	if got.Tier != model.TierReview {
		t.Errorf("tier = %q, want %q when two well-formed birthdays disagree", got.Tier, model.TierReview)
	}

	agree := Score([]model.NormalizedContact{mk("1989-06-29"), mk("--06-29")}, [][2]int{{0, 1}})[0]
	if agree.Features.BirthdayConflict {
		t.Error("BirthdayConflict = true for a no-year birthday matching a full one")
	}
	if agree.Tier != model.TierAutoMerge {
		t.Errorf("tier = %q, want %q when the birthdays agree", agree.Tier, model.TierAutoMerge)
	}
}

// TestScoreNamelessNeedsTwoIdentifiers pins the nameless weight table: one
// shared identifier is held below auto_merge, two reach it, and NameExact is
// never set on the nameless path.
func TestScoreNamelessNeedsTwoIdentifiers(t *testing.T) {
	mk := func(email, phone, org string) model.NormalizedContact {
		c := model.ParsedContact{Org: org}
		if email != "" {
			c.Emails = []model.Email{{Address: email}}
		}
		if phone != "" {
			c.Phones = []model.Phone{{Number: phone}}
		}
		return normalize.Contact(c)
	}
	one := Score([]model.NormalizedContact{mk("x@y.com", "", ""), mk("x@y.com", "", "")}, [][2]int{{0, 1}})[0]
	if !one.Features.Nameless {
		t.Error("Nameless = false, want true when neither contact has a given name")
	}
	if one.Features.NameExact {
		t.Error("NameExact = true on the nameless path, want false")
	}
	if one.Tier == model.TierAutoMerge {
		t.Errorf("tier = %q with a single shared identifier, want below auto_merge", one.Tier)
	}
	if one.Score >= model.ThresholdAutoMerge {
		t.Errorf("score = %.3f, want < %.2f", one.Score, model.ThresholdAutoMerge)
	}

	two := Score([]model.NormalizedContact{mk("x@y.com", "5551234567", ""), mk("x@y.com", "5551234567", "")}, [][2]int{{0, 1}})[0]
	if two.Tier != model.TierAutoMerge {
		t.Errorf("tier = %q with two shared identifiers, want %q", two.Tier, model.TierAutoMerge)
	}
}

// TestScoreEmptyPairs: no candidate pairs yields an empty, non-nil result.
func TestScoreEmptyPairs(t *testing.T) {
	got := Score([]model.NormalizedContact{makeContact("a", "b", nil, nil, "")}, nil)
	if got == nil {
		t.Fatal("Score returned nil, want an empty slice")
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

// TestSharedOrgIgnoresCaseAndSpace, and never matches on an empty org.
func TestSharedOrgIgnoresCaseAndSpace(t *testing.T) {
	mk := func(org string) model.NormalizedContact {
		return normalize.Contact(model.ParsedContact{GivenName: "A", FamilyName: "B", Org: org})
	}
	cases := []struct {
		a, b string
		want bool
	}{
		{"Acme Corp", "acme corp", true},
		{"  Acme Corp  ", "Acme Corp", true},
		{"Acme", "Globex", false},
		{"", "", false},
		{"Acme", "", false},
		{"", "Acme", false},
		{"   ", "   ", false},
	}
	for _, tc := range cases {
		if got := sharedOrg(mk(tc.a), mk(tc.b)); got != tc.want {
			t.Errorf("sharedOrg(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestCompatibleMiddlePunctuationAndUnicode is a regression test for a crash:
// a middle name that is pure punctuation ("." is a common export placeholder,
// e.g. N:Smith;John;.;;) trimmed to the empty string and was then sliced,
// panicking with "slice bounds out of range [:1] with length 0" for every
// pair that reached the middle-name gate. It also pins that a multi-byte
// initial is compared as one rune rather than one byte.
func TestCompatibleMiddlePunctuationAndUnicode(t *testing.T) {
	cases := []struct {
		name   string
		ma, mb string
		want   bool
	}{
		{"placeholder dot vs initial", ".", "j", true},
		{"initial vs placeholder dot", "j", ".", true},
		{"placeholder dots vs full middle", "..", "james", true},
		{"two placeholders", ".", "..", true},
		{"absent vs present", "", "james", true},
		{"present vs absent", "james", "", true},
		{"identical middles", "james", "james", true},
		{"initial matches full middle", "j", "james", true},
		{"initial with period matches full", "j.", "james", true},
		{"initial does not match a different middle", "b", "james", false},
		{"two different full middles", "andrew", "beatrice", false},
		{"multi-byte initial matches its full name", "ö", "östen", true},
		{"multi-byte initial vs a different name", "ö", "james", false},
		{"CJK initial matches its full name", "李", "李明", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := compatibleMiddle(tc.ma, tc.mb)
			if got != tc.want {
				t.Errorf("compatibleMiddle(%q, %q) = %v, want %v", tc.ma, tc.mb, got, tc.want)
			}
			// The relation must be symmetric, or merge order would change the answer.
			if rev := compatibleMiddle(tc.mb, tc.ma); rev != got {
				t.Errorf("compatibleMiddle(%q, %q) = %v but the reverse = %v; must be symmetric",
					tc.ma, tc.mb, got, rev)
			}
		})
	}
}

// TestSameNameSurvivesPlaceholderMiddleName drives the same regression through
// the public entry point: two exports of one person where one side carries a
// "." placeholder middle name must score, not crash, and must still merge.
func TestSameNameSurvivesPlaceholderMiddleName(t *testing.T) {
	mk := func(middle string) model.NormalizedContact {
		return normalize.Contact(model.ParsedContact{
			GivenName: "John", MiddleName: middle, FamilyName: "Smith",
			Phones: []model.Phone{{Number: "5558675309"}},
		})
	}
	for _, middle := range []string{".", "..", ". "} {
		got := Score([]model.NormalizedContact{mk(middle), mk("Quincy")}, [][2]int{{0, 1}})[0]
		if !got.Features.NameExact {
			t.Errorf("middle %q: NameExact = false, want a placeholder middle name treated as absent", middle)
		}
		if got.Tier != model.TierAutoMerge {
			t.Errorf("middle %q: tier = %q, want %q", middle, got.Tier, model.TierAutoMerge)
		}
	}
}

// TestScoreCapsAtOne: the weights deliberately over-sum (name+email+phone+org
// +birthday = 1.10) so a birthday can lift a weak pair, which means a perfect
// pair must be clamped or the score leaves the documented 0..1 range.
func TestScoreCapsAtOne(t *testing.T) {
	named := func() model.NormalizedContact {
		return normalize.Contact(model.ParsedContact{
			GivenName: "John", FamilyName: "Smith", Org: "Acme", Birthday: "1989-06-29",
			Emails: []model.Email{{Address: "j@acme.com"}},
			Phones: []model.Phone{{Number: "5558675309"}},
		})
	}
	got := Score([]model.NormalizedContact{named(), named()}, [][2]int{{0, 1}})[0]
	if got.Score != 1.0 {
		t.Errorf("score = %.4f for an all-signals-match named pair, want exactly 1.0", got.Score)
	}
	if got.Tier != model.TierAutoMerge {
		t.Errorf("tier = %q, want %q", got.Tier, model.TierAutoMerge)
	}

	// Same clamp on the nameless weight table (0.45+0.45+0.10+0.10 = 1.10).
	nameless := func() model.NormalizedContact {
		return normalize.Contact(model.ParsedContact{
			Org: "Acme", Birthday: "1989-06-29",
			Emails: []model.Email{{Address: "j@acme.com"}},
			Phones: []model.Phone{{Number: "5558675309"}},
		})
	}
	gotNameless := Score([]model.NormalizedContact{nameless(), nameless()}, [][2]int{{0, 1}})[0]
	if gotNameless.Score != 1.0 {
		t.Errorf("nameless score = %.4f for an all-signals match, want exactly 1.0", gotNameless.Score)
	}
	if !gotNameless.Features.Nameless {
		t.Error("Nameless = false on the nameless weight table")
	}
}
