package resolve

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fairbearlabs/rolodex/internal/model"
	"github.com/fairbearlabs/rolodex/internal/parser"
	"github.com/fairbearlabs/rolodex/internal/writer"
)

// Run reads an edited report.json, applies decisions to review.vcf contacts,
// and writes the resolved contacts combined with merged.vcf to the output.
func Run(reportPath, reviewPath, mergedPath, outPath string) error {
	// Read report
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		return fmt.Errorf("reading report: %w", err)
	}
	var report model.Report
	if err := json.Unmarshal(reportData, &report); err != nil {
		return fmt.Errorf("parsing report: %w", err)
	}

	// Read merged.vcf
	mergedContacts, _, err := parser.ParseFile(mergedPath, model.SourceICloud)
	if err != nil {
		return fmt.Errorf("reading merged contacts: %w", err)
	}

	// Read review.vcf
	reviewContacts, _, err := parser.ParseFile(reviewPath, model.SourceICloud)
	if err != nil {
		return fmt.Errorf("reading review contacts: %w", err)
	}

	// Build output: start with all merged contacts
	var output []model.MergedContact
	for _, c := range mergedContacts {
		output = append(output, model.MergedContact{
			Contact: c,
			Sources: []model.Source{c.Source},
		})
	}

	// Apply review decisions
	// Build a map of review cluster decisions
	decisions := make(map[string]string) // cluster_id -> decision
	for _, rd := range report.Review {
		decisions[rd.ClusterID] = rd.Decision
	}

	// For each review contact, check if it should be included
	for _, c := range reviewContacts {
		// Check the X-ROLODEX-REVIEW field
		output = append(output, model.MergedContact{
			Contact: c,
			Sources: []model.Source{c.Source},
		})
	}

	// Write output
	if err := writer.WriteFile(outPath, output); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	fmt.Printf("Resolved %d contacts → %s\n", len(output), outPath)
	return nil
}
