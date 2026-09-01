package scorer

import (
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/normalize"
)

func makeContact(given, family string, emails []string, phones []string, org string) model.NormalizedContact {
	var em []model.Email
	for _, e := range emails {
		em = append(em, model.Email{Address: e})
	}
	var ph []model.Phone
	for _, p := range phones {
		ph = append(ph, model.Phone{Number: p})
	}

	return model.NormalizedContact{
		Parsed: model.ParsedContact{
			GivenName:  given,
			FamilyName: family,
			Emails:     em,
			Phones:     ph,
			Org:        org,
		},
		NormalizedGivenName:  given,
		NormalizedFamilyName: family,
		NormalizedEmails:     emails,
		NormalizedPhones:     phones,
	}
}

func TestScoreIdenticalContacts(t *testing.T) {
	a := makeContact("robert", "smith", []string{"bob@gmail.com"}, []string{"5551234567"}, "Acme")
	b := makeContact("robert", "smith", []string{"bob@gmail.com"}, []string{"5551234567"}, "Acme")

	contacts := []model.NormalizedContact{a, b}
	pairs := [][2]int{{0, 1}}
	scored := Score(contacts, pairs)

	if len(scored) != 1 {
		t.Fatalf("expected 1 scored pair, got %d", len(scored))
	}
	if scored[0].Score < model.ThresholdAutoMerge {
		t.Errorf("score = %.3f, expected >= %.2f for identical contacts", scored[0].Score, model.ThresholdAutoMerge)
	}
	if scored[0].Tier != model.TierAutoMerge {
		t.Errorf("tier = %q, want %q", scored[0].Tier, model.TierAutoMerge)
	}
}

func TestScoreNicknameMatch(t *testing.T) {
	// Bob and Robert with shared email + phone should auto-merge via nickname expansion
	a := makeContact("bob", "smith", []string{"bob@gmail.com"}, []string{"5551234567"}, "")
	b := makeContact("robert", "smith", []string{"bob@gmail.com"}, []string{"5551234567"}, "")

	contacts := []model.NormalizedContact{a, b}
	pairs := [][2]int{{0, 1}}
	scored := Score(contacts, pairs)

	if scored[0].Score < model.ThresholdAutoMerge {
		t.Errorf("score = %.3f, expected >= %.2f for Bob/Robert + shared email+phone",
			scored[0].Score, model.ThresholdAutoMerge)
	}
}

func TestScoreNicknameSharedEmailIsReviewed(t *testing.T) {
	// Bob and Robert expand to the same name, so with a shared email the
	// pair scores 0.65 and is reviewed. It is not an exact-name match: a
	// nickname is similarity, not identity, so one identifier does not
	// auto-merge it (see TestNicknameIsSimilarityNotIdentity).
	a := makeContact("bob", "smith", []string{"bob@gmail.com"}, nil, "")
	b := makeContact("robert", "smith", []string{"bob@gmail.com"}, nil, "")

	contacts := []model.NormalizedContact{a, b}
	pairs := [][2]int{{0, 1}}
	scored := Score(contacts, pairs)

	if scored[0].Tier != model.TierReview || scored[0].Features.NameExact {
		t.Errorf("tier = %q exact=%v (score=%.3f), want review for Bob/Robert + shared email",
			scored[0].Tier, scored[0].Features.NameExact, scored[0].Score)
	}
}

func TestScoreTransitiveNickname(t *testing.T) {
	// Bobby and Bob should both resolve to Robert
	a := makeContact("bobby", "smith", []string{"bob@gmail.com"}, []string{"5551234567"}, "")
	b := makeContact("bob", "smith", []string{"bob@gmail.com"}, []string{"5551234567"}, "")

	contacts := []model.NormalizedContact{a, b}
	pairs := [][2]int{{0, 1}}
	scored := Score(contacts, pairs)

	// Both expand to "robert", so name similarity should be 1.0
	if scored[0].Score < model.ThresholdAutoMerge {
		t.Errorf("score = %.3f, Bobby/Bob should auto-merge with shared email+phone", scored[0].Score)
	}
}

