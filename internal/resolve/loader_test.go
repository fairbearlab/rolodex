package resolve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

func TestLoadReportAndReview(t *testing.T) {
	dir := t.TempDir()

	// Write a minimal report.json
	report := model.Report{
		Review: []model.ReviewDecision{
			{ClusterID: "test-1", Score: 0.72, Decision: "pending"},
		},
	}
	reportData, _ := json.MarshalIndent(report, "", "  ")
	reportPath := filepath.Join(dir, "report.json")
	_ = os.WriteFile(reportPath, reportData, 0600)

	// Write a minimal review.vcf
	reviewPath := filepath.Join(dir, "review.vcf")
	vcf := "BEGIN:VCARD\nVERSION:3.0\nFN:Alice Test\nN:Test;Alice;;;\nEND:VCARD\n"
	_ = os.WriteFile(reviewPath, []byte(vcf), 0600)

	loaded, err := LoadReportAndReview(reportPath, reviewPath)
	if err != nil {
		t.Fatalf("LoadReportAndReview: %v", err)
	}

	if len(loaded.Report.Review) != 1 {
		t.Errorf("expected 1 review decision, got %d", len(loaded.Report.Review))
	}
	if loaded.Report.Review[0].ClusterID != "test-1" {
		t.Errorf("cluster ID = %q, want %q", loaded.Report.Review[0].ClusterID, "test-1")
	}
	if len(loaded.ReviewContacts) != 1 {
		t.Errorf("expected 1 review contact, got %d", len(loaded.ReviewContacts))
	}
}

func TestLoadReportAndReviewBadReport(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.json")
	_ = os.WriteFile(reportPath, []byte("not json"), 0600)

	reviewPath := filepath.Join(dir, "review.vcf")
	_ = os.WriteFile(reviewPath, []byte("BEGIN:VCARD\nVERSION:3.0\nFN:Test\nEND:VCARD\n"), 0600)

	_, err := LoadReportAndReview(reportPath, reviewPath)
	if err == nil {
		t.Fatal("expected error for invalid JSON report")
	}
}

func TestLoadReportAndReviewMissingFile(t *testing.T) {
	_, err := LoadReportAndReview("/nonexistent/report.json", "/nonexistent/review.vcf")
	if err == nil {
		t.Fatal("expected error for missing files")
	}
}

func TestLoadReportAndReviewRoundTripsSource(t *testing.T) {
	dir := t.TempDir()
	report := model.Report{Review: []model.ReviewDecision{{
		ClusterID: "c1", Score: 0.65, Decision: "pending",
		Contacts: []model.ContactRef{{Source: model.SourceICloud}, {Source: model.SourceGoogle}},
	}}}
	reportData, _ := json.Marshal(report)
	reportPath := filepath.Join(dir, "report.json")
	_ = os.WriteFile(reportPath, reportData, 0600)

	reviewPath := filepath.Join(dir, "review.vcf")
	vcf := "BEGIN:VCARD\nVERSION:3.0\nFN:A\nN:A;;;;\nX-ROLODEX-SOURCE:icloud\nEND:VCARD\n" +
		"BEGIN:VCARD\nVERSION:3.0\nFN:A\nN:A;;;;\nX-ROLODEX-SOURCE:google\nEND:VCARD\n"
	_ = os.WriteFile(reviewPath, []byte(vcf), 0600)

	loaded, err := LoadReportAndReview(reportPath, reviewPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.ReviewContacts) != 2 {
		t.Fatalf("expected 2 contacts, got %d", len(loaded.ReviewContacts))
	}
	if loaded.ReviewContacts[0].Source != model.SourceICloud || loaded.ReviewContacts[1].Source != model.SourceGoogle {
		t.Errorf("sources = %q, %q; want icloud, google",
			loaded.ReviewContacts[0].Source, loaded.ReviewContacts[1].Source)
	}
}
