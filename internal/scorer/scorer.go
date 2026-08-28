package scorer

import (
	"strings"

	"github.com/xrash/smetrics"

	"github.com/fairbearlab/rolodex/internal/model"
)

// Signal weights. Name/email/phone/org sum to 1.0; birthday is a bonus on
// top and the total is capped at 1.0. The linear score ranks pairs; tier
// assignment also applies the exact-name rules in Classify.
const (
	WeightName     = 0.40
	WeightEmail    = 0.25
	WeightPhone    = 0.25
	WeightOrg      = 0.10
	WeightBirthday = 0.10

	// Weights when name is missing — two shared identifiers should reach auto_merge
	WeightEmailNoName    = 0.45
	WeightPhoneNoName    = 0.45
	WeightOrgNoName      = 0.10
	WeightBirthdayNoName = 0.10
)

// Score computes candidate pair scores for all blocked pairs.
func Score(contacts []model.NormalizedContact, pairs [][2]int) []model.ScoredPair {
	result := make([]model.ScoredPair, 0, len(pairs))
	for _, p := range pairs {
		score, features := scorePair(contacts[p[0]], contacts[p[1]])
		tier := Classify(score, features)
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
	bdayMatch := sharedBirthday(a, b)

	if !hasName {
		// Nameless contact: redistribute weights
		// Require at least two matching identifiers to reach auto_merge
		score := 0.0
		matchCount := 0
		for _, sig := range []struct {
			hit bool
			w   float64
		}{
			{emailMatch, WeightEmailNoName},
			{phoneMatch, WeightPhoneNoName},
			{orgMatch, WeightOrgNoName},
			{bdayMatch, WeightBirthdayNoName},
		} {
			if sig.hit {
				score += sig.w
				matchCount++
			}
		}
		if score > 1.0 {
			score = 1.0
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
			SharedBirthday: bdayMatch,
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

	score := nameSim * WeightName
	if emailMatch {
		score += WeightEmail
	}
	if phoneMatch {
		score += WeightPhone
	}
	if orgMatch {
		score += WeightOrg
	}
	if bdayMatch {
		score += WeightBirthday
	}

	if score > 1.0 {
		score = 1.0
	}
	features := model.ScoreFeatures{
		NameSimilarity: nameSim,
		SharedEmail:    emailMatch,
		SharedPhone:    phoneMatch,
		SharedOrg:      orgMatch,
		SharedBirthday: bdayMatch,
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

// sharedBirthday reports whether both contacts carry the same canonical
// birthday. A no-year value (--MM-DD) matches a full date with the same
// month and day, since iCloud may omit the year that Google keeps.
func sharedBirthday(a, b model.NormalizedContact) bool {
	ba, bb := a.Parsed.Birthday, b.Parsed.Birthday
	if ba == "" || bb == "" {
		return false
	}
	if ba == bb {
		return true
	}
	noYearA, noYearB := strings.HasPrefix(ba, "--"), strings.HasPrefix(bb, "--")
	if noYearA == noYearB {
		return false
	}
	if noYearA {
		return strings.HasSuffix(bb, ba[1:]) // "--06-29" -> "-06-29"
	}
	return strings.HasSuffix(ba, bb[1:])
}

// Classify assigns a tier from the linear score and the feature breakdown.
//
// The score thresholds apply first. On top of them, an effectively identical
// name (similarity >= model.ThresholdExactName) plus one confirming
// identifier — shared phone, email or birthday — is auto_merge even though
// the linear score (0.40 + 0.25 = 0.65) cannot reach the auto_merge
// threshold. An identical name with no confirming identifier is floored at
// review rather than distinct: common names from two sources may be two
// people, which is exactly what the review tier is for.
func Classify(score float64, f model.ScoreFeatures) model.Tier {
	exact := f.ExactName()
	switch {
	case score >= model.ThresholdAutoMerge:
		return model.TierAutoMerge
	case exact && (f.SharedPhone || f.SharedEmail || f.SharedBirthday):
		return model.TierAutoMerge
	case score >= model.ThresholdReview || exact:
		return model.TierReview
	default:
		return model.TierDistinct
	}
}
