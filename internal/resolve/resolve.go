package resolve

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/normalize"
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

	// Read merged.vcf, restoring real provenance from X-ROLODEX-SOURCE
	mergedContacts, _, err := parser.ParseFile(mergedPath, "merged")
	if err != nil {
		return fmt.Errorf("reading merged contacts: %w", err)
	}

	// Read review.vcf, restoring real provenance from X-ROLODEX-SOURCE
	reviewContacts, _, err := parser.ParseFile(reviewPath, "review")
	if err != nil {
		return fmt.Errorf("reading review contacts: %w", err)
	}

	// Build output: start with all merged contacts (with restored provenance)
	var output []model.MergedContact
	for _, c := range mergedContacts {
		sources := extractProvenance(c)
		output = append(output, model.MergedContact{
			Contact: c,
			Sources: sources,
		})
	}

	// Process review decisions by cluster. Review contacts in review.vcf are
	// ordered to match report.Review: each cluster contributes len(Contacts)
	// entries in sequence.
	reviewIdx := 0
	skipCount := 0
	mergeCount := 0
	for _, rd := range report.Review {
		clusterSize := len(rd.Contacts)
		if reviewIdx+clusterSize > len(reviewContacts) {
			return fmt.Errorf("report references more review contacts than exist in review.vcf")
		}
		clusterContacts := reviewContacts[reviewIdx : reviewIdx+clusterSize]
		reviewIdx += clusterSize

		switch rd.Decision {
		case "merge":
			// Actually merge the cluster into a single contact
			mc := mergeReviewCluster(clusterContacts)
			output = append(output, mc)
			mergeCount++
		case "skip":
			skipCount += clusterSize
			// Skip means exclude these contacts from the output
		default:
			// "pending" or unknown: keep all contacts as-is (safe default)
			for _, c := range clusterContacts {
				sources := extractProvenance(c)
				output = append(output, model.MergedContact{
					Contact: c,
					Sources: sources,
				})
			}
		}
	}

	// Validate that all review contacts were accounted for — if the report
	// has fewer clusters than review.vcf, the unreferenced contacts would be
	// silently dropped.
	if reviewIdx < len(reviewContacts) {
		return fmt.Errorf("review.vcf has %d contacts but report only references %d — %d contacts would be lost",
			len(reviewContacts), reviewIdx, len(reviewContacts)-reviewIdx)
	}

	if mergeCount > 0 {
		fmt.Printf("Merged %d review clusters\n", mergeCount)
	}
	if skipCount > 0 {
		fmt.Printf("Skipped %d review contacts (decision: skip)\n", skipCount)
	}

	// Write output
	if err := writer.WriteFile(outPath, output); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	fmt.Printf("Resolved %d contacts → %s\n", len(output), outPath)
	return nil
}

// extractProvenance reads the X-ROLODEX-SOURCE extra field to restore real
// source provenance that was written by the writer. Falls back to the
// parser-assigned Source if the field is missing.
func extractProvenance(c model.ParsedContact) []model.Source {
	if vals, ok := c.Extra["X-ROLODEX-SOURCE"]; ok && len(vals) > 0 {
		raw := vals[0]
		// Format is either "icloud", "google", or "merged(icloud+google)"
		if strings.HasPrefix(raw, "merged(") && strings.HasSuffix(raw, ")") {
			inner := raw[len("merged(") : len(raw)-1]
			parts := strings.Split(inner, "+")
			sources := make([]model.Source, len(parts))
			for i, p := range parts {
				sources[i] = model.Source(strings.TrimSpace(p))
			}
			return sources
		}
		return []model.Source{model.Source(raw)}
	}
	return []model.Source{c.Source}
}

