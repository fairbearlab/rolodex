package calibration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fairbearlab/rolodex/internal/model"
)

func TestLogAppendAndEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cal.jsonl")

	log, err := NewLog(path)
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}

	e := Entry{
		ClusterID:      "abc123",
		Decision:       "merge",
		Score:          0.82,
		Features:       model.ScoreFeatures{NameSimilarity: 0.91, SharedEmail: true},
		ViewMode:       "compact",
		DecisionTimeMs: 450,
		Timestamp:      time.Now(),
	}
	if err := log.Append(e); err != nil {
		t.Fatalf("Append: %v", err)
	}

	entries := log.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ClusterID != "abc123" {
		t.Errorf("ClusterID = %q, want %q", entries[0].ClusterID, "abc123")
	}

	// Verify file was written
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Error("calibration file is empty")
	}

	log.Close()
}

func approxEq(a, b float64) bool {
	const eps = 0.001
	d := a - b
	return d < eps && d > -eps
}

func TestAnalyzeBasic(t *testing.T) {
	entries := []Entry{
		{ClusterID: "a", Decision: "merge", Score: 0.82, ViewMode: "compact", DecisionTimeMs: 500},
		{ClusterID: "b", Decision: "merge", Score: 0.79, ViewMode: "compact", DecisionTimeMs: 600},
		{ClusterID: "c", Decision: "skip", Score: 0.65, ViewMode: "detailed", DecisionTimeMs: 3000},
		{ClusterID: "d", Decision: "merge", Score: 0.70, ViewMode: "detailed", DecisionTimeMs: 4000},
	}

	s := Analyze(entries)

	if s.TotalReviewed != 4 {
		t.Errorf("TotalReviewed = %d, want 4", s.TotalReviewed)
	}
	if s.MergeCount != 3 {
		t.Errorf("MergeCount = %d, want 3", s.MergeCount)
	}
	if s.SkipCount != 1 {
		t.Errorf("SkipCount = %d, want 1", s.SkipCount)
	}
	if s.SuggestedAutoMerge == nil {
		t.Fatal("SuggestedAutoMerge is nil")
	}
	// Lowest merged score is 0.70, rounded down to 0.70
	if !approxEq(*s.SuggestedAutoMerge, 0.70) {
		t.Errorf("SuggestedAutoMerge = %v, want ~0.70", *s.SuggestedAutoMerge)
	}
	if s.SuggestedFloor == nil {
		t.Fatal("SuggestedFloor is nil")
	}
	// 0.70 - 0.05 = 0.65
	if !approxEq(*s.SuggestedFloor, 0.65) {
		t.Errorf("SuggestedFloor = %v, want ~0.65", *s.SuggestedFloor)
	}
}

func TestAnalyzeReplayBasedUndo(t *testing.T) {
	// Undo should cause replay: only the final decision per cluster counts
	entries := []Entry{
		{ClusterID: "a", Decision: "merge", Score: 0.82, ViewMode: "compact", DecisionTimeMs: 500},
		{ClusterID: "a", Decision: "undo", Score: 0.82, ViewMode: "compact", DecisionTimeMs: 0},
		{ClusterID: "a", Decision: "skip", Score: 0.82, ViewMode: "compact", DecisionTimeMs: 800},
	}

	s := Analyze(entries)

	if s.MergeCount != 0 {
		t.Errorf("MergeCount = %d, want 0 (undo then skip)", s.MergeCount)
	}
	if s.SkipCount != 1 {
		t.Errorf("SkipCount = %d, want 1", s.SkipCount)
	}
}

func TestAnalyzeNoMerges(t *testing.T) {
	entries := []Entry{
		{ClusterID: "a", Decision: "skip", Score: 0.65},
		{ClusterID: "b", Decision: "skip", Score: 0.70},
	}

	s := Analyze(entries)

	if s.SuggestedAutoMerge != nil {
		t.Errorf("SuggestedAutoMerge should be nil when no merges, got %.2f", *s.SuggestedAutoMerge)
	}
}

func TestAnalyzeFloorClamp(t *testing.T) {
	// If lowest merged score is 0.52, floor would be 0.47, clamped to 0.50
	entries := []Entry{
		{ClusterID: "a", Decision: "merge", Score: 0.52},
	}

	s := Analyze(entries)

	if s.SuggestedFloor == nil {
		t.Fatal("SuggestedFloor is nil")
	}
	if !approxEq(*s.SuggestedFloor, 0.50) {
		t.Errorf("SuggestedFloor = %v, want ~0.50 (clamped)", *s.SuggestedFloor)
	}
}

func TestBandStats(t *testing.T) {
	entries := []Entry{
		{ClusterID: "a", Decision: "merge", Score: 0.82},
		{ClusterID: "b", Decision: "merge", Score: 0.80},
		{ClusterID: "c", Decision: "skip", Score: 0.79},
		{ClusterID: "d", Decision: "merge", Score: 0.65},
		{ClusterID: "e", Decision: "skip", Score: 0.62},
	}

	s := Analyze(entries)

	if len(s.BandStats) != 2 {
		t.Fatalf("expected 2 bands with data, got %d", len(s.BandStats))
	}

	// Band 0.78-1.00: 2 merged, 1 skipped
	high := s.BandStats[0]
	if high.Merged != 2 || high.Skipped != 1 {
		t.Errorf("band %s: merged=%d skipped=%d, want 2/1", high.Label, high.Merged, high.Skipped)
	}

	// Band 0.60-0.78: 1 merged, 1 skipped
	mid := s.BandStats[1]
	if mid.Merged != 1 || mid.Skipped != 1 {
		t.Errorf("band %s: merged=%d skipped=%d, want 1/1", mid.Label, mid.Merged, mid.Skipped)
	}
}
