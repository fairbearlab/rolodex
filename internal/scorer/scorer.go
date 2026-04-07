package scorer

import (
	"strings"

	"github.com/xrash/smetrics"

	"github.com/fairbearlab/rolodex/internal/model"
)

const (
	weightName  = 0.40
	weightEmail = 0.25
	weightPhone = 0.25
	weightOrg   = 0.10

	// Weights when name is missing — two shared identifiers should reach auto_merge
	weightEmailNoName = 0.45
	weightPhoneNoName = 0.45
	weightOrgNoName   = 0.10
)

// Score computes candidate pair scores for all blocked pairs.
func Score(contacts []model.NormalizedContact, pairs [][2]int) []model.ScoredPair {
	result := make([]model.ScoredPair, 0, len(pairs))
	for _, p := range pairs {
		score, features := scorePair(contacts[p[0]], contacts[p[1]])
		tier := classify(score)
		result = append(result, model.ScoredPair{
			A:        p[0],
			B:        p[1],
			Score:    score,
			Tier:     tier,
			Features: features,
		})
	}
	return result
}

func scorePair(a, b model.NormalizedContact) (float64, model.ScoreFeatures) {
	nameA := a.NormalizedGivenName
	nameB := b.NormalizedGivenName
	hasName := nameA != "" && nameB != ""

	emailMatch := sharedEmail(a, b)
	phoneMatch := sharedPhone(a, b)
	orgMatch := sharedOrg(a, b)

	if !hasName {
		// Nameless contact: redistribute weights
		// Require at least two matching identifiers to reach auto_merge
		score := 0.0
		if emailMatch {
			score += weightEmailNoName
		}
		if phoneMatch {
			score += weightPhoneNoName
		}
		if orgMatch {
			score += weightOrgNoName
		}

		// Count matching identifiers
		matchCount := 0
		if emailMatch {
			matchCount++
		}
		if phoneMatch {
			matchCount++
		}
		if orgMatch {
			matchCount++
		}
		// Need at least 2 matching identifiers for auto_merge
		if matchCount < 2 && score >= model.ThresholdAutoMerge {
			score = model.ThresholdAutoMerge - 0.01
		}
		features := model.ScoreFeatures{
			NameSimilarity: 0,
			SharedEmail:    emailMatch,
			SharedPhone:    phoneMatch,
			SharedOrg:      orgMatch,
		}
		return score, features
	}

	// Score name similarity with nickname expansion
	// Compare full names (given + family) as the design doc specifies
	fullA := nameA
	fullB := nameB
	if a.NormalizedFamilyName != "" {
		fullA = nameA + " " + a.NormalizedFamilyName
	}
	if b.NormalizedFamilyName != "" {
		fullB = nameB + " " + b.NormalizedFamilyName
	}

	nameSim := nameSimilarity(fullA, fullB)

	score := nameSim * weightName
	if emailMatch {
		score += weightEmail
	}
	if phoneMatch {
		score += weightPhone
	}
	if orgMatch {
		score += weightOrg
	}

	if score > 1.0 {
		score = 1.0
	}
	features := model.ScoreFeatures{
		NameSimilarity: nameSim,
		SharedEmail:    emailMatch,
		SharedPhone:    phoneMatch,
		SharedOrg:      orgMatch,
	}
	return score, features
}

func nameSimilarity(a, b string) float64 {
	// Try direct comparison first
	directScore := smetrics.JaroWinkler(a, b, 0.7, 4)

	// Try with nickname expansion on the given name part
	// For full names like "bob smith", expand "bob" -> "robert" -> "robert smith"
	expandedA := expandFullName(a)
	expandedB := expandFullName(b)
	expandedScore := smetrics.JaroWinkler(expandedA, expandedB, 0.7, 4)

	if expandedScore > directScore {
		return expandedScore
	}
	return directScore
}

// expandFullName expands the first word (given name) if it's a known nickname.
func expandFullName(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return name
	}
	parts[0] = expandName(parts[0])
	return strings.Join(parts, " ")
}

func sharedEmail(a, b model.NormalizedContact) bool {
	for _, ea := range a.NormalizedEmails {
		for _, eb := range b.NormalizedEmails {
			if ea == eb {
				return true
			}
		}
	}
	return false
}

func sharedPhone(a, b model.NormalizedContact) bool {
	for _, pa := range a.NormalizedPhones {
		for _, pb := range b.NormalizedPhones {
			if pa == pb {
				return true
			}
		}
	}
	return false
}

func sharedOrg(a, b model.NormalizedContact) bool {
	orgA := strings.ToLower(strings.TrimSpace(a.Parsed.Org))
	orgB := strings.ToLower(strings.TrimSpace(b.Parsed.Org))
	return orgA != "" && orgB != "" && orgA == orgB
}

func classify(score float64) model.Tier {
	switch {
	case score >= model.ThresholdAutoMerge:
		return model.TierAutoMerge
	case score >= model.ThresholdReview:
		return model.TierReview
	default:
		return model.TierDistinct
	}
}
