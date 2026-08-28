package merger

import (
	"strconv"
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
		Emails:        []model.Email{{Address: given + strconv.Itoa(i) + "@example.com"}},
		Phones:        []model.Phone{{Number: "212555010" + strconv.Itoa(i)}},
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

// TestMergePairMapHandlesReversedIndices covers the a > b swap when building
// the pair-index lookup: a caller (or a future blocker change) may hand
// Merge a ScoredPair with A > B, and the swap must still key it so the
// cluster-assembly loop (which always looks up with the smaller index
// first) finds it.
func TestMergePairMapHandlesReversedIndices(t *testing.T) {
	contacts := []model.NormalizedContact{
		normalize.Contact(model.ParsedContact{Source: model.SourceICloud, GivenName: "Priya", FamilyName: "Nair",
			Phones: []model.Phone{{Number: "3175550000"}}}),
		normalize.Contact(model.ParsedContact{Source: model.SourceGoogle, GivenName: "Priya", FamilyName: "Nair",
			Phones: []model.Phone{{Number: "(317) 555-0000"}}}),
	}
	// A=1, B=0: the reverse of index order.
	pairs := []model.ScoredPair{{A: 1, B: 0, Score: 1.0, Tier: model.TierAutoMerge,
		Features: model.ScoreFeatures{SharedPhone: true, NameExact: true}}}

	result := Merge(contacts, pairs)
	if len(result.Merged) != 1 {
		t.Fatalf("got %d merged contacts, want 1 (both contacts in one cluster): %+v", len(result.Merged), result.Merged)
	}
	if len(result.Merged[0].MergedFrom) != 2 {
		t.Errorf("MergedFrom = %v, want both indices merged", result.Merged[0].MergedFrom)
	}
	if len(result.Clusters) != 1 || len(result.Clusters[0].Pairs) != 1 {
		t.Errorf("clusters = %+v, want one cluster carrying the reversed pair", result.Clusters)
	}
}

// TestMergeNearNameOnlyPrefersHigherScoreOverCrossSource: the sort orders
// near-name-only edges by score first and cross-source only as a tie-break.
// Two same-source contacts with an exact name match (score 0.40) must pair
// before a cross-source pair with a lower-similarity name (score < 0.40),
// even though the cross-source pair would otherwise be preferred.
func TestMergeNearNameOnlyPrefersHigherScoreOverCrossSource(t *testing.T) {
	mk := func(src model.Source, given string, i int) model.NormalizedContact {
		return normalize.Contact(model.ParsedContact{
			Source: src, GivenName: given, FamilyName: "Lee",
			FormattedName: given + " Lee",
			Emails:        []model.Email{{Address: given + strconv.Itoa(i) + "@example.com"}},
			Phones:        []model.Phone{{Number: "212555010" + strconv.Itoa(i)}},
		})
	}
	// 0 and 1 share the exact same name (same source): highest possible
	// near-name score. 2 is cross-source but a lower-similarity spelling.
	contacts := []model.NormalizedContact{
		mk(model.SourceICloud, "David", 0),
		mk(model.SourceICloud, "David", 1),
		mk(model.SourceGoogle, "Davids", 2),
	}
	pairs := allPairs(contacts)
	for _, p := range pairs {
		if p.Tier != model.TierReview {
			t.Fatalf("pair %d-%d tier = %q, want review (test precondition)", p.A, p.B, p.Tier)
		}
	}
	// Confirm the precondition this test relies on: pair(0,1) must outscore
	// both cross-source pairs, or the test proves nothing about tie-breaking.
	scoreOf := func(a, b int) float64 {
		for _, p := range pairs {
			if (p.A == a && p.B == b) || (p.A == b && p.B == a) {
				return p.Score
			}
		}
		t.Fatalf("no scored pair for %d-%d", a, b)
		return 0
	}
	s01, s02, s12 := scoreOf(0, 1), scoreOf(0, 2), scoreOf(1, 2)
	if !(s01 > s02 && s01 > s12) {
		t.Fatalf("test precondition failed: score(0,1)=%.3f must exceed score(0,2)=%.3f and score(1,2)=%.3f", s01, s02, s12)
	}

	result := Merge(contacts, pairs)
	var paired [][2]int
	for _, c := range result.Clusters {
		if len(c.Indices) == 2 {
			paired = append(paired, [2]int{c.Indices[0], c.Indices[1]})
		}
	}
	if len(paired) != 1 || paired[0] != [2]int{0, 1} {
		t.Errorf("clusters = %+v, want exactly the (0,1) pair (highest score wins over the cross-source tie-break)", result.Clusters)
	}
	// Contact 2 has no partner left and must stay distinct, not merged.
	for _, mc := range result.Merged {
		if len(mc.MergedFrom) == 1 && mc.MergedFrom[0] == 2 {
			return
		}
	}
	t.Error("contact 2 (the lower-score partner) did not end up distinct")
}