func TestScoreDistinctContacts(t *testing.T) {
	a := makeContact("alice", "johnson", []string{"alice@example.com"}, nil, "BigCo")
	b := makeContact("charlie", "williams", []string{"charlie@other.com"}, nil, "SmallCo")

	contacts := []model.NormalizedContact{a, b}
	pairs := [][2]int{{0, 1}}
	scored := Score(contacts, pairs)

	if scored[0].Tier != model.TierDistinct {
		t.Errorf("tier = %q, want %q for completely different contacts", scored[0].Tier, model.TierDistinct)
	}
}

func TestScoreNamelessContact(t *testing.T) {
	// Contact with no name but shared email + phone should still merge
	a := makeContact("", "", []string{"shared@example.com"}, []string{"5551234567"}, "")
	b := makeContact("", "", []string{"shared@example.com"}, []string{"5551234567"}, "")

	contacts := []model.NormalizedContact{a, b}
	pairs := [][2]int{{0, 1}}
	scored := Score(contacts, pairs)

	if scored[0].Tier != model.TierAutoMerge {
		t.Errorf("tier = %q, want auto_merge for nameless contacts with shared email+phone (score=%.3f)",
			scored[0].Tier, scored[0].Score)
	}
}

func TestScoreNamelessSingleIdentifier(t *testing.T) {
	// Nameless contact with only one shared identifier should NOT auto-merge
	a := makeContact("", "", []string{"shared@example.com"}, nil, "")
	b := makeContact("", "", []string{"shared@example.com"}, nil, "")

	contacts := []model.NormalizedContact{a, b}
	pairs := [][2]int{{0, 1}}
	scored := Score(contacts, pairs)

	if scored[0].Tier == model.TierAutoMerge {
		t.Errorf("nameless contact with single shared identifier should not auto-merge (score=%.3f)",
			scored[0].Score)
	}
}

