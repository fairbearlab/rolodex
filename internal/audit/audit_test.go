package audit

import (
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

func TestAudit_EmailOnly(t *testing.T) {
	contacts := []model.ParsedContact{
		{FormattedName: "Has Email", Emails: []model.Email{{Address: "a@b.com"}}},
	}
	result := Audit(contacts, AuditOptions{})
	if result.UnreachableCount != 0 {
		t.Errorf("contact with email should not be flagged, got %d unreachable", result.UnreachableCount)
	}
}

func TestAudit_PhoneOnly(t *testing.T) {
	contacts := []model.ParsedContact{
		{FormattedName: "Has Phone", Phones: []model.Phone{{Number: "555-1234"}}},
	}
	result := Audit(contacts, AuditOptions{})
	if result.UnreachableCount != 0 {
		t.Errorf("contact with phone should not be flagged, got %d unreachable", result.UnreachableCount)
	}
}

func TestAudit_Both(t *testing.T) {
	contacts := []model.ParsedContact{
		{
			FormattedName: "Has Both",
			Emails:        []model.Email{{Address: "a@b.com"}},
			Phones:        []model.Phone{{Number: "555-1234"}},
		},
	}
	result := Audit(contacts, AuditOptions{})
	if result.UnreachableCount != 0 {
		t.Errorf("contact with both should not be flagged, got %d unreachable", result.UnreachableCount)
	}
}

func TestAudit_Neither(t *testing.T) {
	contacts := []model.ParsedContact{
		{FormattedName: "No Contact Info"},
	}
	// Default: names-only contacts are skipped (low-signal noise)
	result := Audit(contacts, AuditOptions{})
	if result.UnreachableCount != 0 {
		t.Errorf("names-only contact should not be flagged by default, got %d unreachable", result.UnreachableCount)
	}
	// With IncludeNamesOnly: names-only contacts are included
	result = Audit(contacts, AuditOptions{IncludeNamesOnly: true})
	if result.UnreachableCount != 1 {
		t.Errorf("names-only contact should be flagged with IncludeNamesOnly, got %d unreachable", result.UnreachableCount)
	}
	if result.Unreachable[0].Name != "No Contact Info" {
		t.Errorf("name = %q, want %q", result.Unreachable[0].Name, "No Contact Info")
	}
}

func TestAudit_OrgButNoContact(t *testing.T) {
	contacts := []model.ParsedContact{
		{FormattedName: "Has Org", Org: "Acme Corp"},
	}
	result := Audit(contacts, AuditOptions{})
	if result.UnreachableCount != 1 {
		t.Errorf("contact with org but no email/phone should be flagged, got %d unreachable", result.UnreachableCount)
	}
	if !result.Unreachable[0].HasOrg {
		t.Error("expected HasOrg = true")
	}
}

func TestAudit_Empty(t *testing.T) {
	result := Audit(nil, AuditOptions{})
	if result.Total != 0 {
		t.Errorf("total = %d, want 0", result.Total)
	}
	if result.UnreachableCount != 0 {
		t.Errorf("unreachable = %d, want 0", result.UnreachableCount)
	}
}

func TestAudit_Mixed(t *testing.T) {
	contacts := []model.ParsedContact{
		{FormattedName: "Reachable", Emails: []model.Email{{Address: "a@b.com"}}},
		{FormattedName: "Names Only"}, // skipped by default
		{FormattedName: "Also Reachable", Phones: []model.Phone{{Number: "555"}}},
		{FormattedName: "Has Org No Contact", Org: "Corp"}, // flagged
	}
	result := Audit(contacts, AuditOptions{})
	if result.Total != 4 {
		t.Errorf("total = %d, want 4", result.Total)
	}
	if result.UnreachableCount != 1 {
		t.Errorf("unreachable = %d, want 1 (names-only skipped by default)", result.UnreachableCount)
	}
	// With IncludeNamesOnly, both unreachable contacts are flagged
	result = Audit(contacts, AuditOptions{IncludeNamesOnly: true})
	if result.UnreachableCount != 2 {
		t.Errorf("unreachable with IncludeNamesOnly = %d, want 2", result.UnreachableCount)
	}
}

func TestAudit_IndexTracking(t *testing.T) {
	contacts := []model.ParsedContact{
		{FormattedName: "OK", Emails: []model.Email{{Address: "a@b.com"}}},
		{FormattedName: "Bad"},
		{FormattedName: "OK2", Phones: []model.Phone{{Number: "555"}}},
		{FormattedName: "Bad2"},
	}
	result := Audit(contacts, AuditOptions{IncludeNamesOnly: true})
	if len(result.Unreachable) != 2 {
		t.Fatalf("expected 2 unreachable, got %d", len(result.Unreachable))
	}
	if result.Unreachable[0].Index != 1 {
		t.Errorf("first unreachable index = %d, want 1", result.Unreachable[0].Index)
	}
	if result.Unreachable[1].Index != 3 {
		t.Errorf("second unreachable index = %d, want 3", result.Unreachable[1].Index)
	}
}

func TestAudit_ContactNameFallback(t *testing.T) {
	tests := []struct {
		contact model.ParsedContact
		want    string
	}{
		{model.ParsedContact{FormattedName: "Alice"}, "Alice"},
		{model.ParsedContact{GivenName: "Bob", FamilyName: "Smith"}, "Bob Smith"},
		{model.ParsedContact{FamilyName: "Jones"}, "Jones"},
		{model.ParsedContact{Org: "Acme"}, "Acme"},
		{model.ParsedContact{}, "(unknown)"},
	}
	for _, tt := range tests {
		result := Audit([]model.ParsedContact{tt.contact}, AuditOptions{IncludeNamesOnly: true})
		if result.UnreachableCount != 1 {
			t.Fatalf("expected 1 unreachable for %+v", tt.contact)
		}
		if result.Unreachable[0].Name != tt.want {
			t.Errorf("name = %q, want %q", result.Unreachable[0].Name, tt.want)
		}
	}
}

func TestAudit_HasAddress(t *testing.T) {
	contacts := []model.ParsedContact{
		{
			FormattedName: "Address Only",
			Addresses:     []model.Address{{Street: "123 Main St"}},
		},
	}
	result := Audit(contacts, AuditOptions{})
	if result.UnreachableCount != 1 {
		t.Fatalf("expected 1 unreachable, got %d", result.UnreachableCount)
	}
	if !result.Unreachable[0].HasAddress {
		t.Error("expected HasAddress = true")
	}
}
