package review

import (
	"fmt"
	"sort"
	"time"

	"github.com/fairbearlab/rolodex/internal/calibration"
	"github.com/fairbearlab/rolodex/internal/model"
)

// CompactThreshold is the score at or above which the compact card is used.
//
// Pairs whose linear score reaches the review threshold carry a shared
// identifier (phone or email) alongside a near-match name, so a one-glance
// card is enough. Pairs below it were surfaced by the near-name rule with
// no confirming identifier and need the full field-by-field view. Pairs
// held in review by a birthday conflict always get the detailed view,
// whatever their score: the compact card has no birthday row.
const CompactThreshold = model.ThresholdReview

// ViewMode controls which layout to render.
type ViewMode int

const (
	ViewCompact ViewMode = iota
	ViewDetailed
)

// String returns the view mode as a string for calibration logging.
func (v ViewMode) String() string {
	if v == ViewDetailed {
		return "detailed"
	}
	return "compact"
}

// ReviewCluster holds everything needed to render and decide on one review-tier cluster.
type ReviewCluster struct {
	ClusterID string
	Decision  model.ReviewDecision
	Contacts  []model.ParsedContact
	Features  model.ScoreFeatures
	Resolved  string // "pending", "merge", or "skip"
}

// UserDecision records one decision from the review session.
type UserDecision struct {
	ClusterIndex int
	Choice       string // "merge" or "skip"
	Timestamp    time.Time
	DecisionMs   int64
	ViewMode     ViewMode
}

// ReviewModel is the BubbleTea model for the interactive review.
type ReviewModel struct {
	Report       model.Report
	Clusters     []ReviewCluster
	CurrentIndex int
	Decisions    []UserDecision // stack for undo
	CalLog       *calibration.Log
	Width        int
	Height       int
	ViewOverride *ViewMode // non-nil if user toggled with 'd'
	ShowHelp     bool
	Done         bool
	StartTime    time.Time
	PairStart    time.Time // when current pair was shown
	ScrollOffset int       // for scrolling in detailed view
	ReportPath   string    // path to report.json for persistence
	LastError    error     // last persistence error, shown in status bar
}

// BuildClusters constructs ReviewClusters from a report and review contacts.
// Clusters are sorted by score desc, then cluster_id asc (deterministic tie-breaker).
// Review contacts in the slice correspond sequentially to report.Review clusters.
//
// The two files must agree exactly. If review.vcf is short, the position
// of every later cluster is wrong and the reviewer would decide on the
// wrong people; if it is long, the report is stale. Either is fatal here,
// as it already is in resolve.
func BuildClusters(report model.Report, reviewContacts []model.ParsedContact) ([]ReviewCluster, error) {
	var clusters []ReviewCluster
	contactIdx := 0

	for _, rd := range report.Review {
		clusterSize := len(rd.Contacts)
		if clusterSize == 0 {
			return nil, fmt.Errorf("review cluster %s has zero contacts — report.json may be corrupt", rd.ClusterID)
		}
		if contactIdx+clusterSize > len(reviewContacts) {
			return nil, fmt.Errorf("report references more review contacts than exist in review.vcf (cluster %s needs %d, %d left)",
				rd.ClusterID, clusterSize, len(reviewContacts)-contactIdx)
		}
		contacts := append([]model.ParsedContact(nil), reviewContacts[contactIdx:contactIdx+clusterSize]...)
		contactIdx += clusterSize

		// Length alone does not prove alignment: a reordered or stale
		// review.vcf with the same contact count passes the check above and
		// then hands every cluster somebody else's people. The decision would
		// be recorded against the wrong cluster id. resolve already refuses
		// this; the TUI must refuse it before a keystroke is taken.
		for _, c := range contacts {
			if ids, ok := c.Extra["X-ROLODEX-CLUSTER"]; ok && len(ids) > 0 && ids[0] != rd.ClusterID {
				return nil, fmt.Errorf("cluster ID mismatch: review.vcf contact has cluster %s but report expects %s (review.vcf may have been reordered)",
					ids[0], rd.ClusterID)
			}
		}

		// The parser restores Source from X-ROLODEX-SOURCE in review.vcf. If
		// that is missing (older files, hand-edited input) fall back to the
		// provenance the report recorded for the same position.
		for i := range contacts {
			if !isKnownSource(contacts[i].Source) && i < len(rd.Contacts) && isKnownSource(rd.Contacts[i].Source) {
				contacts[i].Source = rd.Contacts[i].Source
			}
		}

		clusters = append(clusters, ReviewCluster{
			ClusterID: rd.ClusterID,
			Decision:  rd,
			Contacts:  contacts,
			Features:  rd.Features,
			Resolved:  rd.Decision,
		})
	}

	if contactIdx < len(reviewContacts) {
		return nil, fmt.Errorf("review.vcf has %d contacts but report only references %d — report.json is stale",
			len(reviewContacts), contactIdx)
	}

	// Sort: score desc, then cluster_id asc for stability
	sort.SliceStable(clusters, func(i, j int) bool {
		if clusters[i].Decision.Score != clusters[j].Decision.Score {
			return clusters[i].Decision.Score > clusters[j].Decision.Score
		}
		return clusters[i].ClusterID < clusters[j].ClusterID
	})

	return clusters, nil
}

func isKnownSource(s model.Source) bool {
	return s == model.SourceICloud || s == model.SourceGoogle
}

// CurrentCluster returns the cluster currently being reviewed, or nil if done.
func (m *ReviewModel) CurrentCluster() *ReviewCluster {
	if m.CurrentIndex >= len(m.Clusters) {
		return nil
	}
	return &m.Clusters[m.CurrentIndex]
}

// PendingCount returns the number of clusters still pending.
func (m *ReviewModel) PendingCount() int {
	count := 0
	for _, c := range m.Clusters {
		if c.Resolved == "pending" {
			count++
		}
	}
	return count
}

// ResolvedCount returns the number of clusters already decided.
func (m *ReviewModel) ResolvedCount() int {
	return len(m.Clusters) - m.PendingCount()
}

// ActiveViewMode returns the effective view mode for the current cluster.
func (m *ReviewModel) ActiveViewMode() ViewMode {
	if m.ViewOverride != nil {
		return *m.ViewOverride
	}
	c := m.CurrentCluster()
	if c == nil {
		return ViewCompact
	}
	// Multi-contact clusters always use detailed view
	if len(c.Contacts) > 2 {
		return ViewDetailed
	}
	// A birthday conflict is the reason the pair is here, and an unreadable
	// birthday is what kept it out of auto_merge; only the detailed view
	// shows the birthdays.
	if c.Features.BirthdayConflict || c.Features.BirthdayUnknown {
		return ViewDetailed
	}
	if c.Decision.Score >= CompactThreshold {
		return ViewCompact
	}
	return ViewDetailed
}

// AdvanceToNextPending moves CurrentIndex to the next pending cluster.
// Returns false if no pending clusters remain.
func (m *ReviewModel) AdvanceToNextPending() bool {
	for i := m.CurrentIndex; i < len(m.Clusters); i++ {
		if m.Clusters[i].Resolved == "pending" {
			m.CurrentIndex = i
			m.ViewOverride = nil
			m.ScrollOffset = 0
			m.PairStart = time.Now()
			return true
		}
	}
	// Wrap around and check from beginning
	for i := 0; i < m.CurrentIndex; i++ {
		if m.Clusters[i].Resolved == "pending" {
			m.CurrentIndex = i
			m.ViewOverride = nil
			m.ScrollOffset = 0
			m.PairStart = time.Now()
			return true
		}
	}
	return false
}
