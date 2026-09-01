package review

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

// writeRunFixture writes a report.json carrying the given review decisions
// and a review.vcf with contactCount minimal cards, mirroring what `merge`
// produces. It intentionally lets the caller mismatch the two, the same way
// a stale report.json or a hand-edited review.vcf would in the field.
func writeRunFixture(t *testing.T, dir string, review []model.ReviewDecision, contactCount int) (reportPath, reviewPath string) {
	t.Helper()
	report := model.Report{Review: review}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	reportPath = filepath.Join(dir, "report.json")
	if err := os.WriteFile(reportPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	var vcf strings.Builder
	for i := 0; i < contactCount; i++ {
		fmt.Fprintf(&vcf, "BEGIN:VCARD\nVERSION:3.0\nFN:Contact %d\nN:Contact;%d;;;\nEND:VCARD\n", i, i)
	}
	reviewPath = filepath.Join(dir, "review.vcf")
	if err := os.WriteFile(reviewPath, []byte(vcf.String()), 0600); err != nil {
		t.Fatal(err)
	}
	return reportPath, reviewPath
}

// TestRunRejectsMismatchedReviewFile drives the "mismatched review.vcf is
// fatal" hardening through the real entry point, not just BuildClusters
// directly: Run must surface the error before ever reaching the TUI.
func TestRunRejectsMismatchedReviewFile(t *testing.T) {
	dir := t.TempDir()
	review := []model.ReviewDecision{{
		ClusterID: "c0", Score: 0.7, Decision: "pending",
		Contacts: []model.ContactRef{{Source: model.SourceICloud}, {Source: model.SourceGoogle}},
	}}
	// report.json wants 2 review contacts; review.vcf only has 1.
	reportPath, reviewPath := writeRunFixture(t, dir, review, 1)

	done, err := Run(reportPath, reviewPath, filepath.Join(dir, "cal.jsonl"))
	if err == nil {
		t.Fatal("expected an error for a short review.vcf, got nil")
	}
	if done {
		t.Error("done = true on a fatal mismatch, want false")
	}
}

// TestRunNothingToReview covers the empty-report early return: no TUI is
// launched and Run reports done=true.
func TestRunNothingToReview(t *testing.T) {
	dir := t.TempDir()
	reportPath, reviewPath := writeRunFixture(t, dir, nil, 0)

	done, err := Run(reportPath, reviewPath, filepath.Join(dir, "cal.jsonl"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !done {
		t.Error("done = false when report.Review is empty, want true")
	}
}

// TestRunAllClustersAlreadyResolved covers the pending==0 early return: a
// report.json that was already fully decided (e.g. resolved once, review
// re-run by mistake) must not launch the TUI either.
func TestRunAllClustersAlreadyResolved(t *testing.T) {
	dir := t.TempDir()
	review := []model.ReviewDecision{{
		ClusterID: "c0", Score: 0.7, Decision: "merge",
		Contacts: []model.ContactRef{{Source: model.SourceICloud}, {Source: model.SourceGoogle}},
	}}
	reportPath, reviewPath := writeRunFixture(t, dir, review, 2)

	done, err := Run(reportPath, reviewPath, filepath.Join(dir, "cal.jsonl"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !done {
		t.Error("done = false when every cluster is already resolved, want true")
	}
}

// TestRunMissingFiles covers the LoadReportAndReview error path.
func TestRunMissingFiles(t *testing.T) {
	_, err := Run("/nonexistent/report.json", "/nonexistent/review.vcf", "")
	if err == nil {
		t.Fatal("expected an error for missing report/review files, got nil")
	}
}

// A report aimed at a directory that does not exist must error out of
// writeReport, not vanish.
func TestWriteReportParentDirMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "report.json")
	err := writeReport(path, map[string]string{"k": "v"})
	if err == nil || !strings.Contains(err.Error(), "writing report") {
		t.Errorf("err = %v, want a writing-report error", err)
	}
}
