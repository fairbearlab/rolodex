package scorer

import (
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

// The nickname table serves two jobs that were pulling against each other:
// it feeds name *similarity* (so Bob Smith and Robert Smith get a review
// card) and, through sameGivenName, name *identity* (so Bob Smith + one
// shared identifier auto-merged with no card). Identity is the stronger
// claim and a nickname cannot make it: Alex/Alexander is also
// Alex/Alexandra, Sam/Samuel is Sam/Samantha, Pat/Patrick is Pat/Patricia,
// Frank/Francis is Frank/Frances — siblings and spouses on one household
// landline. A nickname pair with one shared identifier is a review card,
// never an auto-merge; that costs a keystroke, not a person.
func TestNicknameIsSimilarityNotIdentity(t *testing.T) {
	for _, pair := range [][2]string{
		{"Alexander", "Alex"}, {"Robert", "Bert"}, {"Samuel", "Sam"}, {"Patrick", "Pat"},
		{"Benjamin", "Ben"}, {"Raymond", "Ray"}, {"Francis", "Frank"}, {"Elizabeth", "Beth"},
		{"Andrew", "Drew"}, {"Christopher", "Chris"}, {"Robert", "Bob"},
	} {
		a := parsed(pair[0], "Smith", "", nil, []string{"317-555-9876"})
		b := parsed(pair[1], "Smith", "", nil, []string{"(317) 555-9876"})
		got := tierOf(a, b)
		if got.Features.NameExact {
			t.Errorf("%s/%s: NameExact = true; a nickname is not an identical name", pair[0], pair[1])
		}
		if got.Tier != model.TierReview {
			t.Errorf("%s/%s + shared phone: tier = %q (score %.3f), want review", pair[0], pair[1], got.Tier, got.Score)
		}
		if got.Features.NameSimilarity < 1.0 {
			t.Errorf("%s/%s: similarity = %.3f, want the nickname expansion to still make them alike", pair[0], pair[1], got.Features.NameSimilarity)
		}
	}

	// A literally identical name with one identifier still auto-merges.
	a := parsed("Robert", "Smith", "", nil, []string{"317-555-9876"})
	b := parsed("Robert", "Smith", "", nil, []string{"(317) 555-9876"})
	if got := tierOf(a, b); got.Tier != model.TierAutoMerge || !got.Features.NameExact {
		t.Errorf("Robert/Robert + shared phone: tier=%q exact=%v, want auto_merge", got.Tier, got.Features.NameExact)
	}
}

// Removing jack, liam, jamie, leo and harry from the table because they are
// standalone names fixed the identity problem for those five at the cost of
// the similarity floor: Jack Smith and John Smith on one phone scored 0.578
// and were marked distinct — two cards written to merged.vcf as separate
// people, never shown to a human. Now that a nickname cannot make an
// identity, the five can go back in and contribute to similarity only.
func TestStandaloneNicknamesStillReachReview(t *testing.T) {
	for _, pair := range [][2]string{
		{"Jack", "John"}, {"Liam", "William"}, {"Jamie", "James"}, {"Leo", "Leonard"}, {"Harry", "Henry"},
	} {
		a := parsed(pair[0], "Smith", "", nil, []string{"317-555-9876"})
		b := parsed(pair[1], "Smith", "", nil, []string{"317-555-9876"})
		got := tierOf(a, b)
		if got.Tier != model.TierReview {
			t.Errorf("%s/%s + shared phone: tier = %q (score %.3f), want review", pair[0], pair[1], got.Tier, got.Score)
		}
		if got.Features.NameExact {
			t.Errorf("%s/%s: NameExact = true, want a standalone name never to be an identity match", pair[0], pair[1])
		}
	}
}
