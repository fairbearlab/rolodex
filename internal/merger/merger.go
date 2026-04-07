package merger

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/normalize"
)

// Result holds the output of the merge stage.
type Result struct {
	Merged  []model.MergedContact // auto-merged, confident
	Review  []model.MergedContact // review-tier, needs human eyes
	Clusters []model.Cluster       // cluster info for reporting
}

// Merge takes normalized contacts and scored pairs, clusters them via union-find,
// validates all pairs within each cluster, and produces merged output.
func Merge(contacts []model.NormalizedContact, pairs []model.ScoredPair) Result {
	n := len(contacts)
	uf := newUnionFind(n)

	// Build pair lookup by indices
	pairMap := make(map[[2]int]model.ScoredPair)
	for _, p := range pairs {
		a, b := p.A, p.B
		if a > b {
			a, b = b, a
		}
		if p.Tier != model.TierDistinct {
			uf.union(a, b)
		}
		pairMap[[2]int{a, b}] = p
	}

	// Get clusters with deterministic ordering
	groups := uf.clusters()
	roots := make([]int, 0, len(groups))
	for root := range groups {
		roots = append(roots, root)
	}
	sort.Ints(roots)

	var result Result
	merged := make(map[int]bool)

	for _, root := range roots {
		members := groups[root]
		if len(members) == 1 {
			continue // no merge candidate, handled below as distinct
		}

		// Collect all pairs in this cluster
		cluster := model.Cluster{Indices: members}
		allAutoMerge := true
		minScore := 1.0

		for i := 0; i < len(members); i++ {
			for j := i + 1; j < len(members); j++ {
				a, b := members[i], members[j]
				if a > b {
					a, b = b, a
				}
				if p, ok := pairMap[[2]int{a, b}]; ok {
					cluster.Pairs = append(cluster.Pairs, p)
					if p.Score < minScore {
						minScore = p.Score
					}
					if p.Tier == model.TierReview {
						allAutoMerge = false
					} else if p.Tier == model.TierDistinct {
						allAutoMerge = false
					}
				} else {
					// Pair wasn't scored (not blocked together) — treat as distinct.
					// This invalidates the cluster for auto-merge.
					allAutoMerge = false
				}
			}
		}

		result.Clusters = append(result.Clusters, cluster)

		for _, idx := range members {
			merged[idx] = true
		}

		if allAutoMerge {
			// Merge all contacts in the cluster
			mc := mergeCluster(contacts, members, minScore)
			result.Merged = append(result.Merged, mc)
		} else {
			// Cluster has review-tier pairs, unscored cross-pairs, or mixed tiers.
			// Put all contacts in review for human decision.
			clusterID := ClusterID(contacts, members)
			for _, idx := range members {
				c := contacts[idx].Parsed
				if c.Extra == nil {
					c.Extra = make(map[string][]string)
				}
				c.Extra["X-ROLODEX-CLUSTER"] = []string{clusterID}
				result.Review = append(result.Review, model.MergedContact{
					Contact:    c,
					Sources:    []model.Source{c.Source},
					Score:      minScore,
					MergedFrom: members,
					ReviewFlag: true,
				})
			}
		}
	}

	// Add distinct contacts (not part of any merge/review cluster)
	for i, c := range contacts {
		if !merged[i] {
			result.Merged = append(result.Merged, model.MergedContact{
				Contact:    c.Parsed,
				Sources:    []model.Source{c.Parsed.Source},
				Score:      0,
				MergedFrom: []int{i},
			})
		}
	}

	return result
}

// ClusterID generates a stable content-hash ID for a cluster.
func ClusterID(contacts []model.NormalizedContact, indices []int) string {
	// Sort indices for deterministic hash regardless of union-find traversal order
	sorted := make([]int, len(indices))
	copy(sorted, indices)
	sort.Ints(sorted)
	var parts []string
	for _, idx := range sorted {
		c := contacts[idx].Parsed
		parts = append(parts, fmt.Sprintf("%s:%s:%s",
			c.Source, c.FamilyName, c.GivenName))
	}
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return fmt.Sprintf("%x", h[:8])
}

// mergeCluster merges all contacts in a cluster with iCloud priority.
func mergeCluster(contacts []model.NormalizedContact, indices []int, score float64) model.MergedContact {
	// Find iCloud contact (priority source) and others
	var icloudIdx int = -1
	for _, idx := range indices {
		if contacts[idx].Parsed.Source == model.SourceICloud {
			icloudIdx = idx
			break
		}
	}

	// Start with priority source, or first contact if no iCloud
	var base model.ParsedContact
	if icloudIdx >= 0 {
		base = contacts[icloudIdx].Parsed
	} else {
		base = contacts[indices[0]].Parsed
	}

	// Collect all sources
	var sources []model.Source
	sourceSet := make(map[model.Source]bool)
	for _, idx := range indices {
		s := contacts[idx].Parsed.Source
		if !sourceSet[s] {
			sourceSet[s] = true
			sources = append(sources, s)
		}
	}

	// Union multi-value fields from all contacts
	emailSet := make(map[string]model.Email)
	phoneSet := make(map[string]model.Phone)
	addrSeen := make(map[string]bool)
	for _, a := range base.Addresses {
		addrSeen[addrContentKey(a)] = true
	}

	// Add base emails/phones first (iCloud labels win for dupes)
	for _, e := range base.Emails {
		key := normalize.Email(e.Address)
		emailSet[key] = e
	}
	for _, p := range base.Phones {
		key := normalize.Phone(p.Number)
		if key != "" {
			phoneSet[key] = p
		}
	}

	// Union from other contacts
	for _, idx := range indices {
		c := contacts[idx].Parsed
		if c.Source == base.Source && idx == icloudIdx {
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
			if key != "" {
				if _, exists := phoneSet[key]; !exists {
					phoneSet[key] = p
				}
			}
		}

		// Passthrough: fill empty single-value fields from non-priority source
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

		// Union addresses by content (not type — two HOME addresses with different
		// physical locations should both be kept)
		for _, a := range c.Addresses {
			key := addrContentKey(a)
			if !addrSeen[key] {
				base.Addresses = append(base.Addresses, a)
				addrSeen[key] = true
			}
		}

		// Union extra fields (append unique values for shared keys)
		if base.Extra == nil {
			base.Extra = make(map[string][]string)
		}
		for k, vals := range c.Extra {
			existing := base.Extra[k]
			seen := make(map[string]bool, len(existing))
			for _, v := range existing {
				seen[v] = true
			}
			for _, v := range vals {
				if !seen[v] {
					existing = append(existing, v)
				}
			}
			base.Extra[k] = existing
		}
	}

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
		Contact:    base,
		Sources:    sources,
		Score:      score,
		MergedFrom: indices,
	}
}

func addrContentKey(a model.Address) string {
	return strings.ToLower(strings.Join([]string{
		a.Street, a.City, a.Region, a.PostCode, a.Country,
	}, "|"))
}
