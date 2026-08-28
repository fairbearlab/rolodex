package main

import (
	"encoding/json"
	"fmt"
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
		outPath, reviewPath, reportPath, false)
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
	reportData, err := os.ReadFile(filepath.Clean(reportPath))
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
	if err := os.WriteFile(emptyPath, []byte(""), 0600); err != nil {
		t.Fatalf("failed to create empty vcf file: %v", err)
	}

	outPath := filepath.Join(tmpDir, "merged.vcf")
	reviewPath := filepath.Join(tmpDir, "review.vcf")

	err := merge(emptyPath, "../../testdata/google.vcf",
		outPath, reviewPath, "", false)
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
`), 0600); err != nil {
		t.Fatalf("failed to write iCloud test VCF: %v", err)
	}
	if err := os.WriteFile(googleVCF, []byte(`BEGIN:VCARD
VERSION:3.0
N:Different;Bob;;;
FN:Bob Different
EMAIL:bob@example.com
END:VCARD
`), 0600); err != nil {
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
`), 0600); err != nil {
		t.Fatalf("failed to write iCloud test VCF: %v", err)
	}
	if err := os.WriteFile(googleVCF, []byte(`BEGIN:VCARD
VERSION:3.0
N:Two;Person;;;
FN:Person Two
EMAIL:two@example.com
END:VCARD
`), 0600); err != nil {
		t.Fatalf("failed to write Google test VCF: %v", err)
	}

	outDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(outDir, 0750); err != nil {
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
`), 0600); err != nil {
		t.Fatalf("failed to write iCloud test VCF: %v", err)
	}
	if err := os.WriteFile(googleVCF, []byte(`BEGIN:VCARD
VERSION:3.0
N:Skywalker;Luke;;;
FN:Luke Skywalker
EMAIL:luke@jedi.com
END:VCARD
`), 0600); err != nil {
		t.Fatalf("failed to write Google test VCF: %v", err)
	}

	outPath := filepath.Join(tmpDir, "final.vcf")
	reportPath := filepath.Join(tmpDir, "report.json")
	err := run(icloudVCF, googleVCF, outPath, reportPath, false)
	if err != nil {
		t.Fatalf("run --report failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Clean(reportPath))
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
`), 0600); err != nil {
		t.Fatalf("failed to write iCloud test VCF: %v", err)
	}
	if err := os.WriteFile(googleVCF, []byte(`BEGIN:VCARD
VERSION:3.0
N:Test;Other;;;
FN:Other Test
EMAIL:other@test.com
END:VCARD
`), 0600); err != nil {
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
`), 0600); err != nil {
		t.Fatalf("failed to write test VCF: %v", err)
	}

	err := runAuditCmd(vcf, "csv", false)
	if err == nil {
		t.Error("expected error for invalid format")
	}
}

func TestAuditFlagsAfterPath(t *testing.T) {
	tmpDir := t.TempDir()
	vcf := filepath.Join(tmpDir, "test.vcf")
	if err := os.WriteFile(vcf, []byte(`BEGIN:VCARD
VERSION:3.0
N:Test;User;;;
FN:Test User
EMAIL:test@example.com
END:VCARD
`), 0600); err != nil {
		t.Fatalf("failed to write test VCF: %v", err)
	}

	// Flags after the file path should still be parsed
	err := runAuditCmdFlags([]string{vcf, "--format", "json"})
	if err != nil {
		t.Fatalf("audit with flags after path failed: %v", err)
	}

	// --format=value syntax should also work
	err = runAuditCmdFlags([]string{vcf, "--format=json"})
	if err != nil {
		t.Fatalf("audit with --format=json after path failed: %v", err)
	}

	// Extra positional args should be rejected
	err = runAuditCmdFlags([]string{vcf, "extra.vcf"})
	if err == nil {
		t.Error("expected error for extra positional args")
	}
}

func TestRunReportOverlapOut(t *testing.T) {
	tmpDir := t.TempDir()

	icloudVCF := filepath.Join(tmpDir, "icloud.vcf")
	googleVCF := filepath.Join(tmpDir, "google.vcf")
	if err := os.WriteFile(icloudVCF, []byte(`BEGIN:VCARD
VERSION:3.0
N:Test;User;;;
FN:Test User
EMAIL:test@example.com
END:VCARD
`), 0600); err != nil {
		t.Fatalf("failed to write iCloud test VCF: %v", err)
	}
	if err := os.WriteFile(googleVCF, []byte(`BEGIN:VCARD
VERSION:3.0
N:Other;User;;;
FN:Other User
EMAIL:other@example.com
END:VCARD
`), 0600); err != nil {
		t.Fatalf("failed to write Google test VCF: %v", err)
	}

	samePath := filepath.Join(tmpDir, "output.vcf")
	err := run(icloudVCF, googleVCF, samePath, samePath, false)
	if err == nil {
		t.Error("expected error when --report and --out point to the same file")
	}
}

func TestRunReportOverlapKeepIntermediates(t *testing.T) {
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(outDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	icloudVCF := filepath.Join(tmpDir, "icloud.vcf")
	googleVCF := filepath.Join(tmpDir, "google.vcf")
	writeTestVCF(t, icloudVCF, "One", "one@example.com")
	writeTestVCF(t, googleVCF, "Two", "two@example.com")

	outPath := filepath.Join(outDir, "final.vcf")

	// --report pointing to merged.vcf in the output dir should fail with --keep
	err := run(icloudVCF, googleVCF, outPath, filepath.Join(outDir, "merged.vcf"), true)
	if err == nil {
		t.Error("expected error when --report collides with --keep merged.vcf")
	}

	// --report pointing to review.vcf in the output dir should fail with --keep
	err = run(icloudVCF, googleVCF, outPath, filepath.Join(outDir, "review.vcf"), true)
	if err == nil {
		t.Error("expected error when --report collides with --keep review.vcf")
	}

	// Same paths should be fine without --keep
	err = run(icloudVCF, googleVCF, outPath, filepath.Join(outDir, "merged.vcf"), false)
	if err != nil {
		t.Errorf("--report=merged.vcf without --keep should succeed: %v", err)
	}
}

func TestRunKeepClearsStaleArtifacts(t *testing.T) {
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(outDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	icloudVCF := filepath.Join(tmpDir, "icloud.vcf")
	googleVCF := filepath.Join(tmpDir, "google.vcf")
	writeTestVCF(t, icloudVCF, "One", "one@example.com")
	writeTestVCF(t, googleVCF, "Two", "two@example.com")

	outPath := filepath.Join(outDir, "final.vcf")

	// Plant stale artifacts from a hypothetical previous --keep run
	staleReview := filepath.Join(outDir, "review.vcf")
	staleCal := filepath.Join(outDir, "calibration.jsonl")
	if err := os.WriteFile(staleReview, []byte("STALE"), 0600); err != nil {
		t.Fatalf("write stale review: %v", err)
	}
	if err := os.WriteFile(staleCal, []byte("STALE"), 0600); err != nil {
		t.Fatalf("write stale calibration: %v", err)
	}

	// Run with --keep; this run has no review pairs, so review.vcf and
	// calibration.jsonl should NOT be produced. The stale copies must be removed.
	err := run(icloudVCF, googleVCF, outPath, "", true)
	if err != nil {
		t.Fatalf("run --keep failed: %v", err)
	}

	if _, err := os.Stat(staleReview); !os.IsNotExist(err) {
		t.Error("stale review.vcf was not cleaned up by --keep")
	}
	if _, err := os.Stat(staleCal); !os.IsNotExist(err) {
		t.Error("stale calibration.jsonl was not cleaned up by --keep")
	}
}

// writeTestVCF writes a minimal single-contact VCF for testing.
func writeTestVCF(t *testing.T, path, name, email string) {
	t.Helper()
	data := fmt.Sprintf("BEGIN:VCARD\nVERSION:3.0\nN:%s;Test;;;\nFN:Test %s\nEMAIL:%s\nEND:VCARD\n", name, name, email)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatalf("failed to write test VCF %s: %v", path, err)
	}
}
