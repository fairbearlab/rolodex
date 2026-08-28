package scorer

import (
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/normalize"
)

// These tests pin the silent-data-loss cases found by the pre-landing
// red-team pass on the sparse-export scoring rules. Each was reproduced
// end-to-end before being fixed; each fix strictly increases precision.

func parsed(given, family, bday string, emails, phones []string) model.NormalizedContact {
	c := model.ParsedContact{GivenName: given, FamilyName: family, Birthday: normalize.Birthday(bday)}
	for _, e := range emails {
		c.Emails = append(c.Emails, model.Email{Address: e})
	}
	for _, p := range phones {
		c.Phones = append(c.Phones, model.Phone{Number: p})
	}
	return normalize.Contact(c)
}

func tierOf(a, b model.NormalizedContact) model.ScoredPair {
	return Score([]model.NormalizedContact{a, b}, [][2]int{{0, 1}})[0]
}

// Two contacts with no family name do not share one. "Alex" and "Alex" on
// one office switchboard, different emails, are as likely two people as one.
func TestSameNameNeedsAFamilyName(t *testing.T) {
	a := parsed("Alex", "", "", []string{"alex.rivera@corp.com"}, []string{"+1 415 555 0100"})
	b := parsed("Alex", "", "", []string{"alex.tan@corp.com"}, []string{"(415) 555-0100"})
	got := tierOf(a, b)
	if got.Features.NameExact {
		t.Error("NameExact = true for two given-name-only contacts")
	}
	if got.Tier != model.TierReview {
		t.Errorf("tier = %q, want %q: a shared switchboard is not identity", got.Tier, model.TierReview)
	}
	if !got.Features.NearName() {
		t.Error("NearName = false; the pair should still be surfaced for a human")
	}
}

// A single-letter given name is an initial, not a name.
func TestSameNameRejectsInitials(t *testing.T) {
	cases := []struct{ ga, gb string }{
		{"J", "J"}, {"J.", "J."}, {"J", "John"}, {"John", "J."},
	}
	for _, tc := range cases {
		a := parsed(tc.ga, "Smith", "", nil, []string{"3175550000"})
		b := parsed(tc.gb, "Smith", "", nil, []string{"3175550000"})
		if got := tierOf(a, b); got.Features.NameExact || got.Tier == model.TierAutoMerge {
			t.Errorf("%q/%q Smith on a shared phone: NameExact=%v tier=%q, want no auto_merge on an initial",
				tc.ga, tc.gb, got.Features.NameExact, got.Tier)
		}
	}
	// Multi-byte initial ("Ö") is still an initial; a two-letter name is not.
	if isInitial("ö") != true || isInitial("al") != false {
		t.Error("isInitial counts runes, not bytes")
	}
}

// Equal raw strings that are not dates are not a shared birthday. Before
// this, "1989" == "1989" promoted an identical name to auto_merge with no
// shared phone or email at all.
func TestSharedBirthdayRequiresCanonicalDates(t *testing.T) {
	for _, bday := range []string{"1989", "1989-10", "unknown", "0", "0000-00-00", "1989-13-45"} {
		a := parsed("Maria", "Rodriguez", bday, []string{"maria.r@example.com"}, nil)
		b := parsed("Maria", "Rodriguez", bday, []string{"mrodriguez@example.org"}, nil)
		got := tierOf(a, b)
		if got.Features.SharedBirthday {
			t.Errorf("bday=%q: SharedBirthday = true for a non-date", bday)
		}
		if got.Tier == model.TierAutoMerge {
			t.Errorf("bday=%q: tier = auto_merge with disjoint emails and no phone", bday)
		}
	}
	// The no-year comparison is month/day equality, not a suffix test.
	if sharedBirthday(withBirthday(makeContact("a", "b", nil, nil, ""), "--06-29"),
		withBirthday(makeContact("a", "b", nil, nil, ""), "garbage-06-29")) {
		t.Error("sharedBirthday matched a no-year date against a suffix of free text")
	}
}

// The birthday guard must fail closed. A hand-typed US slash date used to
// pass through normalization unchanged and silently disable the conflict
// check, merging a father and son 37 years apart on the family landline.
func TestBirthdayGuardFailsClosed(t *testing.T) {
	father := parsed("John", "Smith", "1952-03-15", nil, []string{"2125550199"})
	son := parsed("John", "Smith", "10/22/1989", nil, []string{"2125550199"})
	got := tierOf(father, son)
	if !got.Features.BirthdayConflict {
		t.Errorf("BirthdayConflict = false: %q should now normalize and disagree with 1952-03-15", son.Parsed.Birthday)
	}
	if got.Tier != model.TierReview {
		t.Errorf("tier = %q, want %q", got.Tier, model.TierReview)
	}

	// A birthday the normalizer still cannot read: unknown, so the
	// single-identifier exact-name rule does not fire.
	unreadable := parsed("John", "Smith", "born in the fifties", nil, []string{"2125550199"})
	got = tierOf(father, unreadable)
	if got.Features.BirthdayConflict {
		t.Error("BirthdayConflict = true for an unreadable birthday; it is unknown, not a conflict")
	}
	if !got.Features.BirthdayUnknown {
		t.Error("BirthdayUnknown = false")
	}
	if got.Tier != model.TierReview {
		t.Errorf("tier = %q, want %q: the guard could not run, so the name rule must not merge", got.Tier, model.TierReview)
	}

	// Two identifiers (score >= auto_merge threshold) still clear it: the
	// unknown birthday only withholds the single-identifier shortcut.
	strong := parsed("John", "Smith", "born in the fifties", []string{"john@example.com"}, []string{"2125550199"})
	fatherStrong := parsed("John", "Smith", "1952-03-15", []string{"john@example.com"}, []string{"2125550199"})
	if got := tierOf(fatherStrong, strong); got.Tier != model.TierAutoMerge {
		t.Errorf("tier = %q, want %q with a shared email and phone", got.Tier, model.TierAutoMerge)
	}
}
