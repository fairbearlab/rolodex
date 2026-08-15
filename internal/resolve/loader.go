package resolve

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/parser"
)

// LoadedData holds the parsed report and review contacts.
type LoadedData struct {
	Report         model.Report
	ReviewContacts []model.ParsedContact
}

// LoadReportAndReview reads report.json and review.vcf, returning both.
// This is the shared loader used by both resolve and review commands.
func LoadReportAndReview(reportPath, reviewPath string) (LoadedData, error) {
	reportData, err := os.ReadFile(filepath.Clean(reportPath))
	if err != nil {
		return LoadedData{}, fmt.Errorf("reading report: %w", err)
	}
	var report model.Report
	if err := json.Unmarshal(reportData, &report); err != nil {
		return LoadedData{}, fmt.Errorf("parsing report: %w", err)
	}

	reviewContacts, _, err := parser.ParseFile(reviewPath, "review")
	if err != nil {
		return LoadedData{}, fmt.Errorf("reading review contacts: %w", err)
	}

	return LoadedData{
		Report:         report,
		ReviewContacts: reviewContacts,
	}, nil
}
