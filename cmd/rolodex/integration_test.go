package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/parser"
	"github.com/fairbearlab/rolodex/internal/prune"
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

// --- prune integration tests ---

func TestPruneOnTestdata(t *testing.T) {
	contacts, _, err := parser.ParseFile("../../testdata/icloud.vcf", model.SourceICloud)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	result := prune.Split(contacts, prune.Options{ReachableBy: prune.DefaultChannels})
	if result.Total != 5 {
		t.Errorf("total = %d, want 5", result.Total)
	}
	// Charlie Williams has only email (reachable)
	// All iCloud contacts have at least email or phone
	if len(result.Removed) != 0 {
		t.Errorf("expected 0 unreachable in icloud testdata, got %d", len(result.Removed))
	}
}

func TestPruneMissingFile(t *testing.T) {
	err := runPrune([]string{"/nonexistent/file.vcf"}, &bytes.Buffer{})
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestPruneInvalidFormat(t *testing.T) {
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

	err := runPrune([]string{vcf, "--format", "csv"}, &bytes.Buffer{})
	if err == nil {
		t.Error("expected error for invalid format")
	}
}

func TestPruneFlagsAfterPath(t *testing.T) {
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
	var stdout bytes.Buffer
	err := runPrune([]string{vcf, "--format", "json"}, &stdout)
	if err != nil {
		t.Fatalf("prune with flags after path failed: %v", err)
	}
	if !strings.HasPrefix(stdout.String(), "{") {
		t.Errorf("--format json after the path was ignored:\n%s", stdout.String())
	}

	// --format=value syntax should also work
	err = runPrune([]string{vcf, "--format=json"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("prune with --format=json after path failed: %v", err)
	}

	// --out after the path must not be mistaken for a dry run
	kept := filepath.Join(tmpDir, "kept.vcf")
	if err := runPrune([]string{vcf, "--out", kept}, &bytes.Buffer{}); err != nil {
		t.Fatalf("prune with --out after path failed: %v", err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("--out after the path was ignored: %v", err)
	}

	// Extra positional args should be rejected
	err = runPrune([]string{vcf, "extra.vcf"}, &bytes.Buffer{})
	if err == nil {
		t.Error("expected error for extra positional args")
	}
}

// audit is gone; the message says where it went.
func TestAuditCommandRemoved(t *testing.T) {
	err := dispatch("audit", []string{"x.vcf"})
	if err == nil {
		t.Fatal("audit should fail")
	}
	want := `audit was replaced by "rolodex prune"; run it without --out for a report`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	usage := captureStderr(t, printUsage)
	if !strings.Contains(usage, "prune    Split a .vcf into reachable and unreachable contacts") {
		t.Errorf("usage does not list prune:\n%s", usage)
	}
	if strings.Contains(usage, "audit") {
		t.Errorf("usage still lists audit:\n%s", usage)
	}
	if err := dispatch("no-such-command", nil); !errors.Is(err, errUnknownCommand) {
		t.Errorf("unknown command error = %v", err)
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

// --out may itself be the --keep merged.vcf path: the kept copy is skipped
// rather than flagged as a collision, and the final output lands there.
func TestRunKeepOutNamedMergedVcf(t *testing.T) {
	tmpDir := t.TempDir()

	icloudVCF := filepath.Join(tmpDir, "icloud.vcf")
	googleVCF := filepath.Join(tmpDir, "google.vcf")
	if err := os.WriteFile(icloudVCF, []byte("BEGIN:VCARD\r\nVERSION:3.0\r\nN:One;Person;;;\r\nFN:Person One\r\nEMAIL:one@example.com\r\nEND:VCARD\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(googleVCF, []byte("BEGIN:VCARD\r\nVERSION:3.0\r\nN:Two;Person;;;\r\nFN:Person Two\r\nEMAIL:two@example.com\r\nEND:VCARD\r\n"), 0600); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(outDir, 0750); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(outDir, "merged.vcf")
	if err := run(icloudVCF, googleVCF, outPath, "", true); err != nil {
		t.Fatalf("run --keep with --out merged.vcf: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("final output missing: %v", err)
	}
}

// dispatch wires every command name to its handler; each arm reached with no
// flags must come back with that command's own usage error, proving the name
// routes to the right place.
func TestDispatchRoutesEveryCommand(t *testing.T) {
	for cmd, want := range map[string]string{
		"run":     "both --icloud and --google flags are required",
		"merge":   "both --icloud and --google flags are required",
		"review":  "--report and --review flags are required",
		"resolve": "--report, --review, and --merged flags are required",
	} {
		err := dispatch(cmd, nil)
		if err == nil || err.Error() != want {
			t.Errorf("dispatch(%q) = %v, want %q", cmd, err, want)
		}
	}
}
