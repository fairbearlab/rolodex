package merger

import (
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/normalize"
	"github.com/fairbearlab/rolodex/internal/scorer"
)

// namesake builds a contact with the given name and its own private phone
// and email, so that a set of them shares nothing but the name.
func namesake(src model.Source, given, family string, i int) model.NormalizedContact {
	return normalize.Contact(model.ParsedContact{
		Source: src, GivenName: given, FamilyName: family,
		FormattedName: given + " " + family,
		Emails:        []model.Email{{Address: given + string(rune('a'+i)) + "@example.com"}},
		Phones:        []model.Phone{{Number: "212555010" + string(rune('0'+i))}},
	})
}

// allPairs scores every pair, as blocking would for contacts sharing a name.
func allPairs(contacts []model.NormalizedContact) []model.ScoredPair {
	var idx [][2]int
	for i := range contacts {
		for j := i + 1; j < len(contacts); j++ {
			idx = append(idx, [2]int{i, j})
		}
	}
	return scorer.Score(contacts, idx)
}

// Six unrelated "David Lee"s — three per source, six phones, six emails,
// zero shared identifiers — used to become one six-member review cluster
// with one merge-all action. They must come out as pairs, and no review
// cluster may hold more than two of them.
func TestMergeDoesNotChainNearNameOnlyEdges(t *testing.T) {
	var contacts []model.NormalizedContact
	for i := 0; i < 3; i++ {
		contacts = append(contacts, namesake(model.SourceICloud, "David", "Lee", i))
	}
	for i := 3; i < 6; i++ {
		contacts = append(contacts, namesake(model.SourceGoogle, "David", "Lee", i))
	}
	pairs := allPairs(contacts)
	for _, p := range pairs {
		if p.Tier != model.TierReview {
			t.Fatalf("pair %d-%d tier = %q, want review (test precondition: name-only pairs are floored at review)", p.A, p.B, p.Tier)
		}
	}

	result := Merge(contacts, pairs)
	if len(result.Merged) != 0 {
		t.Errorf("%d contacts auto-merged or left distinct without review; every namesake should be paired", len(result.Merged))
	}
	if len(result.Clusters) != 3 {
		t.Fatalf("got %d clusters, want 3 pairs", len(result.Clusters))
	}
	for _, c := range result.Clusters {
		if len(c.Indices) != 2 {
			t.Errorf("cluster %v has %d members; a near-name-only cluster must be a pair", c.Indices, len(c.Indices))
		}
		if contacts[c.Indices[0]].Parsed.Source == contacts[c.Indices[1]].Parsed.Source {
			t.Errorf("cluster %v pairs two contacts from the same source when a cross-source partner was available", c.Indices)
		}
	}
	if len(result.Review) != 6 {
		t.Errorf("review has %d contacts, want 6", len(result.Review))
	}
}

// A third namesake with no ties must not be stacked onto a confirmed pair:
// the confirmed pair auto-merges as before and the namesake stays distinct.
func TestMergeNearNameOnlyEdgeDoesNotJoinConfirmedCluster(t *testing.T) {
	contacts := []model.NormalizedContact{
		normalize.Contact(model.ParsedContact{Source: model.SourceICloud, GivenName: "Maria", FamilyName: "Rodriguez",
			Phones: []model.Phone{{Number: "3125550100"}}}),
		normalize.Contact(model.ParsedContact{Source: model.SourceGoogle, GivenName: "Maria", FamilyName: "Rodriguez",
			Phones: []model.Phone{{Number: "(312) 555-0100"}}}),
		namesake(model.SourceGoogle, "Maria", "Rodriguez", 2),
	}
	result := Merge(contacts, allPairs(contacts))

	if len(result.Clusters) != 1 || len(result.Clusters[0].Indices) != 2 {
		t.Fatalf("clusters = %+v, want exactly one two-member cluster", result.Clusters)
	}
	if len(result.Review) != 0 {
		t.Errorf("review has %d contacts, want 0: the namesake must not drag the confirmed pair into review", len(result.Review))
	}
	var merged, distinct int
	for _, mc := range result.Merged {
		if len(mc.MergedFrom) > 1 {
			merged++
		} else {
			distinct++
		}
	}
	if merged != 1 || distinct != 1 {
		t.Errorf("merged=%d distinct=%d, want 1 auto-merge and 1 distinct namesake", merged, distinct)
	}
}

// A near-name pair with a shared identifier is a confirmed edge and still
// chains: same name + shared phone on one side, shared email on the other.
func TestMergeConfirmedReviewEdgesStillChain(t *testing.T) {
	contacts := []model.NormalizedContact{
		normalize.Contact(model.ParsedContact{Source: model.SourceICloud, GivenName: "Eric", FamilyName: "Johnson",
			Phones: []model.Phone{{Number: "3175551212"}}}),
		normalize.Contact(model.ParsedContact{Source: model.SourceGoogle, GivenName: "Erica", FamilyName: "Johnson",
			Phones: []model.Phone{{Number: "3175551212"}}, Emails: []model.Email{{Address: "ej@example.com"}}}),
		normalize.Contact(model.ParsedContact{Source: model.SourceGoogle, GivenName: "Erika", FamilyName: "Johnson",
			Emails: []model.Email{{Address: "ej@example.com"}}}),
	}
	result := Merge(contacts, allPairs(contacts))
	if len(result.Clusters) != 1 || len(result.Clusters[0].Indices) != 3 {
		t.Fatalf("clusters = %+v, want one three-member cluster linked by phone and email", result.Clusters)
	}
}

// Two unrelated "Alex" pairs, different phones, used to hash to the same
// cluster id, and the review TUI then wrote one decision onto both.
func TestClusterIDIsUniquePerCluster(t *testing.T) {
	mk := func(src model.Source, phone string) model.NormalizedContact {
		return normalize.Contact(model.ParsedContact{Source: src, GivenName: "Alex",
			Phones: []model.Phone{{Number: phone}}})
	}
	contacts := []model.NormalizedContact{
		mk(model.SourceICloud, "2125550101"), mk(model.SourceGoogle, "2125550101"),
		mk(model.SourceICloud, "2125550202"), mk(model.SourceGoogle, "2125550202"),
	}
	a := ClusterID(contacts, []int{0, 1})
	b := ClusterID(contacts, []int{2, 3})
	if a == b {
		t.Fatalf("ClusterID collision: both clusters hash to %s", a)
	}
	if ClusterID(contacts, []int{1, 0}) != a {
		t.Error("ClusterID depends on member order")
	}
}
