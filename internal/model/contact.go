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

	Birthday string // raw BDAY value

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
	NormalizedEmails     []string // lowercased, trimmed
	NormalizedPhones     []string // digits only
}

// ScoreFeatures holds per-feature scores for a scored pair.
type ScoreFeatures struct {
	NameSimilarity float64 `json:"name_similarity"`
	SharedEmail    bool    `json:"shared_email"`
	SharedPhone    bool    `json:"shared_phone"`
	SharedOrg      bool    `json:"shared_org"`
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
const (
	ThresholdAutoMerge = 0.85
	ThresholdReview    = 0.60
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