// mergeReviewCluster merges a slice of review contacts into one contact
// using iCloud-priority for single-value fields and union for multi-value fields.
func mergeReviewCluster(contacts []model.ParsedContact) model.MergedContact {
	if len(contacts) == 1 {
		return model.MergedContact{
			Contact: contacts[0],
			Sources: extractProvenance(contacts[0]),
		}
	}

	// Find iCloud contact as priority base
	baseIdx := 0
	for i, c := range contacts {
		sources := extractProvenance(c)
		for _, s := range sources {
			if s == model.SourceICloud {
				baseIdx = i
				break
			}
		}
	}
	base := contacts[baseIdx]

	// Collect all sources
	var allSources []model.Source
	sourceSet := make(map[model.Source]bool)
	for _, c := range contacts {
		for _, s := range extractProvenance(c) {
			if !sourceSet[s] {
				sourceSet[s] = true
				allSources = append(allSources, s)
			}
		}
	}

	// Union multi-value fields
	emailSet := make(map[string]model.Email)
	phoneSet := make(map[string]model.Phone)

	for _, e := range base.Emails {
		emailSet[normalize.Email(e.Address)] = e
	}
	for _, p := range base.Phones {
		phoneSet[normalize.Phone(p.Number)] = p
	}

	for i, c := range contacts {
		if i == baseIdx {
			continue
		}
		for _, e := range c.Emails {
			key := normalize.Email(e.Address)
			if _, exists := emailSet[key]; !exists {
				emailSet[key] = e
			}
		}
		for _, p := range c.Phones {
			key := normalize.Phone(p.Number)
			if _, exists := phoneSet[key]; !exists {
				phoneSet[key] = p
			}
		}

		// Fill empty single-value fields from non-priority source
		if base.FormattedName == "" && c.FormattedName != "" {
			base.FormattedName = c.FormattedName
		}
		if base.FamilyName == "" && c.FamilyName != "" {
			base.FamilyName = c.FamilyName
		}
		if base.GivenName == "" && c.GivenName != "" {
			base.GivenName = c.GivenName
		}
		if base.MiddleName == "" && c.MiddleName != "" {
			base.MiddleName = c.MiddleName
		}
		if base.Org == "" && c.Org != "" {
			base.Org = c.Org
		}
		if base.Title == "" && c.Title != "" {
			base.Title = c.Title
		}
		if base.Birthday == "" && c.Birthday != "" {
			base.Birthday = c.Birthday
		}
		if base.Note == "" && c.Note != "" {
			base.Note = c.Note
		}
		if base.URL == "" && c.URL != "" {
			base.URL = c.URL
		}
		if len(base.Photo) == 0 && len(c.Photo) > 0 {
			base.Photo = c.Photo
			base.PhotoType = c.PhotoType
		}

		// Union addresses by type
		addrTypes := make(map[string]bool)
		for _, a := range base.Addresses {
			addrTypes[a.Type] = true
		}
		for _, a := range c.Addresses {
			if !addrTypes[a.Type] {
				base.Addresses = append(base.Addresses, a)
				addrTypes[a.Type] = true
			}
		}

		// Union extra fields
		if base.Extra == nil {
			base.Extra = make(map[string][]string)
		}
		for k, vals := range c.Extra {
			if _, exists := base.Extra[k]; !exists {
				base.Extra[k] = vals
			}
		}
	}

	// Drop review-only extension fields — the user resolved this cluster,
	// so it should not carry stale review/score tags in the output.
	delete(base.Extra, "X-ROLODEX-REVIEW")
	delete(base.Extra, "X-ROLODEX-SCORE")

	// Rebuild email/phone slices with deterministic ordering
	base.Emails = make([]model.Email, 0, len(emailSet))
	for _, e := range emailSet {
		base.Emails = append(base.Emails, e)
	}
	sort.Slice(base.Emails, func(i, j int) bool {
		return base.Emails[i].Address < base.Emails[j].Address
	})
	base.Phones = make([]model.Phone, 0, len(phoneSet))
	for _, p := range phoneSet {
		base.Phones = append(base.Phones, p)
	}
	sort.Slice(base.Phones, func(i, j int) bool {
		return base.Phones[i].Number < base.Phones[j].Number
	})

	return model.MergedContact{
		Contact: base,
		Sources: allSources,
	}
}
