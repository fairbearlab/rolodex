package scorer

import (
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
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

func TestScoreNicknameSharedEmailAutoMerges(t *testing.T) {
	// Bob and Robert expand to the same name; with a shared email that is
	// an exact-name + identifier match and auto-merges (the linear score
	// alone is only 0.65).
	a := makeContact("bob", "smith", []string{"bob@gmail.com"}, nil, "")
	b := makeContact("robert", "smith", []string{"bob@gmail.com"}, nil, "")

	contacts := []model.NormalizedContact{a, b}
	pairs := [][2]int{{0, 1}}
	scored := Score(contacts, pairs)

	if scored[0].Tier != model.TierAutoMerge {
		t.Errorf("tier = %q (score=%.3f), want auto_merge for Bob/Robert + shared email",
			scored[0].Tier, scored[0].Score)
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
