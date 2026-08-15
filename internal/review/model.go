package review

import (
	"sort"
	"time"

	"github.com/fairbearlab/rolodex/internal/calibration"
	"github.com/fairbearlab/rolodex/internal/model"
)

// CompactThreshold is the score above which compact mode is used.
const CompactThreshold = 0.78

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
func BuildClusters(report model.Report, reviewContacts []model.ParsedContact) []ReviewCluster {
	var clusters []ReviewCluster
	contactIdx := 0

	for _, rd := range report.Review {
		clusterSize := len(rd.Contacts)
		var contacts []model.ParsedContact
		if contactIdx+clusterSize <= len(reviewContacts) {
			contacts = reviewContacts[contactIdx : contactIdx+clusterSize]
			contactIdx += clusterSize
		}

		clusters = append(clusters, ReviewCluster{
			ClusterID: rd.ClusterID,
			Decision:  rd,
			Contacts:  contacts,
			Features:  rd.Features,
			Resolved:  rd.Decision,
		})
	}

	// Sort: score desc, then cluster_id asc for stability
	sort.SliceStable(clusters, func(i, j int) bool {
		if clusters[i].Decision.Score != clusters[j].Decision.Score {
			return clusters[i].Decision.Score > clusters[j].Decision.Score
		}
		return clusters[i].ClusterID < clusters[j].ClusterID
	})

	return clusters
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
