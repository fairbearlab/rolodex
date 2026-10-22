package reporter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/fairbearlab/rolodex/internal/merger"
	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/normalize"
	"github.com/fairbearlab/rolodex/internal/writer"
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
			ICloudTotal:  icloudCount,
			GoogleTotal:  googleCount,
			WarningCount: len(warnings),
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

		// Find the best-scoring pair in the cluster
		bestScore := 0.0
		var bestFeatures model.ScoreFeatures
		for _, p := range cluster.Pairs {
			if p.Score > bestScore {
				bestScore = p.Score
				bestFeatures = p.Features
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
				Features:  bestFeatures,
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

	// Same-name pairs the merger saw but did not review. Both sides are in
	// the output as separate people; without this entry the resemblance
	// would be recorded nowhere.
	for _, d := range result.Deferred {
		var refs []model.ContactRef
		for _, side := range d.Sides {
			for _, idx := range side {
				refs = append(refs, model.ContactRef{
					Source: contacts[idx].Parsed.Source,
					Name:   contactName(contacts[idx].Parsed),
					Index:  idx,
				})
			}
		}
		report.Deferred = append(report.Deferred, model.DeferredPair{
			Score:    d.Score,
			Contacts: refs,
			Reason:   describeDeferred(contacts, d),
		})
	}
	report.Summary.DeferredCount = len(report.Deferred)

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
	if err := writer.WriteBytes(path, data); err != nil {
		return fmt.Errorf("writing report: %w", err)
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

// describeDeferred explains why a same-name pair was not put in front of
// the reviewer.
func describeDeferred(contacts []model.NormalizedContact, d model.DeferredEdge) string {
	side := func(members []int) string {
		names := make([]string, len(members))
		for i, idx := range members {
			names[i] = fmt.Sprintf("%s (%s)", contactName(contacts[idx].Parsed), contacts[idx].Parsed.Source)
		}
		if len(members) > 1 {
			return "the merged cluster of " + strings.Join(names, " + ")
		}
		return names[0]
	}
	return fmt.Sprintf("same name only: %s resembles %s by name, but one side is already merged on a shared identifier "+
		"and a name alone does not join a cluster; both are in the output as separate people",
		side(d.Sides[0]), side(d.Sides[1]))
}

// conflictField describes one single-value field mergeCluster keeps exactly
// one value of, so every other value is lost. value extracts the field's
// display text (empty means absent). Two present values are the same value,
// and not a conflict, when their key matches (defaults to value — PHOTO
// instead keys on a content hash, since two different-length-but-
// coincidentally-same-summary photos must not compare equal, and identical
// bytes must). equal overrides that identity check to run on the raw values
// instead, for BDAY alone: iCloud's "--10-22" and Google's "1989-10-22" are
// the same birthday even though neither their display text nor a naive key
// would agree.
//
// Most fields are "first non-empty wins" in mergeCluster, but two are not,
// and prefer mirrors that: given the value kept so far and a later member's
// value (both present), it reports whether mergeCluster would replace the
// kept one. BDAY upgrades a year-less birthday to the first agreeing full
// date (normalize.PreferBirthday); PHOTO lets inline image bytes replace a
// URI-only reference. Without this, Kept/Winner would name a value the
// merger did not actually keep, and — worse for BDAY — a later value that
// agrees with the partial base but not with the full date the merger kept
// would never be reported as lost.
type conflictField struct {
	name   string
	value  func(model.ParsedContact) string
	key    func(model.ParsedContact) string
	equal  func(a, b string) bool
	prefer func(kept, candidate model.ParsedContact) bool
}

var conflictFields = []conflictField{
	{name: "FN", value: func(c model.ParsedContact) string { return c.FormattedName }},
	{name: "ORG", value: func(c model.ParsedContact) string { return c.Org }},
	{name: "TITLE", value: func(c model.ParsedContact) string { return c.Title }},
	{
		name:  "BDAY",
		value: func(c model.ParsedContact) string { return c.Birthday },
		equal: normalize.BirthdaysAgree,
		prefer: func(kept, candidate model.ParsedContact) bool {
			return normalize.PreferBirthday(kept.Birthday, candidate.Birthday) != kept.Birthday
		},
	},
	{name: "NOTE", value: func(c model.ParsedContact) string { return c.Note }},
	{name: "URL", value: func(c model.ParsedContact) string { return c.URL }},
	{
		name:  "PHOTO",
		value: photoDisplay,
		key:   photoKey,
		prefer: func(kept, candidate model.ParsedContact) bool {
			return len(kept.Photo) == 0 && len(candidate.Photo) > 0
		},
	},
}

// photoDisplay is PHOTO's report value: a reference URI stays readable, but
// raw image bytes are not valid JSON text and are not worth inlining, so
// they are summarized instead.
func photoDisplay(c model.ParsedContact) string {
	if c.PhotoURI != "" {
		return c.PhotoURI
	}
	if len(c.Photo) > 0 {
		return fmt.Sprintf("<inline photo, %d bytes>", len(c.Photo))
	}
	return ""
}

// photoKey is what two PHOTO values are deduplicated on: a URI compares
// literally, and inline bytes compare by content hash so two contacts that
// happen to carry the identical photo are not reported as conflicting.
func photoKey(c model.ParsedContact) string {
	if c.PhotoURI != "" {
		return "uri:" + c.PhotoURI
	}
	if len(c.Photo) > 0 {
		sum := sha256.Sum256(c.Photo)
		return "sha256:" + hex.EncodeToString(sum[:])
	}
	return ""
}

// findConflicts reports every single-value field where two or more members
// of the cluster carry a differing value. Every member is compared against
// the value mergeCluster kept, not just one iCloud card against one Google
// card, so a same-source conflict — a second iCloud NOTE in a 3+-member
// cluster, a pair the merger's pairwise scoring never even forms — is
// caught the same as a cross-source one.
func findConflicts(contacts []model.NormalizedContact, indices []int) []model.Conflict {
	if len(indices) < 2 {
		return nil
	}

	// baseIdx mirrors merger.mergeCluster's choice of base contact: the
	// first iCloud member if there is one, else the cluster's first member
	// in the same index order mergeCluster iterates in.
	baseIdx := indices[0]
	for _, idx := range indices {
		if contacts[idx].Parsed.Source == model.SourceICloud {
			baseIdx = idx
			break
		}
	}
	// order lists baseIdx first, then the rest in cluster order — the same
	// fill order mergeCluster uses, so "first non-empty value" here and
	// there picks the same winner.
	order := make([]int, 0, len(indices))
	order = append(order, baseIdx)
	for _, idx := range indices {
		if idx != baseIdx {
			order = append(order, idx)
		}
	}

	var conflicts []model.Conflict
	for _, f := range conflictFields {
		key := f.key
		if key == nil {
			key = f.value
		}

		// Pass 1: replay mergeCluster's fold over the members in its order
		// to find the value it kept — the first non-empty one, unless the
		// field's prefer says a later member's value displaces it.
		winIdx := -1
		var keptContact model.ParsedContact
		for _, idx := range order {
			c := contacts[idx].Parsed
			if f.value(c) == "" {
				continue
			}
			if winIdx < 0 || (f.prefer != nil && f.prefer(keptContact, c)) {
				winIdx, keptContact = idx, c
			}
		}
		if winIdx < 0 {
			continue
		}
		kept, keptKey := f.value(keptContact), key(keptContact)

		// Pass 2: every other present value that is not the kept one was
		// lost. equal, when set (BDAY), compares the raw values canonically
		// instead of the key.
		var discarded []model.ConflictValue
		seenKeys := map[string]bool{keptKey: true}
		for _, idx := range order {
			if idx == winIdx {
				continue
			}
			c := contacts[idx].Parsed
			v := f.value(c)
			if v == "" {
				continue
			}
			k := key(c)
			same := k == keptKey
			if f.equal != nil {
				same = f.equal(kept, v)
			}
			if same || seenKeys[k] {
				continue
			}
			seenKeys[k] = true
			discarded = append(discarded, model.ConflictValue{
				Source: c.Source,
				Index:  idx,
				Value:  v,
			})
		}

		if len(discarded) > 0 {
			conflicts = append(conflicts, model.Conflict{
				Field:     f.name,
				Winner:    keptContact.Source,
				Kept:      kept,
				Discarded: discarded,
			})
		}
	}

	return conflicts
}
