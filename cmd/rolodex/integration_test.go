package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fairbearlab/rolodex/internal/audit"
	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/parser"
)

func TestFullPipeline(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "merged.vcf")
	reviewPath := filepath.Join(tmpDir, "review.vcf")
	reportPath := filepath.Join(tmpDir, "report.json")

	err := merge("../../testdata/icloud.vcf", "../../testdata/google.vcf",
		outPath, reviewPath, reportPath)
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	// Check merged.vcf exists and is non-empty
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("merged.vcf not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("merged.vcf is empty")
	}

	// Check report.json
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("report.json not found: %v", err)
	}

	var report model.Report
	if err := json.Unmarshal(reportData, &report); err != nil {
		t.Fatalf("report.json invalid JSON: %v", err)
	}

	if report.Summary.ICloudTotal != 5 {
		t.Errorf("icloud_total = %d, want 5", report.Summary.ICloudTotal)
	}
	if report.Summary.GoogleTotal != 5 {
		t.Errorf("google_total = %d, want 5", report.Summary.GoogleTotal)
	}

	// We expect some auto-merges (Alice Johnson, Wei Chen, Maria Garcia share email/phone)
	if report.Summary.AutoMerged == 0 {
		t.Error("expected at least one auto-merge")
	}

	t.Logf("Report: %d auto-merged, %d review, %d distinct",
		report.Summary.AutoMerged, report.Summary.ReviewCount, report.Summary.DistinctCount)
}

func TestEmptyInput(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an empty vcf file
	emptyPath := filepath.Join(tmpDir, "empty.vcf")
	if err := os.WriteFile(emptyPath, []byte(""), 0644); err != nil {
		t.Fatalf("failed to create empty vcf file: %v", err)
	}

	outPath := filepath.Join(tmpDir, "merged.vcf")
	reviewPath := filepath.Join(tmpDir, "review.vcf")

	err := merge(emptyPath, "../../testdata/google.vcf",
		outPath, reviewPath, "")
	if err != nil {
		t.Fatalf("merge with empty input failed: %v", err)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("merged.vcf not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("merged.vcf should contain Google contacts even with empty iCloud input")
	}
}

// --- run command tests ---

func TestRunPipeline(t *testing.T) {
	pr, err := runPipeline("../../testdata/icloud.vcf", "../../testdata/google.vcf")
	if err != nil {
		t.Fatalf("runPipeline failed: %v", err)
	}
	if pr.ICloudCount != 5 {
		t.Errorf("ICloudCount = %d, want 5", pr.ICloudCount)
	}
	if pr.GoogleCount != 5 {
		t.Errorf("GoogleCount = %d, want 5", pr.GoogleCount)
	}
	if pr.AutoCount == 0 {
		t.Error("expected at least one auto-merge pair")
	}
	if len(pr.MergeResult.Merged) == 0 {
		t.Error("expected merged contacts")
	}
}

func TestRunNoReviewFastPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two VCFs with no overlapping contacts (no review pairs)
	icloudVCF := filepath.Join(tmpDir, "icloud.vcf")
	googleVCF := filepath.Join(tmpDir, "google.vcf")
	if err := os.WriteFile(icloudVCF, []byte(`BEGIN:VCARD
VERSION:3.0
N:Unique;Alice;;;
FN:Alice Unique
EMAIL:alice@example.com
END:VCARD
`), 0644); err != nil {
		t.Fatalf("failed to write iCloud test VCF: %v", err)
	}
	if err := os.WriteFile(googleVCF, []byte(`BEGIN:VCARD
VERSION:3.0
N:Different;Bob;;;
FN:Bob Different
EMAIL:bob@example.com
END:VCARD
`), 0644); err != nil {
		t.Fatalf("failed to write Google test VCF: %v", err)
	}

	outPath := filepath.Join(tmpDir, "final.vcf")
	err := run(icloudVCF, googleVCF, outPath, "", false)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("final.vcf not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("final.vcf is empty")
	}
}

func TestRunKeepIntermediates(t *testing.T) {
	tmpDir := t.TempDir()

	icloudVCF := filepath.Join(tmpDir, "icloud.vcf")
	googleVCF := filepath.Join(tmpDir, "google.vcf")
	if err := os.WriteFile(icloudVCF, []byte(`BEGIN:VCARD
VERSION:3.0
N:One;Person;;;
FN:Person One
EMAIL:one@example.com
END:VCARD
`), 0644); err != nil {
		t.Fatalf("failed to write iCloud test VCF: %v", err)
	}
	if err := os.WriteFile(googleVCF, []byte(`BEGIN:VCARD
VERSION:3.0
N:Two;Person;;;
FN:Person Two
EMAIL:two@example.com
END:VCARD
`), 0644); err != nil {
		t.Fatalf("failed to write Google test VCF: %v", err)
	}

	outDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("failed to create output directory: %v", err)
	}
	outPath := filepath.Join(outDir, "final.vcf")
	err := run(icloudVCF, googleVCF, outPath, "", true)
	if err != nil {
		t.Fatalf("run --keep failed: %v", err)
	}

	// Check that merged.vcf was kept alongside the output
	mergedKept := filepath.Join(outDir, "merged.vcf")
	if _, err := os.Stat(mergedKept); err != nil {
		t.Errorf("merged.vcf not kept: %v", err)
	}
}

