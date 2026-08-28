package review

import (
	"fmt"
	"testing"
	"time"

	"github.com/fairbearlab/rolodex/internal/model"
)

func makeReport(scores []float64) model.Report {
	var reviews []model.ReviewDecision
	for i, s := range scores {
		reviews = append(reviews, model.ReviewDecision{
			ClusterID: fmt.Sprintf("cluster-%d", i),
			Score:     s,
			Contacts: []model.ContactRef{
				{Source: model.SourceICloud, Name: fmt.Sprintf("Contact A%d", i), Index: i * 2},
				{Source: model.SourceGoogle, Name: fmt.Sprintf("Contact B%d", i), Index: i*2 + 1},
			},
			Features: model.ScoreFeatures{NameSimilarity: s * 0.9},
			Decision: "pending",
		})
	}
	return model.Report{Review: reviews}
}

func makeContacts(n int) []model.ParsedContact {
	contacts := make([]model.ParsedContact, n)
	for i := range contacts {
		contacts[i] = model.ParsedContact{
			GivenName:  fmt.Sprintf("Contact%d", i),
			FamilyName: "Test",
			Source:     model.SourceICloud,
		}
	}
	return contacts
}

func TestBuildClustersSortOrder(t *testing.T) {
	report := makeReport([]float64{0.65, 0.82, 0.71})
	contacts := makeContacts(6)

	clusters := BuildClusters(report, contacts)

	if len(clusters) != 3 {
		t.Fatalf("expected 3 clusters, got %d", len(clusters))
	}

	// Should be sorted by score descending
	if clusters[0].Decision.Score != 0.82 {
		t.Errorf("first cluster score = %.2f, want 0.82", clusters[0].Decision.Score)
	}
	if clusters[1].Decision.Score != 0.71 {
		t.Errorf("second cluster score = %.2f, want 0.71", clusters[1].Decision.Score)
	}
	if clusters[2].Decision.Score != 0.65 {
		t.Errorf("third cluster score = %.2f, want 0.65", clusters[2].Decision.Score)
	}
}

func TestBuildClustersTieBreaker(t *testing.T) {
	report := model.Report{
		Review: []model.ReviewDecision{
			{ClusterID: "zzz", Score: 0.70, Contacts: []model.ContactRef{{}, {}}, Decision: "pending"},
			{ClusterID: "aaa", Score: 0.70, Contacts: []model.ContactRef{{}, {}}, Decision: "pending"},
		},
	}
	contacts := makeContacts(4)

	clusters := BuildClusters(report, contacts)

	// Same score, should be sorted by cluster_id asc
	if clusters[0].ClusterID != "aaa" {
		t.Errorf("first cluster ID = %q, want %q", clusters[0].ClusterID, "aaa")
	}
	if clusters[1].ClusterID != "zzz" {
		t.Errorf("second cluster ID = %q, want %q", clusters[1].ClusterID, "zzz")
	}
}

func TestActiveViewMode(t *testing.T) {
	// One pair at the review threshold (shared identifier -> compact) and
	// one below it (exact-name floor -> detailed).
	report := makeReport([]float64{0.65, 0.40})
	contacts := makeContacts(4)
	clusters := BuildClusters(report, contacts)

	m := ReviewModel{
		Clusters:     clusters,
		CurrentIndex: 0,
		PairStart:    time.Now(),
	}

	// High score -> compact
	if m.ActiveViewMode() != ViewCompact {
		t.Error("expected compact mode for score 0.65")
	}

	// Move to low score cluster
	m.CurrentIndex = 1
	if m.ActiveViewMode() != ViewDetailed {
		t.Error("expected detailed mode for score 0.40")
	}

	// Override should take precedence
	v := ViewCompact
	m.ViewOverride = &v
	if m.ActiveViewMode() != ViewCompact {
		t.Error("expected override to compact")
	}
}

func TestAdvanceToNextPending(t *testing.T) {
	report := makeReport([]float64{0.82, 0.75, 0.65})
	contacts := makeContacts(6)
	clusters := BuildClusters(report, contacts)

	// Mark first as merged
	clusters[0].Resolved = "merge"

	m := ReviewModel{
		Clusters:     clusters,
		CurrentIndex: 0,
		PairStart:    time.Now(),
	}

	if !m.AdvanceToNextPending() {
		t.Fatal("expected to find a pending cluster")
	}
	if m.CurrentIndex != 1 {
		t.Errorf("CurrentIndex = %d, want 1", m.CurrentIndex)
	}

	// Mark all as resolved
	for i := range m.Clusters {
		m.Clusters[i].Resolved = "skip"
	}
	if m.AdvanceToNextPending() {
		t.Error("expected no pending clusters")
	}
}

func TestPendingAndResolvedCount(t *testing.T) {
	report := makeReport([]float64{0.82, 0.75, 0.65})
	contacts := makeContacts(6)
	clusters := BuildClusters(report, contacts)
	clusters[0].Resolved = "merge"

	m := ReviewModel{Clusters: clusters}

	if m.PendingCount() != 2 {
		t.Errorf("PendingCount = %d, want 2", m.PendingCount())
	}
	if m.ResolvedCount() != 1 {
		t.Errorf("ResolvedCount = %d, want 1", m.ResolvedCount())
	}
}

func TestBuildClustersFallsBackToReportSource(t *testing.T) {
	report := model.Report{Review: []model.ReviewDecision{{
		ClusterID: "c1",
		Contacts: []model.ContactRef{
			{Source: model.SourceICloud, Name: "A"},
			{Source: model.SourceGoogle, Name: "A"},
		},
		Decision: "pending",
	}}}
	// Contacts parsed without X-ROLODEX-SOURCE carry the loader's label.
	contacts := []model.ParsedContact{
		{Source: "review", FormattedName: "A"},
		{Source: "review", FormattedName: "A"},
	}
	clusters := BuildClusters(report, contacts)
	if got := clusters[0].Contacts[0].Source; got != model.SourceICloud {
		t.Errorf("contact 0 source = %q, want icloud", got)
	}
	if got := clusters[0].Contacts[1].Source; got != model.SourceGoogle {
		t.Errorf("contact 1 source = %q, want google", got)
	}
	if contacts[0].Source != "review" {
		t.Error("BuildClusters must not mutate the caller's slice")
	}
}

func TestBuildClustersKeepsParsedSource(t *testing.T) {
	report := model.Report{Review: []model.ReviewDecision{{
		ClusterID: "c1",
		Contacts:  []model.ContactRef{{Source: model.SourceGoogle}, {Source: model.SourceICloud}},
		Decision:  "pending",
	}}}
	contacts := []model.ParsedContact{{Source: model.SourceICloud}, {Source: model.SourceGoogle}}
	clusters := BuildClusters(report, contacts)
	if clusters[0].Contacts[0].Source != model.SourceICloud || clusters[0].Contacts[1].Source != model.SourceGoogle {
		t.Error("per-card X-ROLODEX-SOURCE must take precedence over the report")
	}
}
