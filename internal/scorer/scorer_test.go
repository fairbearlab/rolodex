package scorer

import (
	"testing"

	"github.com/fairbearlabs/rolodex/internal/model"
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

func TestScoreNicknameReviewTier(t *testing.T) {
	// Bob and Robert with only shared email should land in review tier, not distinct
	a := makeContact("bob", "smith", []string{"bob@gmail.com"}, nil, "")
	b := makeContact("robert", "smith", []string{"bob@gmail.com"}, nil, "")

	contacts := []model.NormalizedContact{a, b}
	pairs := [][2]int{{0, 1}}
	scored := Score(contacts, pairs)

	if scored[0].Tier != model.TierReview {
		t.Errorf("tier = %q (score=%.3f), want review for Bob/Robert + shared email only",
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
