package review

import (
	"fmt"
	"strings"
	"time"

	"github.com/fairbearlab/rolodex/internal/calibration"
	"github.com/fairbearlab/rolodex/internal/model"
)

func renderSummaryView(m ReviewModel) string {
	w := max(min(m.Width-4, 60), 20)

	summary := calibration.Analyze(calEntriesFromDecisions(m))

	elapsed := time.Since(m.StartTime)
	mins := int(elapsed.Minutes())
	secs := int(elapsed.Seconds()) % 60

	var lines []string
	lines = append(lines, titleStyle.Render(" Review Complete"))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  %d pairs reviewed in %dm %ds", summary.TotalReviewed, mins, secs))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  Merged: %d    Skipped: %d", summary.MergeCount, summary.SkipCount))

	if len(summary.BandStats) > 0 {
		lines = append(lines, "")
		lines = append(lines, "  "+labelStyle.Render("By score band:"))
		for _, b := range summary.BandStats {
			total := b.Merged + b.Skipped
			mergeRate := 0
			if total > 0 {
				mergeRate = b.Merged * 100 / total
			}
			lines = append(lines, fmt.Sprintf("    %s:  %d merged,  %d skipped  (%d%% merge)",
				b.Label, b.Merged, b.Skipped, mergeRate))
		}
	}

	if summary.AvgCompactTimeMs > 0 || summary.AvgDetailedTimeMs > 0 {
		lines = append(lines, "")
		lines = append(lines, "  "+labelStyle.Render("Avg decision time:"))
		if summary.AvgCompactTimeMs > 0 {
			lines = append(lines, fmt.Sprintf("    Compact mode: %.1fs", float64(summary.AvgCompactTimeMs)/1000))
		}
		if summary.AvgDetailedTimeMs > 0 {
			lines = append(lines, fmt.Sprintf("    Detailed mode: %.1fs", float64(summary.AvgDetailedTimeMs)/1000))
		}
	}

	if summary.SuggestedAutoMerge != nil {
		lines = append(lines, "")
		lines = append(lines, "  "+labelStyle.Render("Threshold suggestions:"))
		lines = append(lines, fmt.Sprintf("    Current auto_merge: %.2f", model.ThresholdAutoMerge))
		lines = append(lines, fmt.Sprintf("    Suggested auto_merge: %.2f", *summary.SuggestedAutoMerge))
		lines = append(lines, fmt.Sprintf("    Current review floor: %.2f", model.ThresholdReview))
		if summary.SuggestedFloor != nil {
			lines = append(lines, fmt.Sprintf("    Suggested review floor: %.2f", *summary.SuggestedFloor))
		}
	}

	pending := m.PendingCount()
	lines = append(lines, "")
	if pending > 0 {
		lines = append(lines, fmt.Sprintf("  %d pairs still pending. Run `rolodex review` again to continue.", pending))
	} else {
		lines = append(lines, "  Run `rolodex resolve` to apply decisions.")
	}

	content := strings.Join(lines, "\n")
	return borderStyle.Width(w).Render(content) + "\n"
}

// calEntriesFromDecisions builds calibration entries from the model's in-memory
// decisions, used when the calibration log isn't available.
func calEntriesFromDecisions(m ReviewModel) []calibration.Entry {
	if m.CalLog != nil {
		return m.CalLog.Entries()
	}

	var entries []calibration.Entry
	for _, d := range m.Decisions {
		c := m.Clusters[d.ClusterIndex]
		entries = append(entries, calibration.Entry{
			ClusterID:      c.ClusterID,
			Decision:       d.Choice,
			Score:          c.Decision.Score,
			Features:       c.Features,
			ViewMode:       d.ViewMode.String(),
			DecisionTimeMs: d.DecisionMs,
			Timestamp:      d.Timestamp,
		})
	}
	return entries
}
