package review

import (
	"fmt"
	"strings"
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

func mustBuild(t *testing.T, report model.Report, contacts []model.ParsedContact) []ReviewCluster {
	t.Helper()
	clusters, err := BuildClusters(report, contacts)
	if err != nil {
		t.Fatalf("BuildClusters: %v", err)
	}
	return clusters
}

// A review.vcf that does not line up with report.json used to leave the
// short cluster empty without advancing the cursor, so every later cluster
// was handed a different cluster's contacts and the reviewer decided on
// the wrong people. Any length mismatch is now fatal, as in resolve.
func TestBuildClustersRejectsMismatchedReviewFile(t *testing.T) {
	report := makeReport([]float64{0.65, 0.82, 0.71}) // 3 clusters of 2
	if _, err := BuildClusters(report, makeContacts(5)); err == nil {
		t.Error("short review.vcf (5 contacts for 3 pairs) must be rejected")
	}
	if _, err := BuildClusters(report, makeContacts(7)); err == nil {
		t.Error("long review.vcf (7 contacts for 3 pairs) must be rejected")
	}
	if _, err := BuildClusters(report, makeContacts(6)); err != nil {
		t.Errorf("exact review.vcf rejected: %v", err)
	}
	empty := model.Report{Review: []model.ReviewDecision{{ClusterID: "c0", Decision: "pending"}}}
	if _, err := BuildClusters(empty, nil); err == nil {
		t.Error("a zero-contact cluster must be rejected")
	}
}

// Equal length does not prove alignment. A reordered or stale review.vcf with
// the same contact count passed the length check and then handed every cluster
// somebody else's people, and the TUI recorded the decision against the wrong
// cluster id. resolve already refused this; the TUI must refuse it before a
// keystroke is taken.
func TestBuildClustersRejectsClusterIDMismatch(t *testing.T) {
	report := makeReport([]float64{0.65, 0.82}) // 2 clusters of 2
	contacts := makeContacts(4)
	for i := range contacts {
		if contacts[i].Extra == nil {
			contacts[i].Extra = make(map[string][]string)
		}
		contacts[i].Extra["X-ROLODEX-CLUSTER"] = []string{report.Review[0].ClusterID}
	}
	// Contacts 2 and 3 now claim cluster 0 while the report puts them in
	// cluster 1 — the shape a reordered review.vcf produces.
	if _, err := BuildClusters(report, contacts); err == nil {
		t.Error("review.vcf whose cluster tags disagree with report.json must be rejected")
	}

	// Correctly tagged contacts still build.
	for i := range contacts {
		contacts[i].Extra["X-ROLODEX-CLUSTER"] = []string{report.Review[i/2].ClusterID}
	}
	if _, err := BuildClusters(report, contacts); err != nil {
		t.Errorf("correctly tagged review.vcf rejected: %v", err)
	}
}

func TestBuildClustersSortOrder(t *testing.T) {
	report := makeReport([]float64{0.65, 0.82, 0.71})
	contacts := makeContacts(6)

	clusters := mustBuild(t, report, contacts)

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

	clusters := mustBuild(t, report, contacts)

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
	clusters := mustBuild(t, report, contacts)

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
	clusters := mustBuild(t, report, contacts)

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
	clusters := mustBuild(t, report, contacts)
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
	clusters := mustBuild(t, report, contacts)
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
	clusters := mustBuild(t, report, contacts)
	if clusters[0].Contacts[0].Source != model.SourceICloud || clusters[0].Contacts[1].Source != model.SourceGoogle {
		t.Error("per-card X-ROLODEX-SOURCE must take precedence over the report")
	}
}

func TestBirthdayConflictForcesDetailedView(t *testing.T) {
	report := makeReport([]float64{1.0})
	report.Review[0].Features = model.ScoreFeatures{NameSimilarity: 1, NameExact: true, SharedPhone: true, BirthdayConflict: true}
	m := ReviewModel{Clusters: mustBuild(t, report, makeContacts(2)), PairStart: time.Now(), Width: 100, Height: 60}
	if m.ActiveViewMode() != ViewDetailed {
		t.Error("a birthday-conflict pair must get the detailed view even at score 1.00")
	}
	if out := m.View(); !strings.Contains(out, "birthdays disagree") {
		t.Errorf("detailed view should explain the hold:\n%s", out)
	}
}

func TestBirthdayUnknownForcesDetailedView(t *testing.T) {
	report := makeReport([]float64{0.65})
	report.Review[0].Features = model.ScoreFeatures{NameSimilarity: 1, NameExact: true, SharedPhone: true, BirthdayUnknown: true}
	m := ReviewModel{Clusters: mustBuild(t, report, makeContacts(2)), PairStart: time.Now(), Width: 100, Height: 60}
	if m.ActiveViewMode() != ViewDetailed {
		t.Error("a pair held by an unreadable birthday must get the detailed view, which is the only one showing birthdays")
	}
	if out := m.View(); !strings.Contains(out, "could not be read") {
		t.Errorf("detailed view should explain the hold:\n%s", out)
	}
}
