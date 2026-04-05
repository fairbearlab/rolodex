package blocker

import (
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

func TestBlockByEmail(t *testing.T) {
	contacts := []model.NormalizedContact{
		{NormalizedEmails: []string{"shared@example.com"}, NormalizedFamilyName: "smith"},
		{NormalizedEmails: []string{"shared@example.com"}, NormalizedFamilyName: "jones"},
		{NormalizedEmails: []string{"other@example.com"}, NormalizedFamilyName: "brown"},
	}

	pairs := Block(contacts)

	// Should have a pair for contacts 0 and 1 (shared email)
	found := false
	for _, p := range pairs {
		if (p[0] == 0 && p[1] == 1) || (p[0] == 1 && p[1] == 0) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected pair (0,1) for shared email, got %v", pairs)
	}
}

func TestBlockByPhone(t *testing.T) {
	contacts := []model.NormalizedContact{
		{NormalizedPhones: []string{"5551234567"}, NormalizedFamilyName: "smith"},
		{NormalizedPhones: []string{"5551234567"}, NormalizedFamilyName: "jones"},
	}

	pairs := Block(contacts)
	if len(pairs) == 0 {
		t.Error("expected pairs for shared phone")
	}
}

func TestBlockByLastName(t *testing.T) {
	contacts := []model.NormalizedContact{
		{NormalizedFamilyName: "smith", NormalizedGivenName: "alice"},
		{NormalizedFamilyName: "smith", NormalizedGivenName: "bob"},
		{NormalizedFamilyName: "jones", NormalizedGivenName: "carol"},
	}

	pairs := Block(contacts)

	// Should pair 0,1 (same last name) but not 0,2 or 1,2
	foundSmith := false
	foundJones := false
	for _, p := range pairs {
		if (p[0] == 0 && p[1] == 1) || (p[0] == 1 && p[1] == 0) {
			foundSmith = true
		}
		if p[0] == 2 || p[1] == 2 {
			foundJones = true
		}
	}
	if !foundSmith {
		t.Error("expected pair for shared last name 'smith'")
	}
	if foundJones {
		t.Error("should not pair 'jones' with 'smith'")
	}
}

func TestBlockNoDuplicatePairs(t *testing.T) {
	// Two contacts sharing both email and phone should produce only one pair
	contacts := []model.NormalizedContact{
		{
			NormalizedEmails:     []string{"shared@example.com"},
			NormalizedPhones:     []string{"5551234567"},
			NormalizedFamilyName: "smith",
			NormalizedGivenName:  "alice",
		},
		{
			NormalizedEmails:     []string{"shared@example.com"},
			NormalizedPhones:     []string{"5551234567"},
			NormalizedFamilyName: "smith",
			NormalizedGivenName:  "bob",
		},
	}

	pairs := Block(contacts)
	if len(pairs) != 1 {
		t.Errorf("expected 1 unique pair, got %d: %v", len(pairs), pairs)
	}
}

func TestBlockWithinSource(t *testing.T) {
	// Two contacts from the same source should still be blocked
	contacts := []model.NormalizedContact{
		{
			Parsed:               model.ParsedContact{Source: model.SourceICloud},
			NormalizedEmails:     []string{"dupe@example.com"},
			NormalizedFamilyName: "smith",
		},
		{
			Parsed:               model.ParsedContact{Source: model.SourceICloud},
			NormalizedEmails:     []string{"dupe@example.com"},
			NormalizedFamilyName: "jones",
		},
	}

	pairs := Block(contacts)
	if len(pairs) == 0 {
		t.Error("expected within-source blocking to produce pairs")
	}
}
