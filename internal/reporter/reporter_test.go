package reporter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fairbearlab/rolodex/internal/merger"
	"github.com/fairbearlab/rolodex/internal/model"
)

func TestGenerateAutoMergeCluster(t *testing.T) {
	contacts := []model.NormalizedContact{
		{Parsed: model.ParsedContact{Source: model.SourceICloud, GivenName: "Alice", FamilyName: "Smith", FormattedName: "Alice Smith"}},
		{Parsed: model.ParsedContact{Source: model.SourceGoogle, GivenName: "Alice", FamilyName: "Smith", FormattedName: "Alice Smith"}},
	}

	result := merger.Result{
		Merged: []model.MergedContact{
			{
				Contact:    contacts[0].Parsed,
				Sources:    []model.Source{model.SourceICloud, model.SourceGoogle},
				Score:      0.95,
				MergedFrom: []int{0, 1},
			},
		},
		Clusters: []model.Cluster{
			{
				Indices: []int{0, 1},
				Pairs:   []model.ScoredPair{{A: 0, B: 1, Score: 0.95, Tier: model.TierAutoMerge}},
			},
		},
	}

	report := Generate(contacts, result, 1, 1, nil)

	if report.Summary.AutoMerged != 1 {
		t.Errorf("auto_merged = %d, want 1", report.Summary.AutoMerged)
	}
	if len(report.Merged) != 1 {
		t.Fatalf("expected 1 merge decision, got %d", len(report.Merged))
	}
	if report.Merged[0].Score != 0.95 {
		t.Errorf("merge score = %.2f, want 0.95", report.Merged[0].Score)
	}
	if report.Merged[0].ResultName != "Alice Smith" {
		t.Errorf("result_name = %q, want 'Alice Smith'", report.Merged[0].ResultName)
	}
}

func TestGenerateReviewCluster(t *testing.T) {
	contacts := []model.NormalizedContact{
		{Parsed: model.ParsedContact{Source: model.SourceICloud, GivenName: "Alice", FamilyName: "Smith", FormattedName: "Alice Smith"}},
		{Parsed: model.ParsedContact{Source: model.SourceGoogle, GivenName: "Alicia", FamilyName: "Smith", FormattedName: "Alicia Smith"}},
	}

	result := merger.Result{
		Review: []model.MergedContact{
			{Contact: contacts[0].Parsed, Sources: []model.Source{model.SourceICloud}, Score: 0.72, MergedFrom: []int{0, 1}, ReviewFlag: true},
			{Contact: contacts[1].Parsed, Sources: []model.Source{model.SourceGoogle}, Score: 0.72, MergedFrom: []int{0, 1}, ReviewFlag: true},
		},
		Clusters: []model.Cluster{
			{
				Indices: []int{0, 1},
				Pairs:   []model.ScoredPair{{A: 0, B: 1, Score: 0.72, Tier: model.TierReview}},
			},
		},
	}

	report := Generate(contacts, result, 1, 1, nil)

	if report.Summary.ReviewCount != 1 {
		t.Errorf("review_count = %d, want 1", report.Summary.ReviewCount)
	}
	if len(report.Review) != 1 {
		t.Fatalf("expected 1 review decision, got %d", len(report.Review))
	}
	if report.Review[0].Decision != "pending" {
		t.Errorf("decision = %q, want 'pending'", report.Review[0].Decision)
	}
}

func TestGenerateDistinctOnly(t *testing.T) {
	contacts := []model.NormalizedContact{
		{Parsed: model.ParsedContact{Source: model.SourceICloud, GivenName: "Alice", FormattedName: "Alice"}},
		{Parsed: model.ParsedContact{Source: model.SourceGoogle, GivenName: "Bob", FormattedName: "Bob"}},
	}

	result := merger.Result{
		Merged: []model.MergedContact{
			{Contact: contacts[0].Parsed, Sources: []model.Source{model.SourceICloud}, MergedFrom: []int{0}},
			{Contact: contacts[1].Parsed, Sources: []model.Source{model.SourceGoogle}, MergedFrom: []int{1}},
		},
	}

	report := Generate(contacts, result, 1, 1, nil)

	if report.Summary.DistinctCount != 2 {
		t.Errorf("distinct_count = %d, want 2", report.Summary.DistinctCount)
	}
	if len(report.Distinct) != 2 {
		t.Fatalf("expected 2 distinct entries, got %d", len(report.Distinct))
	}
}

func TestContactNameFallbackChain(t *testing.T) {
	tests := []struct {
		name string
		c    model.ParsedContact
		want string
	}{
		{"formatted name", model.ParsedContact{FormattedName: "Alice Smith"}, "Alice Smith"},
		{"given+family", model.ParsedContact{GivenName: "Alice", FamilyName: "Smith"}, "Alice Smith"},
		{"email fallback", model.ParsedContact{Emails: []model.Email{{Address: "alice@example.com"}}}, "alice@example.com"},
		{"phone fallback", model.ParsedContact{Phones: []model.Phone{{Number: "555-1234"}}}, "555-1234"},
		{"unknown fallback", model.ParsedContact{}, "(unknown)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contactName(tt.c)
			if got != tt.want {
				t.Errorf("contactName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindConflicts(t *testing.T) {
	contacts := []model.NormalizedContact{
		{Parsed: model.ParsedContact{
			Source: model.SourceICloud, FormattedName: "Robert Smith",
			Org: "Acme", Title: "Engineer",
		}},
		{Parsed: model.ParsedContact{
			Source: model.SourceGoogle, FormattedName: "Bob Smith",
			Org: "Acme Corp", Title: "Senior Engineer",
		}},
	}

	conflicts := findConflicts(contacts, []int{0, 1})
	if len(conflicts) < 2 {
		t.Fatalf("expected at least 2 conflicts (FN, TITLE), got %d", len(conflicts))
	}
	for _, c := range conflicts {
		if c.Winner != "icloud" {
			t.Errorf("expected winner=icloud, got %q for field %s", c.Winner, c.Field)
		}
	}
}

func TestWriteFileRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "report.json")

	report := model.Report{
		Summary: model.ReportSummary{ICloudTotal: 5, GoogleTotal: 3, AutoMerged: 2},
	}

	if err := WriteFile(path, report); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}

	var loaded model.Report
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshaling: %v", err)
	}

	if loaded.Summary.ICloudTotal != 5 {
		t.Errorf("icloud_total = %d, want 5", loaded.Summary.ICloudTotal)
	}
}

func TestGenerateWarnings(t *testing.T) {
	contacts := []model.NormalizedContact{}
	result := merger.Result{}
	warnings := []model.Warning{
		{Source: model.SourceICloud, Index: 0, Message: "malformed entry"},
	}

	report := Generate(contacts, result, 0, 0, warnings)

	if report.Summary.WarningCount != 1 {
		t.Errorf("warning_count = %d, want 1", report.Summary.WarningCount)
	}
	if len(report.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(report.Warnings))
	}
}
