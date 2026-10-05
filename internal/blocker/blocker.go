package blocker

import (
	"sort"
	"strings"

	"github.com/fairbearlab/rolodex/internal/model"
)

// maxBlockSize caps every blocking bucket. Pairing a bucket in full is
// O(k^2): one phone shared by 4,000 contacts (a switchboard, a family
// landline, a placeholder like 000-000-0000 that some exports emit) was
// 7,998,000 candidate pairs and 3.99 GB, and ~8,000 was an OOM kill. The
// shared value is no evidence of identity at that size anyway.
const maxBlockSize = 50

// Truncation reports a blocking bucket that was over maxBlockSize and so was
// not paired in full. Its members are paired only within a smaller
// sub-bucket — the same first initial, or the same organization — and a
// sub-bucket that is itself over the cap is dropped.
type Truncation struct {
	Kind     string // "email", "phone" or "last name"
	Key      string // the shared normalized value
	Size     int    // contacts in the bucket
	Unpaired int    // members the filter gave no candidate pair from this bucket
}

// Block groups contacts into candidate match sets using cheap blocking keys.
// Returns pairs of indices that should be scored, and the buckets that were
// too large to pair in full.
func Block(contacts []model.NormalizedContact) ([][2]int, []Truncation) {
	pairSet := make(map[[2]int]bool)
	var truncated []Truncation

	emailIndex := make(map[string][]int)
	phoneIndex := make(map[string][]int)
	lastNameIndex := make(map[string][]int)
	for i, c := range contacts {
		for _, e := range c.NormalizedEmails {
			emailIndex[e] = append(emailIndex[e], i)
		}
		for _, p := range c.NormalizedPhones {
			phoneIndex[p] = append(phoneIndex[p], i)
		}
		if ln := c.NormalizedFamilyName; ln != "" {
			lastNameIndex[ln] = append(lastNameIndex[ln], i)
		}
	}
	for _, idx := range []struct {
		kind    string
		buckets map[string][]int
	}{{"email", emailIndex}, {"phone", phoneIndex}, {"last name", lastNameIndex}} {
		for key, indices := range idx.buckets {
			if len(indices) <= maxBlockSize {
				addPairs(indices, pairSet)
				continue
			}
			unpaired := addFilteredPairs(indices, contacts, pairSet)
			truncated = append(truncated, Truncation{Kind: idx.kind, Key: key, Size: len(indices), Unpaired: unpaired})
		}
	}

	// Convert set to slice with deterministic ordering
	pairs := make([][2]int, 0, len(pairSet))
	for p := range pairSet {
		pairs = append(pairs, p)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i][0] != pairs[j][0] {
			return pairs[i][0] < pairs[j][0]
		}
		return pairs[i][1] < pairs[j][1]
	})
	sort.Slice(truncated, func(i, j int) bool {
		if truncated[i].Size != truncated[j].Size {
			return truncated[i].Size > truncated[j].Size
		}
		if truncated[i].Kind != truncated[j].Kind {
			return truncated[i].Kind < truncated[j].Kind
		}
		return truncated[i].Key < truncated[j].Key
	})
	return pairs, truncated
}

func addPairs(indices []int, pairSet map[[2]int]bool) {
	for i := 0; i < len(indices); i++ {
		for j := i + 1; j < len(indices); j++ {
			a, b := indices[i], indices[j]
			if a > b {
				a, b = b, a
			}
			pairSet[[2]int{a, b}] = true
		}
	}
}

// addFilteredPairs pairs an oversized bucket's members only where their
// first initials match or they share an organization. It sub-buckets by
// those keys in one pass rather than testing every pair, so the cost stays
// linear in the bucket, and a sub-bucket that is itself over the cap is
// dropped. Returns how many members got no pair.
func addFilteredPairs(indices []int, contacts []model.NormalizedContact, pairSet map[[2]int]bool) int {
	byInitial := make(map[string][]int)
	byOrg := make(map[string][]int)
	for _, i := range indices {
		if in := firstInitial(contacts[i].NormalizedGivenName); in != "" {
			byInitial[in] = append(byInitial[in], i)
		}
		if org := orgKey(contacts[i]); org != "" {
			byOrg[org] = append(byOrg[org], i)
		}
	}
	paired := make(map[int]bool)
	for _, subs := range []map[string][]int{byInitial, byOrg} {
		for _, sub := range subs {
			if len(sub) < 2 || len(sub) > maxBlockSize {
				continue
			}
			addPairs(sub, pairSet)
			for _, i := range sub {
				paired[i] = true
			}
		}
	}
	return len(indices) - len(paired)
}

func firstInitial(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return string([]rune(name)[0:1])
}

func orgKey(c model.NormalizedContact) string {
	return strings.ToLower(strings.TrimSpace(c.Parsed.Org))
}