func TestExpandName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Bob", "robert"},
		{"Bobby", "robert"},
		{"Robert", "robert"},
		{"bill", "william"},
		{"Mike", "michael"},
		{"Jim", "james"},
		{"UnknownName", "unknownname"},
		{"", ""},
		{"  Bob  ", "robert"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := expandName(tt.input)
			if got != tt.want {
				t.Errorf("expandName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func withBirthday(c model.NormalizedContact, bday string) model.NormalizedContact {
	c.Parsed.Birthday = bday
	return c
}

// TestClassifyTiers pins the tier for the pair shapes that real exports
// actually produce: most contacts carry a name and at most one identifier.
func TestClassifyTiers(t *testing.T) {
	cases := []struct {
		name     string
		a, b     model.NormalizedContact
		wantTier model.Tier
		minScore float64 // sanity check on the linear score
	}{
		{
			name:     "exact name + shared phone only",
			a:        makeContact("chris", "fielding", nil, []string{"3175559876"}, ""),
			b:        makeContact("chris", "fielding", nil, []string{"3175559876"}, "Continental Aeronautics"),
			wantTier: model.TierAutoMerge, minScore: 0.65,
		},
		{
			name:     "exact name + shared email only",
			a:        makeContact("ahmed", "mady", []string{"a@example.com"}, nil, ""),
			b:        makeContact("ahmed", "mady", []string{"a@example.com"}, nil, ""),
			wantTier: model.TierAutoMerge, minScore: 0.65,
		},
		{
			name:     "exact name + shared birthday only",
			a:        withBirthday(makeContact("jimmy", "schuler", nil, nil, ""), "1989-06-29"),
			b:        withBirthday(makeContact("jimmy", "schuler", nil, nil, ""), "1989-06-29"),
			wantTier: model.TierAutoMerge, minScore: 0.50,
		},
		{
			name:     "exact name + no-year birthday matching full date",
			a:        withBirthday(makeContact("jimmy", "schuler", nil, nil, ""), "--06-29"),
			b:        withBirthday(makeContact("jimmy", "schuler", nil, nil, ""), "1989-06-29"),
			wantTier: model.TierAutoMerge, minScore: 0.50,
		},
		{
			name:     "exact name + shared org only stays in review",
			a:        makeContact("jimmy", "schuler", nil, nil, "Kunkels Drive-In"),
			b:        makeContact("jimmy", "schuler", nil, nil, "Kunkels Drive-In"),
			wantTier: model.TierReview, minScore: 0.50,
		},
		{
			name:     "exact name, nothing else: floored into review, not distinct",
			a:        makeContact("john", "smith", nil, nil, ""),
			b:        makeContact("john", "smith", nil, nil, ""),
			wantTier: model.TierReview, minScore: 0.40,
		},
		{
			name:     "exact name, different birthdays, nothing else: review (not auto)",
			a:        withBirthday(makeContact("john", "smith", nil, nil, ""), "1980-01-01"),
			b:        withBirthday(makeContact("john", "smith", nil, nil, ""), "1975-05-05"),
			wantTier: model.TierReview, minScore: 0.40,
		},
		{
			name:     "near-miss name + shared phone: below exact threshold, linear review",
			a:        makeContact("chris", "fielding", nil, []string{"5551234567"}, ""),
			b:        makeContact("kris", "fielding", nil, []string{"5551234567"}, ""),
			wantTier: model.TierReview, minScore: 0.60,
		},
		{
			name:     "fuzzy name only: still distinct",
			a:        makeContact("jon", "smith", nil, nil, ""),
			b:        makeContact("joan", "smithers", nil, nil, ""),
			wantTier: model.TierDistinct, minScore: 0,
		},
		{
			name:     "different people, same last name, shared org: distinct",
			a:        makeContact("alice", "smith", nil, nil, "Acme"),
			b:        makeContact("zed", "smith", nil, nil, "Acme"),
			wantTier: model.TierDistinct, minScore: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scored := Score([]model.NormalizedContact{tc.a, tc.b}, [][2]int{{0, 1}})
			got := scored[0]
			if got.Tier != tc.wantTier {
				t.Errorf("tier = %q (score=%.3f, features=%+v), want %q", got.Tier, got.Score, got.Features, tc.wantTier)
			}
			if got.Score < tc.minScore {
				t.Errorf("score = %.3f, want >= %.2f", got.Score, tc.minScore)
			}
		})
	}
}

func TestSharedBirthday(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1989-06-29", "1989-06-29", true},
		{"--06-29", "1989-06-29", true},
		{"1989-06-29", "--06-29", true},
		{"--06-29", "--06-29", true},
		{"1989-06-29", "1990-06-29", false},
		{"--06-29", "--06-30", false},
		{"", "1989-06-29", false},
		{"", "", false},
	}
	for _, tc := range cases {
		a := withBirthday(makeContact("a", "b", nil, nil, ""), tc.a)
		b := withBirthday(makeContact("a", "b", nil, nil, ""), tc.b)
		if got := sharedBirthday(a, b); got != tc.want {
			t.Errorf("sharedBirthday(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestClassifyPrecisionGuards pins the cases the adversarial review found:
// "exact" must mean identical, not Jaro-Winkler >= 0.95, and a contradicting
// birthday must always route to a human.
func TestClassifyPrecisionGuards(t *testing.T) {
	cases := []struct {
		name     string
		a, b     model.NormalizedContact
		wantTier model.Tier
	}{
		{
			name:     "Eric/Erica on a shared household phone: near name, not exact -> review",
			a:        makeContact("eric", "johnson", nil, []string{"3175551212"}, ""),
			b:        makeContact("erica", "johnson", nil, []string{"3175551212"}, ""),
			wantTier: model.TierReview,
		},
		{
			name:     "Paul/Paula on a shared email -> review",
			a:        makeContact("paul", "nguyen", []string{"nguyens@example.com"}, nil, ""),
			b:        makeContact("paula", "nguyen", []string{"nguyens@example.com"}, nil, ""),
			wantTier: model.TierReview,
		},
		{
			name:     "identical name, shared phone, different birthdays -> review",
			a:        withBirthday(makeContact("john", "smith", nil, []string{"3175554444"}, ""), "1970-01-01"),
			b:        withBirthday(makeContact("john", "smith", nil, []string{"3175554444"}, ""), "1995-12-31"),
			wantTier: model.TierReview,
		},
		{
			name:     "no-year birthday disagreeing with a full date -> review",
			a:        withBirthday(makeContact("ann", "lee", nil, []string{"3175554444"}, ""), "--06-29"),
			b:        withBirthday(makeContact("ann", "lee", nil, []string{"3175554444"}, ""), "1990-07-04"),
			wantTier: model.TierReview,
		},
		{
			name: "even a full-score pair is held when birthdays disagree",
			a: withBirthday(makeContact("john", "smith", []string{"js@example.com"},
				[]string{"3175554444"}, "Acme"), "1970-01-01"),
			b: withBirthday(makeContact("john", "smith", []string{"js@example.com"},
				[]string{"3175554444"}, "Acme"), "1995-12-31"),
			wantTier: model.TierReview,
		},
		{
			name:     "unreadable birthday is not a conflict, but it disarms the exact-name rule -> review",
			a:        withBirthday(makeContact("john", "smith", nil, []string{"3175554444"}, ""), "circa 1950"),
			b:        withBirthday(makeContact("john", "smith", nil, []string{"3175554444"}, ""), "1995-12-31"),
			wantTier: model.TierReview,
		},
		{
			name:     "unreadable birthday on one side only, none on the other -> nothing to disagree with, auto_merge",
			a:        withBirthday(makeContact("john", "smith", nil, []string{"3175554444"}, ""), "circa 1950"),
			b:        makeContact("john", "smith", nil, []string{"3175554444"}, ""),
			wantTier: model.TierAutoMerge,
		},
		{
			name:     "Will/Liam on a shared phone: similar through the table, not identical -> review",
			a:        makeContact("will", "smith", nil, []string{"3175550000"}, ""),
			b:        makeContact("liam", "smith", nil, []string{"3175550000"}, ""),
			wantTier: model.TierReview,
		},
		{
			name:     "Chris/Christopher on a shared phone: a nickname is not identity -> review",
			a:        makeContact("chris", "petry", nil, []string{"3175550000"}, ""),
			b:        makeContact("christopher", "petry", nil, []string{"3175550000"}, ""),
			wantTier: model.TierReview,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Score([]model.NormalizedContact{tc.a, tc.b}, [][2]int{{0, 1}})[0]
			if got.Tier != tc.wantTier {
				t.Errorf("tier = %q (score=%.3f, features=%+v), want %q", got.Tier, got.Score, got.Features, tc.wantTier)
			}
			if tc.wantTier == model.TierAutoMerge && got.Features.BirthdayConflict {
				t.Error("auto_merge must never carry a birthday conflict")
			}
		})
	}
}

func TestNameExactFeature(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"chris", "christopher", false}, // a nickname is similarity, not identity
		{"eric", "erica", false},
		{"jon", "john", false},
		{"will", "liam", false},
		{"john", "john", true},
	}
	for _, tc := range cases {
		got := Score([]model.NormalizedContact{
			makeContact(tc.a, "x", nil, nil, ""), makeContact(tc.b, "x", nil, nil, ""),
		}, [][2]int{{0, 1}})[0].Features
		if got.NameExact != tc.want {
			t.Errorf("NameExact(%s, %s) = %v (sim %.3f), want %v", tc.a, tc.b, got.NameExact, got.NameSimilarity, tc.want)
		}
	}
	nameless := Score([]model.NormalizedContact{
		makeContact("", "", []string{"a@b.c"}, nil, ""), makeContact("", "", []string{"a@b.c"}, nil, ""),
	}, [][2]int{{0, 1}})[0].Features
	if !nameless.Nameless {
		t.Error("nameless pair should record Nameless=true for the TUI")
	}
}

func TestSameNameIdentity(t *testing.T) {
	mk := func(given, middle, family, suffix string) model.NormalizedContact {
		c := model.ParsedContact{GivenName: given, MiddleName: middle, FamilyName: family, Suffix: suffix,
			Phones: []model.Phone{{Number: "5558675309"}}}
		return normalize.Contact(c)
	}
	cases := []struct {
		name string
		a, b model.NormalizedContact
		want model.Tier
	}{
		{"Jr vs Sr on one landline", mk("John", "", "Smith", "Jr."), mk("John", "", "Smith", "Sr."), model.TierReview},
		{"Jr vs no suffix", mk("John", "", "Smith", "Jr."), mk("John", "", "Smith", ""), model.TierReview},
		{"suffix folded into family name", mk("John", "", "Smith Jr.", ""), mk("John", "", "Smith Sr.", ""), model.TierReview},
		{"III vs IV", mk("John", "", "Smith", "III"), mk("John", "", "Smith", "IV"), model.TierReview},
		{"same suffix both sides", mk("John", "", "Smith", "Jr."), mk("John", "", "Smith", "Jr"), model.TierAutoMerge},
		{"credential suffix ignored", mk("John", "", "Smith", "MD"), mk("John", "", "Smith", ""), model.TierAutoMerge},
		{"different middle names", mk("John", "Andrew", "Smith", ""), mk("John", "Beatrice", "Smith", ""), model.TierReview},
		{"different middle initials", mk("John", "A.", "Smith", ""), mk("John", "B", "Smith", ""), model.TierReview},
		{"initial matches full middle", mk("Charles", "J.", "Galanti", ""), mk("Charles", "James", "Galanti", ""), model.TierAutoMerge},
		{"missing middle on one side", mk("Charles", "J.", "Galanti", ""), mk("Charles", "", "Galanti", ""), model.TierAutoMerge},
		{"two diminutives of one canonical: Ted/Ned", mk("Ted", "", "Smith", ""), mk("Ned", "", "Smith", ""), model.TierReview},
		{"Beth/Betty", mk("Beth", "", "Smith", ""), mk("Betty", "", "Smith", ""), model.TierReview},
		{"nickname vs canonical: Chris/Christopher", mk("Chris", "", "Petry", ""), mk("Christopher", "", "Petry", ""), model.TierReview},
		{"standalone nickname: Jack/John", mk("Jack", "", "Smith", ""), mk("John", "", "Smith", ""), model.TierReview},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Score([]model.NormalizedContact{tc.a, tc.b}, [][2]int{{0, 1}})[0]
			if got.Tier != tc.want {
				t.Errorf("tier = %q (score=%.3f, NameExact=%v), want %q", got.Tier, got.Score, got.Features.NameExact, tc.want)
			}
		})
	}
}

// sameNameParts rejects on the family name before anything else; the
// given/middle reconciliation must never run across different families.
func TestSameNamePartsFamilyMismatch(t *testing.T) {
	if sameNameParts("doe", "john", "", "smith", "john", "") {
		t.Error("different family names compared as the same name")
	}
}

// A given name that is all separators has no tokens; it carries no more
// identity than an initial does, so it must be treated as one.
func TestIsInitialDegenerateGivenNames(t *testing.T) {
	for _, given := range []string{"", ".", " . ", "\t"} {
		if !isInitial(given) {
			t.Errorf("isInitial(%q) = false, want true", given)
		}
	}
	if isInitial("Jo") {
		t.Error("a two-letter name is not an initial")
	}
}
