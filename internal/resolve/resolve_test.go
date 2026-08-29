package resolve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/writer"
)

// writeTestVCF writes MergedContacts to a .vcf file for test input.
func writeTestVCF(t *testing.T, path string, contacts []model.MergedContact) {
	t.Helper()
	if err := writer.WriteFile(path, contacts); err != nil {
		t.Fatalf("writing test vcf %s: %v", path, err)
	}
}

func writeTestReport(t *testing.T, path string, report model.Report) {
	t.Helper()
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshaling test report: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("writing test report: %v", err)
	}
}

func TestRunMergeDecision(t *testing.T) {
	tmpDir := t.TempDir()
	reportPath := filepath.Join(tmpDir, "report.json")
	reviewPath := filepath.Join(tmpDir, "review.vcf")
	mergedPath := filepath.Join(tmpDir, "merged.vcf")
	outPath := filepath.Join(tmpDir, "final.vcf")

	// Write a merged.vcf with one contact
	writeTestVCF(t, mergedPath, []model.MergedContact{
		{
			Contact: model.ParsedContact{
				GivenName: "Existing", FamilyName: "Contact", FormattedName: "Existing Contact",
			},
			Sources: []model.Source{model.SourceICloud},
		},
	})

	// Write review.vcf with two contacts in one cluster
	clusterID := "test-cluster-1"
	writeTestVCF(t, reviewPath, []model.MergedContact{
		{
			Contact: model.ParsedContact{
				GivenName: "Alice", FamilyName: "Smith", FormattedName: "Alice Smith",
				Emails: []model.Email{{Address: "alice@example.com", Type: "HOME"}},
				Extra:  map[string][]string{"X-ROLODEX-CLUSTER": {clusterID}},
			},
			Sources:    []model.Source{model.SourceICloud},
			ReviewFlag: true,
			Score:      0.72,
		},
		{
			Contact: model.ParsedContact{
				GivenName: "Alicia", FamilyName: "Smith", FormattedName: "Alicia Smith",
				Emails: []model.Email{{Address: "alicia@example.com", Type: "HOME"}},
				Extra:  map[string][]string{"X-ROLODEX-CLUSTER": {clusterID}},
			},
			Sources:    []model.Source{model.SourceGoogle},
			ReviewFlag: true,
			Score:      0.72,
		},
	})

	// Write report with decision = "merge"
	writeTestReport(t, reportPath, model.Report{
		Review: []model.ReviewDecision{
			{
				ClusterID: clusterID,
				Score:     0.72,
				Contacts: []model.ContactRef{
					{Source: model.SourceICloud, Name: "Alice Smith", Index: 0},
					{Source: model.SourceGoogle, Name: "Alicia Smith", Index: 1},
				},
				Decision: "merge",
			},
		},
	})

	if err := Run(reportPath, reviewPath, mergedPath, outPath); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("output file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("output file is empty")
	}
}

func TestRunSkipDecision(t *testing.T) {
	tmpDir := t.TempDir()
	reportPath := filepath.Join(tmpDir, "report.json")
	reviewPath := filepath.Join(tmpDir, "review.vcf")
	mergedPath := filepath.Join(tmpDir, "merged.vcf")
	outPath := filepath.Join(tmpDir, "final.vcf")

	writeTestVCF(t, mergedPath, []model.MergedContact{
		{
			Contact: model.ParsedContact{FormattedName: "Keep Me"},
			Sources: []model.Source{model.SourceICloud},
		},
	})

	clusterID := "skip-cluster"
	writeTestVCF(t, reviewPath, []model.MergedContact{
		{
			Contact: model.ParsedContact{
				FormattedName: "Skip A",
				Extra:         map[string][]string{"X-ROLODEX-CLUSTER": {clusterID}},
			},
			Sources: []model.Source{model.SourceICloud}, ReviewFlag: true,
		},
		{
			Contact: model.ParsedContact{
				FormattedName: "Skip B",
				Extra:         map[string][]string{"X-ROLODEX-CLUSTER": {clusterID}},
			},
			Sources: []model.Source{model.SourceGoogle}, ReviewFlag: true,
		},
	})

	writeTestReport(t, reportPath, model.Report{
		Review: []model.ReviewDecision{
			{
				ClusterID: clusterID,
				Contacts:  []model.ContactRef{{}, {}},
				Decision:  "skip",
			},
		},
	})

	if err := Run(reportPath, reviewPath, mergedPath, outPath); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Clean(outPath))
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "Skip A") || strings.Contains(content, "Skip B") {
		t.Error("skipped contacts should not appear in output")
	}
	if !strings.Contains(content, "Keep Me") {
		t.Error("merged contacts should still appear in output")
	}
}

