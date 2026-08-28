package scorer

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

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
	bdayConflict := birthdayConflict(a, b)
	bdayUnknown := birthdayUnknown(a, b)

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
		// Need at least 2 matching identifiers for auto_merge. With the
		// current weights the largest single signal is 0.45, so one match
		// cannot reach the 0.85 threshold and this cannot fire — it is kept
		// so that raising a nameless weight can never silently break the
		// two-identifier rule.
		if matchCount < 2 && score >= model.ThresholdAutoMerge {
			score = model.ThresholdAutoMerge - 0.01
		}
		features := model.ScoreFeatures{
			NameSimilarity:   0,
			SharedEmail:      emailMatch,
			SharedPhone:      phoneMatch,
			SharedOrg:        orgMatch,
			SharedBirthday:   bdayMatch,
			BirthdayConflict: bdayConflict,
			BirthdayUnknown:  bdayUnknown,
			Nameless:         true,
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
	nameExact := sameName(a, b)

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
		NameSimilarity:   nameSim,
		NameExact:        nameExact,
		SharedEmail:      emailMatch,
		SharedPhone:      phoneMatch,
		SharedOrg:        orgMatch,
		SharedBirthday:   bdayMatch,
		BirthdayConflict: bdayConflict,
		BirthdayUnknown:  bdayUnknown,
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

// canonicalBirthdayRe matches the forms normalize.Birthday produces
// (YYYY-MM-DD or --MM-DD). Only those are trusted as evidence in either
// direction: anything else that survived normalization is free text.
var canonicalBirthdayRe = regexp.MustCompile(`^(\d{4}|-)-(\d{2})-(\d{2})$`)

// parseBirthday splits a canonical birthday into its year ("" when the year
// is unknown) and month-day. ok is false for anything non-canonical,
// including a placeholder that merely looks canonical ("0000-00-00"): the
// normalizer passes those through untouched, so the range check is repeated
// here rather than trusted.
func parseBirthday(s string) (year, monthDay string, ok bool) {
	m := canonicalBirthdayRe.FindStringSubmatch(s)
	if m == nil {
		return "", "", false
	}
	month, _ := strconv.Atoi(m[2])
	day, _ := strconv.Atoi(m[3])
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return "", "", false
	}
	if m[1] != "-" {
		year = m[1]
	}
	return year, m[2] + "-" + m[3], true
}

// sharedBirthday reports whether both contacts carry the same well-formed
// birthday. A no-year value (--MM-DD) matches a full date with the same
// month and day, since iCloud may omit the year that Google keeps. Two
// equal raw strings are not enough: "1989" == "1989" or "unknown" ==
// "unknown" is not a shared birthday, and this feature promotes an
// identical name to auto_merge on its own.
func sharedBirthday(a, b model.NormalizedContact) bool {
	yearA, mdA, okA := parseBirthday(a.Parsed.Birthday)
	yearB, mdB, okB := parseBirthday(b.Parsed.Birthday)
	if !okA || !okB || mdA != mdB {
		return false
	}
	return yearA == "" || yearB == "" || yearA == yearB
}

// birthdayConflict reports whether both contacts carry a well-formed birthday
// and the two disagree. Two people with the same name and a shared household
// phone are told apart by exactly this.
func birthdayConflict(a, b model.NormalizedContact) bool {
	_, _, okA := parseBirthday(a.Parsed.Birthday)
	_, _, okB := parseBirthday(b.Parsed.Birthday)
	if !okA || !okB {
		return false
	}
	return !sharedBirthday(a, b)
}

// birthdayUnknown reports whether both contacts carry a birthday but at
// least one is not in a form the conflict check can read. The guard cannot
// run, so Classify must not lean on it: an unreadable birthday is "unknown",
// never "no conflict".
func birthdayUnknown(a, b model.NormalizedContact) bool {
	ba, bb := a.Parsed.Birthday, b.Parsed.Birthday
	if ba == "" || bb == "" {
		return false
	}
	_, _, okA := parseBirthday(ba)
	_, _, okB := parseBirthday(bb)
	return !okA || !okB
}

// sameName reports whether two names identify the same person as far as the
// name fields can tell. It is the gate for the auto-merge rule, so it is
// deliberately stricter than the similarity score:
//
//   - given names equal, or one a nickname of the other (Chris/Christopher);
//     two different diminutives of one canonical (Ted/Ned, Beth/Betty) are
//     usually siblings, not one person, and do not count
//   - family names equal
//   - middle names compatible: equal, an initial matching the other's first
//     letter, or absent on one side (Charles J. Galanti / Charles Galanti)
//   - generational suffixes equal, including absent on both sides; Jr. vs
//     Sr. — or Jr. vs nothing — is a father and son on one landline
func sameName(a, b model.NormalizedContact) bool {
	// Positive name evidence is required. Two contacts with no family name
	// do not share one, and a single-letter given name is an initial, not a
	// name: "Alex" / "Alex" or "J. Smith" / "J. Smith" on a shared office
	// switchboard are as likely two people as one. Such pairs still reach
	// review through the near-name floor; they cannot auto-merge on one
	// identifier. (A one-character CJK given name is also excluded here;
	// that costs recall on the auto-merge rule, never precision.)
	if a.NormalizedFamilyName == "" && b.NormalizedFamilyName == "" {
		return false
	}
	if a.NormalizedFamilyName != b.NormalizedFamilyName {
		return false
	}
	if isInitial(a.NormalizedGivenName) || isInitial(b.NormalizedGivenName) {
		return false
	}
	if a.NormalizedSuffix != b.NormalizedSuffix {
		return false
	}
	// The names must agree with diacritics folded away AND with them kept.
	// Name() drops combining marks, so "Nguyên" and "Nguyễn" — two different
	// Vietnamese names — both fold to "nguyen"; on the folded form alone the
	// exact-name rule auto-merged two household members who shared only a
	// landline. Comparing the accent-preserving form too costs recall on one
	// shape (an export that lost the accent), and that pair still reaches
	// review through the near-name floor. It never costs precision.
	return sameNameParts(a.NormalizedFamilyName, a.NormalizedGivenName, a.NormalizedMiddleName,
		b.NormalizedFamilyName, b.NormalizedGivenName, b.NormalizedMiddleName) &&
		sameNameParts(a.StrictFamilyName, a.StrictGivenName, a.StrictMiddleName,
			b.StrictFamilyName, b.StrictGivenName, b.StrictMiddleName)
}

