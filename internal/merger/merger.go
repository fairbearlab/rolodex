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
	Merged   []model.MergedContact // auto-merged, confident
	Review   []model.MergedContact // review-tier, needs human eyes
	Clusters []model.Cluster       // cluster info for reporting
	Deferred []model.DeferredEdge  // near-name edges not applied; see DeferredEdge
}

// Merge takes normalized contacts and scored pairs, clusters them via union-find,
// validates all pairs within each cluster, and produces merged output.
//
// Clustering is transitive on purpose: A shares a phone with B and B an email
// with C, so A, B and C are one person. That transitivity is only safe on
// edges that carry evidence. A pair held in review by the near-name floor
// alone (same name, nothing else) is not evidence of identity, and unioning
// on it collapses everyone who shares a common name into one cluster — six
// unrelated "David Lee"s, six phones, six emails, one review card, and a
// single merge keystroke destroys five of them. Those edges are applied
// second, and only between two contacts that nothing else has claimed, so a
// near-name pair is reviewed as a pair and a third namesake stays distinct
// rather than being stacked onto a cluster it has no tie to. An edge that
// is not applied is not forgotten either: it is returned in Deferred, one
// entry per pair of sides, so the report can list the namesake that was
// left out — before that, it shipped to merged.vcf as its own person with
// no card, no cluster and no warning.
func Merge(contacts []model.NormalizedContact, pairs []model.ScoredPair) Result {
	n := len(contacts)
	uf := newUnionFind(n)

	// Build pair lookup by indices; union on every edge that carries a
	// confirming identifier or clears the review score threshold.
	pairMap := make(map[[2]int]model.ScoredPair)
	attached := make([]bool, n)
	var nearNameOnly []model.ScoredPair
	for _, p := range pairs {
		a, b := p.A, p.B
		if a > b {
			a, b = b, a
		}
		pairMap[[2]int{a, b}] = p
		switch {
		case p.Tier == model.TierDistinct:
		case isNearNameOnly(p):
			nearNameOnly = append(nearNameOnly, p)
		default:
			uf.union(a, b)
			attached[a], attached[b] = true, true
		}
	}

	// Near-name-only edges pair up unattached contacts. Most similar names
	// first, cross-source before same-source (the tool's job is to match the
	// two exports), then index order, so the pairing is deterministic.
	sort.SliceStable(nearNameOnly, func(i, j int) bool {
		pi, pj := nearNameOnly[i], nearNameOnly[j]
		if pi.Score != pj.Score {
			return pi.Score > pj.Score
		}
		ci := contacts[pi.A].Parsed.Source != contacts[pi.B].Parsed.Source
		cj := contacts[pj.A].Parsed.Source != contacts[pj.B].Parsed.Source
		if ci != cj {
			return ci
		}
		if pi.A != pj.A {
			return pi.A < pj.A
		}
		return pi.B < pj.B
	})
	for _, p := range nearNameOnly {
		if attached[p.A] || attached[p.B] {
			continue
		}
		uf.union(p.A, p.B)
		attached[p.A], attached[p.B] = true, true
	}

	// Get clusters with deterministic ordering
	groups := uf.clusters()
	roots := make([]int, 0, len(groups))
	for root := range groups {
		roots = append(roots, root)
	}
	sort.Ints(roots)

	var result Result
	result.Deferred = deferredEdges(nearNameOnly, uf, groups)
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
					switch p.Tier {
					case model.TierReview, model.TierDistinct:
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

// deferredEdges collects the near-name-only edges that were not applied —
// their endpoints ended up in different groups — as one DeferredEdge per
// pair of groups, carrying the strongest edge between them. Sorted by score
// descending, then by the lowest member index, so the report is stable.
func deferredEdges(nearNameOnly []model.ScoredPair, uf *unionFind, groups map[int][]int) []model.DeferredEdge {
	type key [2]int
	best := make(map[key]float64)
	var order []key
	for _, p := range nearNameOnly {
		ra, rb := uf.find(p.A), uf.find(p.B)
		if ra == rb {
			continue // applied, or joined by other evidence
		}
		if ra > rb {
			ra, rb = rb, ra
		}
		k := key{ra, rb}
		if prev, seen := best[k]; !seen {
			best[k] = p.Score
			order = append(order, k)
		} else if p.Score > prev {
			best[k] = p.Score
		}
	}
	deferred := make([]model.DeferredEdge, 0, len(order))
	for _, k := range order {
		a := append([]int(nil), groups[k[0]]...)
		b := append([]int(nil), groups[k[1]]...)
		sort.Ints(a)
		sort.Ints(b)
		deferred = append(deferred, model.DeferredEdge{Score: best[k], Sides: [2][]int{a, b}})
	}
	sort.SliceStable(deferred, func(i, j int) bool {
		if deferred[i].Score != deferred[j].Score {
			return deferred[i].Score > deferred[j].Score
		}
		return deferred[i].Sides[0][0] < deferred[j].Sides[0][0]
	})
	return deferred
}

// isNearNameOnly reports whether a pair is in review on the strength of its
// name alone: below the review score threshold with no shared phone, email
// or birthday. Such an edge is a prompt for a human, not a link between
// people, and Merge does not chain clusters through it.
func isNearNameOnly(p model.ScoredPair) bool {
	if p.Tier != model.TierReview || p.Score >= model.ThresholdReview {
		return false
	}
	f := p.Features
	return !f.SharedPhone && !f.SharedEmail && !f.SharedBirthday
}

// ClusterID generates a stable content-hash ID for a cluster. Every member's
// index is part of the hash: a contact belongs to exactly one cluster per
// run, so two clusters can never share an id. Hashing names alone let two
// unrelated "Alex" pairs collide, and the review TUI then wrote one decision
// onto both. The same input files yield the same indices, so ids are still
// stable across re-runs.
func ClusterID(contacts []model.NormalizedContact, indices []int) string {
	// Sort indices for deterministic hash regardless of union-find traversal order
	sorted := make([]int, len(indices))
	copy(sorted, indices)
	sort.Ints(sorted)
	var parts []string
	for _, idx := range sorted {
		c := contacts[idx].Parsed
		parts = append(parts, fmt.Sprintf("%s:%d:%s:%s",
			c.Source, idx, c.FamilyName, c.GivenName))
	}
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return fmt.Sprintf("%x", h[:8])
}

// mergeCluster merges all contacts in a cluster with iCloud priority.
func mergeCluster(contacts []model.NormalizedContact, indices []int, score float64) model.MergedContact {
	// Find iCloud contact (priority source) and others
	icloudIdx := -1
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
		base.Birthday = normalize.PreferBirthday(base.Birthday, c.Birthday)
		if base.Note == "" && c.Note != "" {
			base.Note = c.Note
		}
		if base.URL == "" && c.URL != "" {
			base.URL = c.URL
		}
		// Image bytes beat a reference (a link can 404); a reference fills
		// an empty slot.
		if len(base.Photo) == 0 {
			if len(c.Photo) > 0 {
				base.Photo = c.Photo
				base.PhotoURI = ""
				base.PhotoType = c.PhotoType
			} else if base.PhotoURI == "" && c.PhotoURI != "" {
				base.PhotoURI = c.PhotoURI
				base.PhotoType = c.PhotoType
			}
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
			// A card has one UID. The priority card's identity is the one
			// the merged card keeps, so a re-import updates that record
			// instead of creating a third contact with two UID lines.
			if strings.EqualFold(k, "UID") {
				if len(base.Extra[k]) == 0 && len(vals) > 0 {
					base.Extra[k] = []string{vals[0]}
				}
				continue
			}
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
