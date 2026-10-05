package merger

import (
	"strconv"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/normalize"
)

// chain builds n contacts linked in a line by shared identifiers — each one
// shares a phone with the next — under near-identical names, so every link
// is a confirmed review edge and union-find joins them all.
func chain(n int) []model.NormalizedContact {
	contacts := make([]model.NormalizedContact, n)
	for i := range contacts {
		contacts[i] = normalize.Contact(model.ParsedContact{
			Source: model.SourceGoogle, GivenName: "Person" + strconv.Itoa(i), FamilyName: "Switchboard",
			Phones: []model.Phone{
				{Number: "21255" + strconv.Itoa(10000+i)},
				{Number: "21255" + strconv.Itoa(10000+i+1)},
			},
		})
	}
	return contacts
}

// 500 contacts sharing one TEL used to come out as ONE 501-member review
// cluster, shown as a single card whose one merge keystroke fused them all.
// A cluster over the cap is neither merged nor reviewed: its members are
// kept as separate people and the cluster is reported.
func TestMergeOversizedClusterIsKeptSeparate(t *testing.T) {
	n := MaxClusterSize + 1
	contacts := chain(n)
	result := Merge(contacts, allPairs(contacts))

	if len(result.Review) != 0 {
		t.Errorf("review has %d contacts, want 0: an oversized cluster must not become one card", len(result.Review))
	}
	if len(result.Merged) != n {
		t.Errorf("merged has %d contacts, want all %d kept separate", len(result.Merged), n)
	}
	for _, mc := range result.Merged {
		if len(mc.MergedFrom) != 1 {
			t.Errorf("contact %q merged from %v, want itself only", mc.Contact.FormattedName, mc.MergedFrom)
		}
	}
	if len(result.Oversized) != 1 || len(result.Oversized[0].Indices) != n {
		t.Errorf("oversized = %+v, want one cluster of %d", result.Oversized, n)
	}
	if len(result.Clusters) != 0 {
		t.Errorf("clusters = %d, want 0 (an oversized cluster is reported under Oversized)", len(result.Clusters))
	}
}

// A cluster at the cap is still reviewed as one card.
func TestMergeClusterAtCapIsReviewed(t *testing.T) {
	contacts := chain(MaxClusterSize)
	result := Merge(contacts, allPairs(contacts))
	if len(result.Oversized) != 0 {
		t.Errorf("oversized = %+v, want none", result.Oversized)
	}
	if len(result.Review) != MaxClusterSize {
		t.Errorf("review has %d contacts, want %d", len(result.Review), MaxClusterSize)
	}
}
