package reporter

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/fairbearlab/rolodex/internal/merger"
	"github.com/fairbearlab/rolodex/internal/model"
)

// Generate creates a JSON report from the merge result.
func Generate(
	contacts []model.NormalizedContact,
	result merger.Result,
	icloudCount, googleCount int,
	warnings []model.Warning,
) model.Report {
	report := model.Report{
		Summary: model.ReportSummary{
			ICloudTotal:   icloudCount,
			GoogleTotal:   googleCount,
			WarningCount:  len(warnings),
		},
		Warnings: warnings,
	}

	// Count auto-merged vs distinct from merge results
	autoMergedCount := 0
	distinctCount := 0

	for _, mc := range result.Merged {
		if len(mc.MergedFrom) > 1 {
			autoMergedCount++
		} else {
			distinctCount++
		}
	}

	report.Summary.AutoMerged = autoMergedCount
	report.Summary.DistinctCount = distinctCount
	// ReviewCount is set after building report.Review (counts clusters, not contacts)

	// Build a lookup from cluster indices to the actual merged contact,
	// so ResultName reflects passthrough-fill logic applied by the merger.
	mergedByKey := make(map[string]model.MergedContact)
	for _, mc := range result.Merged {
		if len(mc.MergedFrom) > 1 {
			sorted := make([]int, len(mc.MergedFrom))
			copy(sorted, mc.MergedFrom)
			sort.Ints(sorted)
			key := fmt.Sprintf("%v", sorted)
			mergedByKey[key] = mc
		}
	}

	// Build merge decisions
	for _, cluster := range result.Clusters {
		if len(cluster.Indices) <= 1 {
			continue
		}

		// Find the best score in the cluster
		bestScore := 0.0
		for _, p := range cluster.Pairs {
			if p.Score > bestScore {
				bestScore = p.Score
			}
		}

		clusterID := merger.ClusterID(contacts, cluster.Indices)

		// Check if this cluster is auto_merge or review.
		// Must replicate the merger's logic: a cluster is review if any pair
		// is TierReview, TierDistinct, or if any cross-pair is unscored.
		allAutoMerge := true
		for i := 0; i < len(cluster.Indices); i++ {
			for j := i + 1; j < len(cluster.Indices); j++ {
				a, b := cluster.Indices[i], cluster.Indices[j]
				if a > b {
					a, b = b, a
				}
				found := false
				for _, p := range cluster.Pairs {
					pa, pb := p.A, p.B
					if pa > pb {
						pa, pb = pb, pa
					}
					if pa == a && pb == b {
						found = true
						if p.Tier == model.TierReview || p.Tier == model.TierDistinct {
							allAutoMerge = false
						}
						break
					}
				}
				if !found {
					// Unscored cross-pair
					allAutoMerge = false
				}
			}
		}
		isReview := !allAutoMerge

		refs := make([]model.ContactRef, len(cluster.Indices))
		for i, idx := range cluster.Indices {
			refs[i] = model.ContactRef{
				Source: contacts[idx].Parsed.Source,
				Name:   contactName(contacts[idx].Parsed),
				Index:  idx,
			}
		}

		if isReview {
			ambiguity := describeAmbiguity(contacts, cluster)
			report.Review = append(report.Review, model.ReviewDecision{
				ClusterID: clusterID,
				Score:     bestScore,
				Contacts:  refs,
				Ambiguity: ambiguity,
				Decision:  "pending",
			})
		} else {
			conflicts := findConflicts(contacts, cluster.Indices)
			// Derive ResultName from the actual merged contact (which has
			// passthrough-fill applied), falling back to iCloud-priority selection.
			sorted := make([]int, len(cluster.Indices))
			copy(sorted, cluster.Indices)
			sort.Ints(sorted)
			key := fmt.Sprintf("%v", sorted)
			resultName := ""
			if mc, ok := mergedByKey[key]; ok {
				resultName = contactName(mc.Contact)
			} else {
				resultIdx := cluster.Indices[0]
				for _, idx := range cluster.Indices {
					if contacts[idx].Parsed.Source == model.SourceICloud {
						resultIdx = idx
						break
					}
				}
				resultName = contactName(contacts[resultIdx].Parsed)
			}
			report.Merged = append(report.Merged, model.MergeDecision{
				ClusterID:  clusterID,
				Score:      bestScore,
				Contacts:   refs,
				Conflicts:  conflicts,
				ResultName: resultName,
			})
		}
	}

	// Set ReviewCount to number of review clusters (not individual contacts)
	report.Summary.ReviewCount = len(report.Review)

	// Distinct entries
	for _, mc := range result.Merged {
		if len(mc.MergedFrom) == 1 {
			report.Distinct = append(report.Distinct, model.DistinctEntry{
				Source: mc.Sources[0],
				Name:   contactName(mc.Contact),
			})
		}
	}

	return report
}

// WriteFile writes the report as JSON.
func WriteFile(path string, report model.Report) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling report: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("writing report: %w", err)
	}
	// Remove existing file first — os.Rename doesn't overwrite on Windows
	os.Remove(path)
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming report: %w", err)
	}
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
	if len(c.Phones) > 0 {
		return c.Phones[0].Number
	}
	return "(unknown)"
}

func describeAmbiguity(contacts []model.NormalizedContact, cluster model.Cluster) string {
	if len(cluster.Pairs) == 0 {
		return "contacts grouped by transitive connection but no direct pair scored"
	}
	var descriptions []string
	for _, p := range cluster.Pairs {
		nameA := contactName(contacts[p.A].Parsed)
		nameB := contactName(contacts[p.B].Parsed)
		descriptions = append(descriptions,
			fmt.Sprintf("%q and %q scored %.2f (tier: %s)", nameA, nameB, p.Score, p.Tier))
	}
	return strings.Join(descriptions, "; ")
}

func findConflicts(contacts []model.NormalizedContact, indices []int) []model.Conflict {
	if len(indices) < 2 {
		return nil
	}

	var conflicts []model.Conflict
	// Compare first iCloud contact with first non-iCloud
	var icloud, other *model.ParsedContact
	for _, idx := range indices {
		c := &contacts[idx].Parsed
		if c.Source == model.SourceICloud && icloud == nil {
			icloud = c
		} else if c.Source != model.SourceICloud && other == nil {
			other = c
		}
	}

	if icloud == nil || other == nil {
		return nil
	}

	check := func(field, a, b string) {
		if a != b && a != "" && b != "" {
			conflicts = append(conflicts, model.Conflict{
				Field:       field,
				ICloudValue: a,
				GoogleValue: b,
				Winner:      "icloud",
			})
		}
	}

	check("FN", icloud.FormattedName, other.FormattedName)
	check("ORG", icloud.Org, other.Org)
	check("TITLE", icloud.Title, other.Title)
	check("BDAY", icloud.Birthday, other.Birthday)
	check("NOTE", icloud.Note, other.Note)

	return conflicts
}
