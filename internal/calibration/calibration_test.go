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
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Error("calibration file is empty")
	}

	_ = log.Close()
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

// TestBandStatsCoversExactNameFloorPairs: the exact-name rule auto-merges
// pairs whose linear score is well under 0.50 (name 0.40 + one identifier),
// so those decisions need a band of their own or they vanish from the
// calibration report entirely.
func TestBandStatsCoversExactNameFloorPairs(t *testing.T) {
	entries := []Entry{
		{ClusterID: "a", Decision: "merge", Score: 0.90},
		{ClusterID: "b", Decision: "merge", Score: 0.55},
		{ClusterID: "c", Decision: "merge", Score: 0.45},
		{ClusterID: "d", Decision: "skip", Score: 0.40},
		{ClusterID: "e", Decision: "merge", Score: 0.0},
	}

	s := Analyze(entries)

	byLabel := make(map[string]BandStat, len(s.BandStats))
	for _, b := range s.BandStats {
		byLabel[b.Label] = b
	}

	low, ok := byLabel["0.00-0.50"]
	if !ok {
		t.Fatalf("no 0.00-0.50 band in the summary; bands present: %v", byLabel)
	}
	if low.Merged != 2 || low.Skipped != 1 {
		t.Errorf("band %s: merged=%d skipped=%d, want 2/1", low.Label, low.Merged, low.Skipped)
	}

	// Every entry must land in exactly one band: the bounds must not overlap
	// or leave a hole between 0.0 and 1.0.
	total := 0
	for _, b := range s.BandStats {
		total += b.Merged + b.Skipped
	}
	if total != len(entries) {
		t.Errorf("bands accounted for %d of %d entries", total, len(entries))
	}

	// A score exactly on a boundary belongs to the higher band only.
	edge := Analyze([]Entry{{ClusterID: "x", Decision: "merge", Score: 0.50}})
	for _, b := range edge.BandStats {
		if b.Label == "0.00-0.50" && b.Merged > 0 {
			t.Error("score 0.50 was counted in the 0.00-0.50 band; bounds overlap")
		}
	}
	if len(edge.BandStats) != 1 || edge.BandStats[0].Label != "0.50-0.60" {
		t.Errorf("score 0.50 landed in %v, want only the 0.50-0.60 band", edge.BandStats)
	}
}

// TestFloorNeverExceedsSuggestedAutoMerge: the floor is the suggested
// auto_merge threshold minus 0.05, clamped to 0.50 — and the clamp was
// applied without looking at the suggestion. Before the exact-name and
// near-name rules, every reviewed pair scored at least 0.60, so the clamp was
// unreachable; now a name-only pair at 0.40 that the reviewer merges yields
// "Suggested auto_merge: 0.40" followed by "Suggested review floor: 0.50",
// a review band with its floor above its ceiling. When the clamp would put
// the floor at or above the suggestion there is no band to suggest.
func TestFloorNeverExceedsSuggestedAutoMerge(t *testing.T) {
	cases := []struct {
		score     float64
		wantFloor *float64
	}{
		{0.90, ptr(0.85)},
		{0.52, ptr(0.50)}, // clamp engages, still below the suggestion
		{0.50, nil},       // clamp would equal the suggestion: empty band
		{0.40, nil},       // near-name floor pair
		{0.0, nil},        // nameless or exact-name floor pair
	}
	for _, tc := range cases {
		s := Analyze([]Entry{{ClusterID: "a", Decision: "merge", Score: tc.score}})
		if s.SuggestedAutoMerge == nil || !approxEq(*s.SuggestedAutoMerge, tc.score) {
			t.Errorf("score %.2f: SuggestedAutoMerge = %v, want %.2f", tc.score, s.SuggestedAutoMerge, tc.score)
		}
		switch {
		case tc.wantFloor == nil && s.SuggestedFloor != nil:
			t.Errorf("score %.2f: SuggestedFloor = %.2f, want none (it would not be below the suggested auto_merge)", tc.score, *s.SuggestedFloor)
		case tc.wantFloor != nil && s.SuggestedFloor == nil:
			t.Errorf("score %.2f: SuggestedFloor = nil, want %.2f", tc.score, *tc.wantFloor)
		case tc.wantFloor != nil && !approxEq(*s.SuggestedFloor, *tc.wantFloor):
			t.Errorf("score %.2f: SuggestedFloor = %.2f, want %.2f", tc.score, *s.SuggestedFloor, *tc.wantFloor)
		}
		if s.SuggestedFloor != nil && *s.SuggestedFloor >= *s.SuggestedAutoMerge {
			t.Errorf("score %.2f: floor %.2f is not below suggested auto_merge %.2f", tc.score, *s.SuggestedFloor, *s.SuggestedAutoMerge)
		}
	}
}

func ptr(f float64) *float64 { return &f }
