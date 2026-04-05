package resolve

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/parser"
	"github.com/fairbearlab/rolodex/internal/writer"
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
	mergedContacts, _, err := parser.ParseFile(mergedPath, "merged")
	if err != nil {
		return fmt.Errorf("reading merged contacts: %w", err)
	}

	// Read review.vcf
	reviewContacts, _, err := parser.ParseFile(reviewPath, "review")
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

	// Build set of contact names that the user decided to merge
	mergeNames := make(map[string]bool)
	skipCount := 0
	for _, rd := range report.Review {
		if rd.Decision == "merge" {
			for _, ref := range rd.Contacts {
				mergeNames[normalizeRefName(ref.Name)] = true
			}
		}
	}

	// Include review contacts whose name matches a "merge" decision
	for _, c := range reviewContacts {
		name := normalizeRefName(contactName(c))
		if mergeNames[name] {
			output = append(output, model.MergedContact{
				Contact: c,
				Sources: []model.Source{c.Source},
			})
		} else {
			skipCount++
		}
	}

	if skipCount > 0 {
		fmt.Printf("Skipped %d review contacts (decision: skip/pending)\n", skipCount)
	}

	// Write output
	if err := writer.WriteFile(outPath, output); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	fmt.Printf("Resolved %d contacts → %s\n", len(output), outPath)
	return nil
}

func contactName(c model.ParsedContact) string {
	if c.FormattedName != "" {
		return c.FormattedName
	}
	name := strings.TrimSpace(c.GivenName + " " + c.FamilyName)
	if name != "" {
		return name
	}
	if len(c.Emails) > 0 {
		return c.Emails[0].Address
	}
	return ""
}

func normalizeRefName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
