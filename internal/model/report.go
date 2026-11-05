package model

// Report is the JSON output explaining every merge decision.
type Report struct {
	Summary  ReportSummary    `json:"summary"`
	Merged   []MergeDecision  `json:"merged"`
	Review   []ReviewDecision `json:"review"`
	Distinct []DistinctEntry  `json:"distinct"`
	// Deferred lists same-name pairs that were neither merged nor reviewed
	// because one side was already merged on a shared identifier; both
	// sides are in the output as separate people.
	Deferred []DeferredPair `json:"deferred"`
	Warnings []Warning      `json:"warnings"`
}

type ReportSummary struct {
	ICloudTotal   int `json:"icloud_total"`
	GoogleTotal   int `json:"google_total"`
	AutoMerged    int `json:"auto_merged"`
	ReviewCount   int `json:"review_count"`
	DistinctCount int `json:"distinct_count"`
	DeferredCount int `json:"deferred_count"`
	WarningCount  int `json:"warning_count"`
}

type MergeDecision struct {
	ClusterID  string       `json:"cluster_id"`
	Score      float64      `json:"score"`
	Contacts   []ContactRef `json:"contacts"`
	Conflicts  []Conflict   `json:"conflicts"`
	ResultName string       `json:"result_name"`
}

type ReviewDecision struct {
	ClusterID string        `json:"cluster_id"`
	Score     float64       `json:"score"`
	Contacts  []ContactRef  `json:"contacts"`
	Features  ScoreFeatures `json:"features,omitzero"`
	Ambiguity string        `json:"ambiguity"`
	Decision  string        `json:"decision"` // "pending", "merge", "skip"
}

// DeferredPair reports a DeferredEdge: the contacts on both sides, the
// strongest same-name edge between them, and why it was not reviewed.
type DeferredPair struct {
	Score    float64      `json:"score"`
	Contacts []ContactRef `json:"contacts"`
	Reason   string       `json:"reason"`
}

type DistinctEntry struct {
	Source Source `json:"source"`
	Name   string `json:"name"`
}

type ContactRef struct {
	Source Source `json:"source"`
	Name   string `json:"name"`
	Index  int    `json:"index"`
}

// Conflict reports one single-value field where two or more members of a
// merged cluster carried a differing value. mergeCluster keeps exactly one
// (Kept, from Winner), and Discarded lists every other value that was lost —
// including a second same-source value a purely-pairwise comparison would
// never see.
type Conflict struct {
	Field     string          `json:"field"`
	Winner    Source          `json:"winner"` // source of the value that was kept
	Kept      string          `json:"kept"`
	Discarded []ConflictValue `json:"discarded"`
}

// ConflictValue is one value findConflicts saw and did not keep, and the
// contact it came from (Source and Index into the run's contact list, so
// the report can point back at the original card).
type ConflictValue struct {
	Source Source `json:"source"`
	Index  int    `json:"index"`
	Value  string `json:"value"`
}

type Warning struct {
	Source  Source `json:"source"`
	Index   int    `json:"index"`
	Message string `json:"message"`
	Raw     string `json:"raw,omitempty"`
}