func TestRunReportSaved(t *testing.T) {
	tmpDir := t.TempDir()

	icloudVCF := filepath.Join(tmpDir, "icloud.vcf")
	googleVCF := filepath.Join(tmpDir, "google.vcf")
	if err := os.WriteFile(icloudVCF, []byte(`BEGIN:VCARD
VERSION:3.0
N:Solo;Han;;;
FN:Han Solo
EMAIL:han@falcon.com
END:VCARD
`), 0644); err != nil {
		t.Fatalf("failed to write iCloud test VCF: %v", err)
	}
	if err := os.WriteFile(googleVCF, []byte(`BEGIN:VCARD
VERSION:3.0
N:Skywalker;Luke;;;
FN:Luke Skywalker
EMAIL:luke@jedi.com
END:VCARD
`), 0644); err != nil {
		t.Fatalf("failed to write Google test VCF: %v", err)
	}

	outPath := filepath.Join(tmpDir, "final.vcf")
	reportPath := filepath.Join(tmpDir, "report.json")
	err := run(icloudVCF, googleVCF, outPath, reportPath, false)
	if err != nil {
		t.Fatalf("run --report failed: %v", err)
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("report.json not saved: %v", err)
	}

	var report model.Report
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("report.json invalid: %v", err)
	}
	if report.Summary.ICloudTotal != 1 {
		t.Errorf("icloud_total = %d, want 1", report.Summary.ICloudTotal)
	}
}

func TestRunTempDirCleanup(t *testing.T) {
	// Capture pre-existing temp dirs to avoid flaky matches from
	// other tests or prior runs.
	before, _ := filepath.Glob(filepath.Join(os.TempDir(), "rolodex-run-*"))
	beforeSet := make(map[string]bool, len(before))
	for _, e := range before {
		beforeSet[e] = true
	}

	tmpDir := t.TempDir()

	icloudVCF := filepath.Join(tmpDir, "icloud.vcf")
	googleVCF := filepath.Join(tmpDir, "google.vcf")
	if err := os.WriteFile(icloudVCF, []byte(`BEGIN:VCARD
VERSION:3.0
N:Test;Cleanup;;;
FN:Cleanup Test
EMAIL:clean@test.com
END:VCARD
`), 0644); err != nil {
		t.Fatalf("failed to write iCloud test VCF: %v", err)
	}
	if err := os.WriteFile(googleVCF, []byte(`BEGIN:VCARD
VERSION:3.0
N:Test;Other;;;
FN:Other Test
EMAIL:other@test.com
END:VCARD
`), 0644); err != nil {
		t.Fatalf("failed to write Google test VCF: %v", err)
	}

	outPath := filepath.Join(tmpDir, "final.vcf")
	err := run(icloudVCF, googleVCF, outPath, "", false)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	// Verify no new rolodex-run-* temp dirs were left behind
	entries, _ := filepath.Glob(filepath.Join(os.TempDir(), "rolodex-run-*"))
	for _, e := range entries {
		if !beforeSet[e] {
			t.Errorf("temp dir not cleaned up: %s", e)
		}
	}
}

// --- audit integration tests ---

func TestAuditOnTestdata(t *testing.T) {
	contacts, _, err := parser.ParseFile("../../testdata/icloud.vcf", model.SourceICloud)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	result := audit.Audit(contacts, audit.AuditOptions{})
	if result.Total != 5 {
		t.Errorf("total = %d, want 5", result.Total)
	}
	// Charlie Williams has only email (reachable)
	// All iCloud contacts have at least email or phone
	if result.UnreachableCount != 0 {
		t.Errorf("expected 0 unreachable in icloud testdata, got %d", result.UnreachableCount)
	}
}

func TestAuditMissingFile(t *testing.T) {
	err := runAuditCmd("/nonexistent/file.vcf", "text", false)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestAuditInvalidFormat(t *testing.T) {
	tmpDir := t.TempDir()
	vcf := filepath.Join(tmpDir, "test.vcf")
	if err := os.WriteFile(vcf, []byte(`BEGIN:VCARD
VERSION:3.0
N:Test;User;;;
FN:Test User
END:VCARD
`), 0644); err != nil {
		t.Fatalf("failed to write test VCF: %v", err)
	}

	err := runAuditCmd(vcf, "csv", false)
	if err == nil {
		t.Error("expected error for invalid format")
	}
}
