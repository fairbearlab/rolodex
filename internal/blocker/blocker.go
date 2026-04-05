package blocker

import (
	"strings"

	"github.com/fairbearlabs/rolodex/internal/model"
)

const maxLastNameBlockSize = 50

// Block groups contacts into candidate match sets using cheap blocking keys.
// Returns pairs of indices that should be scored.
func Block(contacts []model.NormalizedContact) [][2]int {
	pairSet := make(map[[2]int]bool)

	// Block by shared normalized email
	emailIndex := make(map[string][]int)
	for i, c := range contacts {
		for _, e := range c.NormalizedEmails {
			emailIndex[e] = append(emailIndex[e], i)
		}
	}
	for _, indices := range emailIndex {
		addPairs(indices, pairSet)
	}

	// Block by shared normalized phone
	phoneIndex := make(map[string][]int)
	for i, c := range contacts {
		for _, p := range c.NormalizedPhones {
			phoneIndex[p] = append(phoneIndex[p], i)
		}
	}
	for _, indices := range phoneIndex {
		addPairs(indices, pairSet)
	}

	// Block by shared last name
	lastNameIndex := make(map[string][]int)
	for i, c := range contacts {
		ln := c.NormalizedFamilyName
		if ln != "" {
			lastNameIndex[ln] = append(lastNameIndex[ln], i)
		}
	}
	for _, indices := range lastNameIndex {
		if len(indices) <= maxLastNameBlockSize {
			addPairs(indices, pairSet)
		} else {
			// Secondary filter: retain only pairs where first initials match
			// or contacts share an organization
			addFilteredPairs(indices, contacts, pairSet)
		}
	}

	// Convert set to slice
	pairs := make([][2]int, 0, len(pairSet))
	for p := range pairSet {
		pairs = append(pairs, p)
	}
	return pairs
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

func addFilteredPairs(indices []int, contacts []model.NormalizedContact, pairSet map[[2]int]bool) {
	for i := 0; i < len(indices); i++ {
		for j := i + 1; j < len(indices); j++ {
			a, b := indices[i], indices[j]
			if a > b {
				a, b = b, a
			}
			if pairSet[[2]int{a, b}] {
				continue // already added by another block
			}
			if firstInitialsMatch(contacts[a], contacts[b]) || shareOrg(contacts[a], contacts[b]) {
				pairSet[[2]int{a, b}] = true
			}
		}
	}
}

func firstInitialsMatch(a, b model.NormalizedContact) bool {
	ai := firstInitial(a.NormalizedGivenName)
	bi := firstInitial(b.NormalizedGivenName)
	return ai != "" && bi != "" && ai == bi
}

func firstInitial(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return string([]rune(name)[0:1])
}

func shareOrg(a, b model.NormalizedContact) bool {
	orgA := strings.ToLower(strings.TrimSpace(a.Parsed.Org))
	orgB := strings.ToLower(strings.TrimSpace(b.Parsed.Org))
	return orgA != "" && orgB != "" && orgA == orgB
}
