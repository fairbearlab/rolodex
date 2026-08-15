package review

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fairbearlab/rolodex/internal/calibration"
	"github.com/fairbearlab/rolodex/internal/resolve"
)

// Run launches the interactive review TUI. It returns true if all clusters
// were resolved, false if the user paused with pending decisions.
func Run(reportPath, reviewPath, calibrationPath string) (bool, error) {
	loaded, err := resolve.LoadReportAndReview(reportPath, reviewPath)
	if err != nil {
		return false, err
	}

	report := loaded.Report

	if len(report.Review) == 0 {
		fmt.Println("Nothing to review — all pairs were auto-merged or distinct.")
		return true, nil
	}

	clusters := BuildClusters(report, loaded.ReviewContacts)

	// Check if all clusters are already resolved
	pending := 0
	for _, c := range clusters {
		if c.Resolved == "pending" {
			pending++
		}
	}
	if pending == 0 {
		fmt.Println("All review clusters already have decisions. Run `rolodex resolve` to apply them.")
		return true, nil
	}

	// Set up calibration log
	if calibrationPath == "" {
		dir := filepath.Dir(reportPath)
		calibrationPath = filepath.Join(dir, "calibration.jsonl")
	}
	calLog, err := calibration.NewLog(calibrationPath)
	if err != nil {
		return false, fmt.Errorf("setting up calibration log: %w", err)
	}
	defer func() { _ = calLog.Close() }()

	// Build the model
	m := ReviewModel{
		Report:     report,
		Clusters:   clusters,
		CalLog:     calLog,
		StartTime:  time.Now(),
		PairStart:  time.Now(),
		ReportPath: reportPath,
	}

	// Advance to first pending cluster
	if !m.AdvanceToNextPending() {
		fmt.Println("All review clusters already have decisions.")
		return true, nil
	}

	fmt.Printf("Reviewing %d pending pairs (of %d total)...\n\n", pending, len(clusters))

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return false, fmt.Errorf("running review TUI: %w", err)
	}

	// Check if all clusters were resolved or user paused early
	if fm, ok := finalModel.(ReviewModel); ok {
		return fm.PendingCount() == 0, nil
	}

	return false, nil
}

// writeReport atomically writes report.json.
func writeReport(path string, report any) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling report: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("writing report: %w", err)
	}
	_ = os.Remove(path) // pre-remove for Windows compatibility
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming report: %w", err)
	}
	return nil
}
