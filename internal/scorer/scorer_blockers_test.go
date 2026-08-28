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

// Google folds the middle initial into the given name where iCloud uses the
// middle slot. That is the common cross-source shape of one person, and it
// used to drop from auto_merge to review twice over: the bare "V" was read
// as a generational suffix, and "john v" / "john" differed in word count.
func TestSameNameFoldsGivenNameInitialIntoMiddle(t *testing.T) {
	icloud := normalize.Contact(model.ParsedContact{Source: model.SourceICloud,
		GivenName: "John", MiddleName: "V", FamilyName: "Doe", Phones: []model.Phone{{Number: "3175550000"}}})
	google := normalize.Contact(model.ParsedContact{Source: model.SourceGoogle,
		GivenName: "John V", FamilyName: "Doe", Phones: []model.Phone{{Number: "(317) 555-0000"}}})
	got := tierOf(icloud, google)
	if !got.Features.NameExact {
		t.Errorf("NameExact = false for John/V/Doe vs \"John V\"/Doe (suffixes %q vs %q)", icloud.NormalizedSuffix, google.NormalizedSuffix)
	}
	if got.Tier != model.TierAutoMerge {
		t.Errorf("tier = %q, want %q", got.Tier, model.TierAutoMerge)
	}

	// A different folded initial is still a different person.
	other := normalize.Contact(model.ParsedContact{Source: model.SourceGoogle,
		GivenName: "John P", FamilyName: "Doe", Phones: []model.Phone{{Number: "3175550000"}}})
	if got := tierOf(icloud, other); got.Features.NameExact {
		t.Error("NameExact = true for middle V vs folded initial P")
	}

	// A real fifth-generation suffix in the N suffix component still separates.
	fifth := normalize.Contact(model.ParsedContact{Source: model.SourceGoogle,
		GivenName: "John", FamilyName: "Doe", Suffix: "V", Phones: []model.Phone{{Number: "3175550000"}}})
	plain := normalize.Contact(model.ParsedContact{Source: model.SourceICloud,
		GivenName: "John", FamilyName: "Doe", Phones: []model.Phone{{Number: "3175550000"}}})
	if got := tierOf(fifth, plain); got.Features.NameExact {
		t.Error("NameExact = true for John Doe V vs John Doe")
	}
}

// A folded middle initial must stay in the name, not be eaten as a suffix.
// "v" sat in normalize's nameSuffixes table, so Name("John V") returned
// "john": the middle initial vanished, and an empty middle name compares
// compatible with every other initial. "John V Doe" and "John W Doe" sharing
// one email then auto-merged — two people, one contact, no review card.
// GenerationalSuffix already refuses to read a trailing single letter as
// generational; Name now applies the same rule.
func TestMiddleInitialIsNotAGenerationalSuffix(t *testing.T) {
	named := func(given, middle, suffix, email string) model.NormalizedContact {
		return normalize.Contact(model.ParsedContact{
			GivenName: given, MiddleName: middle, FamilyName: "Doe",
			Suffix: suffix, Emails: []model.Email{{Address: email}},
		})
	}
	const shared = "doe.family@example.com"

	// Different middle initials are different people, even on a shared email.
	for _, tc := range []struct{ ga, ma, gb, mb string }{
		{"John V", "", "John W", ""},
		{"John V", "", "John", "X"},
		{"John", "V", "John", "W"},
	} {
		got := tierOf(named(tc.ga, tc.ma, "", shared), named(tc.gb, tc.mb, "", shared))
		if got.Features.NameExact || got.Tier == model.TierAutoMerge {
			t.Errorf("%q/%q vs %q/%q: NameExact=%v tier=%q, want no auto_merge on differing middle initials",
				tc.ga, tc.ma, tc.gb, tc.mb, got.Features.NameExact, got.Tier)
		}
	}

	// The cross-source shape this rule exists for still matches: Google folds
	// the initial into the given name, iCloud keeps it in the middle slot.
	if got := tierOf(named("John V", "", "", shared), named("John", "V", "", shared)); !got.Features.NameExact {
		t.Error(`"John V"/"" vs "John"/"V": NameExact = false, want the folded initial to match the middle slot`)
	}

	// A real generational V lives in the N suffix component and still counts.
	if got := tierOf(named("John", "", "V", shared), named("John", "", "", shared)); got.Features.NameExact {
		t.Error(`suffix "V" vs none: NameExact = true, want a generational suffix to distinguish`)
	}

	// Titles are multi-letter and must still be stripped.
	if normalize.Name("Dr. John") != "john" {
		t.Errorf(`Name("Dr. John") = %q, want "john"`, normalize.Name("Dr. John"))
	}
	// The single-letter guard must not resurrect stripped multi-letter suffixes.
	if normalize.Name("John Jr.") != "john" {
		t.Errorf(`Name("John Jr.") = %q, want "john"`, normalize.Name("John Jr."))
	}
}

