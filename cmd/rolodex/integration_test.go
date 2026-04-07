package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
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
