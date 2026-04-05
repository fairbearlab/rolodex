package merger

import (
	"testing"

	"github.com/fairbearlabs/rolodex/internal/model"
)

func TestMergeAutoMergePair(t *testing.T) {
	contacts := []model.NormalizedContact{
		{
			Parsed: model.ParsedContact{
				Source:        model.SourceICloud,
				GivenName:     "Robert",
				FamilyName:    "Smith",
				FormattedName: "Robert Smith",
				Emails:        []model.Email{{Address: "bob@gmail.com", Type: "HOME"}},
				Phones:        []model.Phone{{Number: "+1 (555) 123-4567", Type: "CELL"}},
				Org:           "Acme Corp",
				Title:         "Engineer",
			},
		},
		{
			Parsed: model.ParsedContact{
				Source:        model.SourceGoogle,
				GivenName:     "Bob",
				FamilyName:    "Smith",
				FormattedName: "Bob Smith",
				Emails:        []model.Email{
					{Address: "bob@gmail.com", Type: "HOME"},
					{Address: "robert.smith@acme.com", Type: "WORK"},
				},
				Org:   "Acme Corp",
				Title: "Senior Engineer",
			},
		},
	}

	pairs := []model.ScoredPair{
		{A: 0, B: 1, Score: 0.95, Tier: model.TierAutoMerge},
	}

	result := Merge(contacts, pairs)

	// Should produce 1 merged contact
	mergedCount := 0
	for _, mc := range result.Merged {
		if len(mc.MergedFrom) > 1 {
			mergedCount++
		}
	}
	if mergedCount != 1 {
		t.Fatalf("expected 1 auto-merged, got %d", mergedCount)
	}

	// iCloud values should win for single-value fields
	merged := result.Merged[0]
	if merged.Contact.FormattedName != "Robert Smith" {
		t.Errorf("FN = %q, want 'Robert Smith' (iCloud priority)", merged.Contact.FormattedName)
	}
	if merged.Contact.Title != "Engineer" {
		t.Errorf("title = %q, want 'Engineer' (iCloud priority)", merged.Contact.Title)
	}

	// Should have union of emails
	if len(merged.Contact.Emails) != 2 {
		t.Errorf("expected 2 emails (union), got %d", len(merged.Contact.Emails))
	}
}

func TestMergeReviewPair(t *testing.T) {
	contacts := []model.NormalizedContact{
		{Parsed: model.ParsedContact{Source: model.SourceICloud, GivenName: "Alice", FamilyName: "Smith"}},
		{Parsed: model.ParsedContact{Source: model.SourceGoogle, GivenName: "Alicia", FamilyName: "Smith"}},
	}

	pairs := []model.ScoredPair{
		{A: 0, B: 1, Score: 0.72, Tier: model.TierReview},
	}

	result := Merge(contacts, pairs)

	if len(result.Review) != 2 {
		t.Fatalf("expected 2 review contacts (both versions), got %d", len(result.Review))
	}
	for _, r := range result.Review {
		if !r.ReviewFlag {
			t.Error("review contacts should have ReviewFlag=true")
		}
	}
}

func TestMergeDistinct(t *testing.T) {
	contacts := []model.NormalizedContact{
		{Parsed: model.ParsedContact{Source: model.SourceICloud, GivenName: "Alice"}},
		{Parsed: model.ParsedContact{Source: model.SourceGoogle, GivenName: "Bob"}},
	}

	// No pairs — both are distinct
	result := Merge(contacts, nil)

	if len(result.Merged) != 2 {
		t.Fatalf("expected 2 distinct contacts, got %d", len(result.Merged))
	}
}

func TestMergePassthroughFields(t *testing.T) {
	contacts := []model.NormalizedContact{
		{
			Parsed: model.ParsedContact{
				Source:    model.SourceICloud,
				GivenName: "Alice",
				FamilyName: "Smith",
			},
		},
		{
			Parsed: model.ParsedContact{
				Source:    model.SourceGoogle,
				GivenName: "Alice",
				FamilyName: "Smith",
				Org:       "NewCo", // only in Google
				Birthday:  "1990-01-01", // only in Google
			},
		},
	}

	pairs := []model.ScoredPair{
		{A: 0, B: 1, Score: 0.90, Tier: model.TierAutoMerge},
	}

	result := Merge(contacts, pairs)
	merged := result.Merged[0]

	// Passthrough: fields only in Google should be kept
	if merged.Contact.Org != "NewCo" {
		t.Errorf("org = %q, want 'NewCo' (passthrough from Google)", merged.Contact.Org)
	}
	if merged.Contact.Birthday != "1990-01-01" {
		t.Errorf("bday = %q, want '1990-01-01' (passthrough from Google)", merged.Contact.Birthday)
	}
}

func TestUnionFind(t *testing.T) {
	uf := newUnionFind(5)
	uf.union(0, 1)
	uf.union(2, 3)
	uf.union(1, 3) // connects {0,1} and {2,3}

	if uf.find(0) != uf.find(3) {
		t.Error("0 and 3 should be in same cluster")
	}
	if uf.find(0) == uf.find(4) {
		t.Error("0 and 4 should be in different clusters")
	}

	clusters := uf.clusters()
	if len(clusters) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(clusters))
	}
}