func TestRunPendingKeepsContacts(t *testing.T) {
	tmpDir := t.TempDir()
	reportPath := filepath.Join(tmpDir, "report.json")
	reviewPath := filepath.Join(tmpDir, "review.vcf")
	mergedPath := filepath.Join(tmpDir, "merged.vcf")
	outPath := filepath.Join(tmpDir, "final.vcf")

	writeTestVCF(t, mergedPath, nil)

	clusterID := "pending-cluster"
	writeTestVCF(t, reviewPath, []model.MergedContact{
		{
			Contact: model.ParsedContact{
				FormattedName: "Pending Contact",
				Extra:         map[string][]string{"X-ROLODEX-CLUSTER": {clusterID}},
			},
			Sources: []model.Source{model.SourceICloud}, ReviewFlag: true,
		},
	})

	writeTestReport(t, reportPath, model.Report{
		Review: []model.ReviewDecision{
			{
				ClusterID: clusterID,
				Contacts:  []model.ContactRef{{}},
				Decision:  "pending",
			},
		},
	})

	if err := Run(reportPath, reviewPath, mergedPath, outPath); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Clean(outPath))
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	if !strings.Contains(string(data), "Pending Contact") {
		t.Error("pending contacts should be preserved in output")
	}
}

func TestRunMismatchedReviewCount(t *testing.T) {
	tmpDir := t.TempDir()
	reportPath := filepath.Join(tmpDir, "report.json")
	reviewPath := filepath.Join(tmpDir, "review.vcf")
	mergedPath := filepath.Join(tmpDir, "merged.vcf")
	outPath := filepath.Join(tmpDir, "final.vcf")

	writeTestVCF(t, mergedPath, nil)
	writeTestVCF(t, reviewPath, []model.MergedContact{
		{Contact: model.ParsedContact{FormattedName: "Only One"}, Sources: []model.Source{model.SourceICloud}},
	})

	// Report references 3 contacts but review.vcf has 1
	writeTestReport(t, reportPath, model.Report{
		Review: []model.ReviewDecision{
			{Contacts: []model.ContactRef{{}, {}, {}}, Decision: "merge"},
		},
	})

	err := Run(reportPath, reviewPath, mergedPath, outPath)
	if err == nil {
		t.Fatal("expected error for mismatched review count")
	}
	if !strings.Contains(err.Error(), "more review contacts than exist") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunMalformedReportJSON(t *testing.T) {
	tmpDir := t.TempDir()
	reportPath := filepath.Join(tmpDir, "report.json")
	reviewPath := filepath.Join(tmpDir, "review.vcf")
	mergedPath := filepath.Join(tmpDir, "merged.vcf")
	outPath := filepath.Join(tmpDir, "final.vcf")

	writeTestVCF(t, mergedPath, nil)
	writeTestVCF(t, reviewPath, nil)
	_ = os.WriteFile(reportPath, []byte("{invalid json"), 0600)

	err := Run(reportPath, reviewPath, mergedPath, outPath)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "parsing report") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunClusterIDMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	reportPath := filepath.Join(tmpDir, "report.json")
	reviewPath := filepath.Join(tmpDir, "review.vcf")
	mergedPath := filepath.Join(tmpDir, "merged.vcf")
	outPath := filepath.Join(tmpDir, "final.vcf")

	writeTestVCF(t, mergedPath, nil)

	// review.vcf has cluster-A, but report expects cluster-B
	writeTestVCF(t, reviewPath, []model.MergedContact{
		{
			Contact: model.ParsedContact{
				FormattedName: "Test",
				Extra:         map[string][]string{"X-ROLODEX-CLUSTER": {"cluster-A"}},
			},
			Sources: []model.Source{model.SourceICloud},
		},
	})

	writeTestReport(t, reportPath, model.Report{
		Review: []model.ReviewDecision{
			{
				ClusterID: "cluster-B",
				Contacts:  []model.ContactRef{{}},
				Decision:  "merge",
			},
		},
	})

	err := Run(reportPath, reviewPath, mergedPath, outPath)
	if err == nil {
		t.Fatal("expected error for cluster ID mismatch")
	}
	if !strings.Contains(err.Error(), "cluster ID mismatch") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestMergeReviewClusterPrefersFullBirthdayOverYearless mirrors the merger's
// rule for clusters merged by a reviewer's decision: iCloud's yearless
// `--10-22` must not win over Google's `1989-10-22`.
func TestMergeReviewClusterPrefersFullBirthdayOverYearless(t *testing.T) {
	contacts := []model.ParsedContact{
		{Source: model.SourceICloud, GivenName: "Jane", FamilyName: "Doe", Birthday: "--10-22",
			Extra: map[string][]string{"X-ROLODEX-SOURCE": {"icloud"}}},
		{Source: model.SourceGoogle, GivenName: "Jane", FamilyName: "Doe", Birthday: "1989-10-22",
			Extra: map[string][]string{"X-ROLODEX-SOURCE": {"google"}}},
	}
	got := mergeReviewCluster(contacts).Contact.Birthday
	if got != "1989-10-22" {
		t.Errorf("merged birthday = %q, want the full date 1989-10-22", got)
	}
}

// A review cluster merged by hand keeps one UID, the iCloud card's, as the
// automatic merger does.
func TestMergeReviewClusterKeepsOneUID(t *testing.T) {
	contacts := []model.ParsedContact{
		{Source: model.SourceGoogle, FormattedName: "A", Extra: map[string][]string{"UID": {"google-uid"}}},
		{Source: model.SourceICloud, FormattedName: "A", Extra: map[string][]string{"UID": {"icloud-uid"}}},
	}
	if got := mergeReviewCluster(contacts).Contact.Extra["UID"]; len(got) != 1 || got[0] != "icloud-uid" {
		t.Errorf("UID = %v, want [icloud-uid]", got)
	}
}
