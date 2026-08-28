package model

// Source identifies where a contact came from.
type Source string

const (
	SourceICloud  Source = "icloud"
	SourceGoogle  Source = "google"
	SourceUnknown Source = "unknown"
)

// ParsedContact is the raw canonical representation of a single vCard entry.
type ParsedContact struct {
	Source Source

	// Structured name components
	FamilyName string
	GivenName  string
	MiddleName string
	Prefix     string // Dr., Mr., etc.
	Suffix     string // Jr., III, etc.

	FormattedName string // FN field

	Emails []Email
	Phones []Phone

	Org   string
	Title string

	Birthday string // canonical YYYY-MM-DD or --MM-DD when recognizable, else raw BDAY value

	Addresses []Address
	Note      string
	URL       string

	Photo     []byte // raw PHOTO data
	PhotoType string // e.g. "JPEG", "PNG"

	// Catch-all for fields we don't explicitly model
	Extra map[string][]string

	// Raw vCard text for passthrough on malformed entries
	Raw       string
	Malformed bool
}

type Email struct {
	Address string
	Type    string // HOME, WORK, etc.
}

type Phone struct {
	Number string
	Type   string // CELL, HOME, WORK, etc.
}

type Address struct {
	Type     string
	Street   string
	City     string
	Region   string
	PostCode string
	Country  string
	POBox    string
	Extended string
}

// NormalizedContact extends ParsedContact with normalized forms for matching.
type NormalizedContact struct {
	Parsed ParsedContact

	// Normalized forms used for blocking and scoring
	NormalizedFamilyName string
	NormalizedGivenName  string
	NormalizedMiddleName string
	NormalizedSuffix     string   // generational suffix only (jr, sr, ii, iii, iv, v), from Suffix or the name fields
	NormalizedEmails     []string // lowercased, trimmed
	NormalizedPhones     []string // digits only
}

// ScoreFeatures holds per-feature scores for a scored pair.
type ScoreFeatures struct {
	NameSimilarity float64 `json:"name_similarity"`
	// NameExact is true when the two names identify the same person as far as
	// the name fields can tell: given and family names identical (directly or
	// one being a nickname of the other), middle names compatible, and
	// generational suffixes equal. Stricter than NameSimilarity >= 0.95, which
	// Jaro-Winkler also awards to Eric/Erica, and than normalized equality,
	// which would merge John Smith Jr. with John Smith Sr.
	NameExact   bool `json:"name_exact,omitempty"`
	SharedEmail bool `json:"shared_email"`
	SharedPhone bool `json:"shared_phone"`
	SharedOrg   bool `json:"shared_org"`
	// SharedBirthday is true when both birthdays are present, well-formed
	// and agree; BirthdayConflict when both are well-formed and disagree;
	// BirthdayUnknown when both are present but at least one is not in a
	// form the comparison can read, so neither of the other two can be
	// trusted.
	SharedBirthday   bool `json:"shared_birthday,omitempty"`
	BirthdayConflict bool `json:"birthday_conflict,omitempty"`
	BirthdayUnknown  bool `json:"birthday_unknown,omitempty"`
	// Nameless is true when the pair was scored with the nameless weight table
	// (either contact lacks a given name).
	Nameless bool `json:"nameless,omitempty"`
}

// NearName reports whether the pair's names are near-identical: enough to
// deserve a human look, not enough to merge on.
func (f ScoreFeatures) NearName() bool {
	return f.NameSimilarity >= ThresholdNearName
}

// ScoredPair represents a candidate match between two contacts.
type ScoredPair struct {
	A        int     // index into contact slice
	B        int     // index into contact slice
	Score    float64 // 0.0 to 1.0
	Tier     Tier
	Features ScoreFeatures // per-feature breakdown
}

type Tier string

const (
	TierAutoMerge Tier = "auto_merge"
	TierReview    Tier = "review"
	TierDistinct  Tier = "distinct"
)

// Thresholds for tier classification.
//
// The linear score alone rarely reaches ThresholdAutoMerge on real exports,
// where most contacts carry only a name and one identifier. Two rules sit on
// top of the score thresholds (see scorer.Classify):
//
//   - an identical name (ScoreFeatures.NameExact) plus a shared phone, email
//     or birthday is auto_merge
//   - a near-identical name (similarity >= ThresholdNearName) on its own is at
//     least review, so same-name pairs are surfaced to a human instead of
//     silently dropped
//   - two well-formed birthdays that disagree cap the pair at review: that is
//     the strongest "two different people" signal an export carries
const (
	ThresholdAutoMerge = 0.85
	ThresholdReview    = 0.60
	ThresholdNearName  = 0.95
)

// MergedContact is the output of the merge stage.
type MergedContact struct {
	Contact    ParsedContact
	Sources    []Source // which sources contributed
	Score      float64  // confidence score (0 if unmatched)
	MergedFrom []int    // indices of source contacts that were merged
	ReviewFlag bool     // true if this needs human review
}

// Cluster represents a group of contacts connected by scored pairs.
type Cluster struct {
	Indices []int        // indices into the contact slice
	Pairs   []ScoredPair // all scored pairs in this cluster
}
