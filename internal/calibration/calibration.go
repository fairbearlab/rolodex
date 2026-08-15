package calibration

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/fairbearlab/rolodex/internal/model"
)

// Entry represents a single calibration log entry.
type Entry struct {
	ClusterID      string              `json:"cluster_id"`
	Decision       string              `json:"decision"` // "merge", "skip", or "undo"
	Score          float64             `json:"score"`
	Features       model.ScoreFeatures `json:"features"`
	ViewMode       string              `json:"view_mode"` // "compact" or "detailed"
	DecisionTimeMs int64               `json:"decision_time_ms"`
	Timestamp      time.Time           `json:"timestamp"`
}

// Log manages an append-only calibration JSONL file.
type Log struct {
	path    string
	file    *os.File
	entries []Entry
}

// NewLog opens (or creates) a calibration log file for appending.
func NewLog(path string) (*Log, error) {
	f, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening calibration log: %w", err)
	}
	return &Log{path: path, file: f}, nil
}

// Append writes a calibration entry to disk and keeps it in memory.
func (l *Log) Append(e Entry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := l.file.Write(append(data, '\n')); err != nil {
		return err
	}
	l.entries = append(l.entries, e)
	return nil
}

// Close closes the underlying file.
func (l *Log) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Entries returns all entries logged in this session.
func (l *Log) Entries() []Entry {
	return l.entries
}

// Summary holds computed statistics from calibration data.
type Summary struct {
	TotalReviewed      int
	MergeCount         int
	SkipCount          int
	BandStats          []BandStat
	AvgCompactTimeMs   int64
	AvgDetailedTimeMs  int64
	SuggestedAutoMerge *float64 // nil if no merges
	SuggestedFloor     *float64 // nil if no merges
}

// BandStat holds merge/skip counts for a score band.
type BandStat struct {
	Label     string
	LowBound  float64
	HighBound float64
	Merged    int
	Skipped   int
}

// Analyze computes a summary from calibration entries, using replay-based
// analysis: groups entries by cluster_id, uses only the final decision per
// cluster for threshold calculation.
func Analyze(entries []Entry) Summary {
	// Group by cluster_id, keep only the final decision per cluster
	finalByCluster := make(map[string]Entry)
	for _, e := range entries {
		finalByCluster[e.ClusterID] = e
	}

	var finals []Entry
	for _, e := range finalByCluster {
		if e.Decision == "merge" || e.Decision == "skip" {
			finals = append(finals, e)
		}
	}

	s := Summary{
		TotalReviewed: len(finals),
	}

	var compactTimes, detailedTimes []int64
	lowestMergedScore := math.MaxFloat64

	for _, e := range finals {
		switch e.Decision {
		case "merge":
			s.MergeCount++
			if e.Score < lowestMergedScore {
				lowestMergedScore = e.Score
			}
		case "skip":
			s.SkipCount++
		}

		switch e.ViewMode {
		case "compact":
			compactTimes = append(compactTimes, e.DecisionTimeMs)
		case "detailed":
			detailedTimes = append(detailedTimes, e.DecisionTimeMs)
		}
	}

	// Average decision times
	if len(compactTimes) > 0 {
		var sum int64
		for _, t := range compactTimes {
			sum += t
		}
		s.AvgCompactTimeMs = sum / int64(len(compactTimes))
	}
	if len(detailedTimes) > 0 {
		var sum int64
		for _, t := range detailedTimes {
			sum += t
		}
		s.AvgDetailedTimeMs = sum / int64(len(detailedTimes))
	}

	// Score band stats
	s.BandStats = computeBands(finals)

	// Threshold suggestions
	if s.MergeCount > 0 && lowestMergedScore < math.MaxFloat64 {
		// Suggested auto_merge = lowest score of any merged pair, rounded down to 0.01
		suggested := math.Floor(lowestMergedScore*100) / 100
		s.SuggestedAutoMerge = &suggested

		// Suggested floor = suggested auto_merge - 0.05, clamped to 0.50
		floor := suggested - 0.05
		if floor < 0.50 {
			floor = 0.50
		}
		s.SuggestedFloor = &floor
	}

	return s
}

func computeBands(entries []Entry) []BandStat {
	bands := []BandStat{
		{Label: "0.78-1.00", LowBound: 0.78, HighBound: 1.01},
		{Label: "0.60-0.78", LowBound: 0.60, HighBound: 0.78},
		{Label: "0.50-0.60", LowBound: 0.50, HighBound: 0.60},
	}

	for _, e := range entries {
		for i := range bands {
			if e.Score >= bands[i].LowBound && e.Score < bands[i].HighBound {
				switch e.Decision {
				case "merge":
					bands[i].Merged++
				case "skip":
					bands[i].Skipped++
				}
				break
			}
		}
	}

	// Only return bands with data
	var result []BandStat
	for _, b := range bands {
		if b.Merged > 0 || b.Skipped > 0 {
			result = append(result, b)
		}
	}
	return result
}