// sameNameParts compares one normalization of two names. Both the folded and
// the accent-preserving forms are run through it, so a signal that can confirm
// identity is held to the same standard in both.
func sameNameParts(familyA, givenA0, middleA0, familyB, givenB0, middleB0 string) bool {
	if familyA != familyB {
		return false
	}
	// Google folds the middle name into the given name (N:Doe;John V;;;)
	// where iCloud uses the middle slot (N:Doe;John;V;;). When the middle
	// slot is empty, trailing given-name tokens are compared as the middle
	// name so the two shapes of one person agree.
	givenA, middleA := splitGiven(givenA0, middleA0)
	givenB, middleB := splitGiven(givenB0, middleB0)
	if !compatibleMiddle(middleA, middleB) {
		return false
	}
	return sameGivenName(givenA, givenB)
}

// splitGiven moves trailing given-name tokens into the middle name when the
// middle slot is empty; otherwise both are returned unchanged.
func splitGiven(given, middle string) (string, string) {
	words := strings.Fields(given)
	if middle != "" || len(words) < 2 {
		return given, middle
	}
	return words[0], strings.Join(words[1:], " ")
}

func sameGivenName(ga, gb string) bool {
	if ga == gb {
		return true
	}
	wa, wb := strings.Fields(ga), strings.Fields(gb)
	if len(wa) == 0 || len(wb) == 0 || len(wa) != len(wb) {
		return false
	}
	for i := 1; i < len(wa); i++ {
		if wa[i] != wb[i] {
			return false
		}
	}
	if expandName(wa[0]) != expandName(wb[0]) {
		return false
	}
	_, aNick := nicknames[wa[0]]
	_, bNick := nicknames[wb[0]]
	return !aNick || !bNick // not two distinct diminutives of one canonical
}

// isInitial reports whether a given name carries no more identity than a set
// of initials: "J", "J.", but also "J.R." and "J R". Counting runes on the
// whole string was not enough — "j.r." is three runes, so it passed the guard,
// and two different "J.R. Smith"s on one office switchboard auto-merged with
// no review card. A name is initials when every token in it is a single
// letter.
func isInitial(given string) bool {
	fields := strings.FieldsFunc(given, func(r rune) bool {
		return r == '.' || r == ' ' || r == '\t'
	})
	if len(fields) == 0 {
		return true
	}
	for _, f := range fields {
		if utf8.RuneCountInString(f) > 1 {
			return false
		}
	}
	return true
}

func compatibleMiddle(ma, mb string) bool {
	if ma == "" || mb == "" || ma == mb {
		return true
	}
	// Compare runes, not bytes: a single-rune initial can be multi-byte
	// ("Ö"), and byte-slicing would both miss that match and split the rune.
	ra := []rune(strings.Trim(ma, "."))
	rb := []rune(strings.Trim(mb, "."))
	if len(ra) == 0 || len(rb) == 0 {
		// A punctuation-only middle name ("." is a common export
		// placeholder) carries no information — treat it as absent.
		return true
	}
	if len(ra) == 1 || len(rb) == 1 {
		return ra[0] == rb[0]
	}
	return false
}

// Classify assigns a tier from the linear score and the feature breakdown.
//
// The score thresholds apply first. On top of them, an identical name
// (NameExact: equal after normalization, directly or via nickname expansion)
// plus one confirming identifier — shared phone, email or birthday — is
// auto_merge even though the linear score for that shape (0.40 + 0.25 =
// 0.65) cannot reach the auto_merge threshold. A merely near-identical
// name (Jaro-Winkler >= ThresholdNearName, which Eric/Erica also clears) is
// floored at review rather than distinct, as is an identical name with no
// confirming identifier: common names from two sources may be two people,
// which is exactly what the review tier is for. Two well-formed birthdays
// that disagree cap any pair at review.
//
// The exact-name rule merges on a single identifier, which is precisely the
// shape the birthday guard exists for (parent and child on one landline).
// When both contacts carry a birthday but one cannot be read, the guard
// cannot run, so the rule does not fire and the pair falls through to the
// score thresholds — fail closed, not open.
//
// Note the deliberate asymmetry with BirthdayConflict, which caps both paths:
// an UNKNOWN birthday withholds only the single-identifier shortcut. A pair
// that clears the auto-merge threshold on its own (two shared identifiers)
// still merges. See TestBirthdayGuardFailsClosed.
func Classify(score float64, f model.ScoreFeatures) model.Tier {
	confirmed := f.SharedPhone || f.SharedEmail || f.SharedBirthday
	nameRule := f.NameExact && confirmed && !f.BirthdayUnknown
	autoMerge := score >= model.ThresholdAutoMerge || nameRule
	switch {
	case autoMerge && !f.BirthdayConflict:
		return model.TierAutoMerge
	case autoMerge || score >= model.ThresholdReview || f.NearName():
		return model.TierReview
	default:
		return model.TierDistinct
	}
}