// Diacritics distinguish people. normalize.Name applies NFKD and drops every
// combining mark, so "Nguyên" and "Nguyễn" both fold to "nguyen". That folding
// is right for blocking and similarity, but the exact-name rule merges on one
// identifier, so on the folded form alone two household members sharing a
// landline were fused with no review card. Vietnamese given names are
// distinguished almost entirely by tone marks.
func TestExactNameRequiresMatchingDiacritics(t *testing.T) {
	named := func(given, family string) model.NormalizedContact {
		return normalize.Contact(model.ParsedContact{
			GivenName: given, FamilyName: family,
			Phones: []model.Phone{{Number: "555-0100"}},
		})
	}

	// Different names that fold together must not auto-merge.
	for _, tc := range []struct{ ga, fa, gb, fb string }{
		{"Nguyên", "Le", "Nguyễn", "Le"},
		{"Hà", "Le", "Ha", "Le"},
		{"René", "Dupont", "Rene", "Dupont"},
	} {
		got := tierOf(named(tc.ga, tc.fa), named(tc.gb, tc.fb))
		if got.Features.NameExact || got.Tier == model.TierAutoMerge {
			t.Errorf("%q %q vs %q %q: NameExact=%v tier=%q, want no auto_merge on names that differ by marks",
				tc.ga, tc.fa, tc.gb, tc.fb, got.Features.NameExact, got.Tier)
		}
		// Recall is preserved: the pair still reaches a human.
		if got.Tier != model.TierReview {
			t.Errorf("%q vs %q: tier = %q, want the pair still surfaced for review", tc.ga, tc.gb, got.Tier)
		}
	}

	// Identical accented names, plain ASCII names and nicknames still match.
	for _, tc := range []struct{ ga, fa, gb, fb string }{
		{"José", "Garcia", "José", "Garcia"},
		{"John", "Smith", "John", "Smith"},
		{"Bob", "Smith", "Robert", "Smith"},
		// Compatibility variants are the SAME name written two ways: fullwidth
		// Latin and halfwidth kana are a routine iCloud-vs-Google divergence,
		// so NameStrict folds them (NFKC) while keeping the combining marks
		// that separate "Nguyên" from "Nguyễn".
		{"Ｊｏｈｎ", "Smith", "John", "Smith"},
		{"ﾀﾅｶ", "Z", "タナカ", "Z"},
	} {
		if got := tierOf(named(tc.ga, tc.fa), named(tc.gb, tc.fb)); !got.Features.NameExact {
			t.Errorf("%q %q vs %q %q: NameExact = false, want the rule to still fire",
				tc.ga, tc.fa, tc.gb, tc.fb)
		}
	}
}

// A set of initials is not a name, however it is punctuated. isInitial counted
// runes on the whole string, so "j.r." (three runes) passed the guard that
// rejects "j", and two different "J.R. Smith"s on one office switchboard
// auto-merged with no review card.
func TestSameNameRejectsMultipleInitials(t *testing.T) {
	for _, given := range []string{"J.R.", "J R", "A.B.", "J.R.T.", "J"} {
		a := parsed(given, "Smith", "", []string{"jr.smith@corp.com"}, []string{"3175550000"})
		b := parsed(given, "Smith", "", []string{"jrt.smith@corp.com"}, []string{"3175550000"})
		got := tierOf(a, b)
		if got.Features.NameExact || got.Tier == model.TierAutoMerge {
			t.Errorf("%q Smith on a shared phone: NameExact=%v tier=%q, want no auto_merge on initials",
				given, got.Features.NameExact, got.Tier)
		}
	}
	// A real name with an initial after it is still a name.
	a := parsed("John R.", "Smith", "", nil, []string{"3175550000"})
	b := parsed("John R.", "Smith", "", nil, []string{"3175550000"})
	if got := tierOf(a, b); !got.Features.NameExact {
		t.Error(`"John R." Smith: NameExact = false, want a real given name to still match`)
	}
}
