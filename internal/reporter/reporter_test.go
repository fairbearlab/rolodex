package reporter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fairbearlab/rolodex/internal/merger"
	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/normalize"
	"github.com/fairbearlab/rolodex/internal/scorer"
)

func TestGenerateAutoMergeCluster(t *testing.T) {
	contacts := []model.NormalizedContact{
		{Parsed: model.ParsedContact{Source: model.SourceICloud, GivenName: "Alice", FamilyName: "Smith", FormattedName: "Alice Smith"}},
		{Parsed: model.ParsedContact{Source: model.SourceGoogle, GivenName: "Alice", FamilyName: "Smith", FormattedName: "Alice Smith"}},
	}

	result := merger.Result{
		Merged: []model.MergedContact{
			{
				Contact:    contacts[0].Parsed,
				Sources:    []model.Source{model.SourceICloud, model.SourceGoogle},
				Score:      0.95,
				MergedFrom: []int{0, 1},
			},
		},
		Clusters: []model.Cluster{
			{
				Indices: []int{0, 1},
				Pairs:   []model.ScoredPair{{A: 0, B: 1, Score: 0.95, Tier: model.TierAutoMerge}},
			},
		},
	}

	report := Generate(contacts, result, 1, 1, nil)

	if report.Summary.AutoMerged != 1 {
		t.Errorf("auto_merged = %d, want 1", report.Summary.AutoMerged)
	}
	if len(report.Merged) != 1 {
		t.Fatalf("expected 1 merge decision, got %d", len(report.Merged))
	}
	if report.Merged[0].Score != 0.95 {
		t.Errorf("merge score = %.2f, want 0.95", report.Merged[0].Score)
	}
	if report.Merged[0].ResultName != "Alice Smith" {
		t.Errorf("result_name = %q, want 'Alice Smith'", report.Merged[0].ResultName)
	}
}

func TestGenerateReviewCluster(t *testing.T) {
	contacts := []model.NormalizedContact{
		{Parsed: model.ParsedContact{Source: model.SourceICloud, GivenName: "Alice", FamilyName: "Smith", FormattedName: "Alice Smith"}},
		{Parsed: model.ParsedContact{Source: model.SourceGoogle, GivenName: "Alicia", FamilyName: "Smith", FormattedName: "Alicia Smith"}},
	}

	result := merger.Result{
		Review: []model.MergedContact{
			{Contact: contacts[0].Parsed, Sources: []model.Source{model.SourceICloud}, Score: 0.72, MergedFrom: []int{0, 1}, ReviewFlag: true},
			{Contact: contacts[1].Parsed, Sources: []model.Source{model.SourceGoogle}, Score: 0.72, MergedFrom: []int{0, 1}, ReviewFlag: true},
		},
		Clusters: []model.Cluster{
			{
				Indices: []int{0, 1},
				Pairs:   []model.ScoredPair{{A: 0, B: 1, Score: 0.72, Tier: model.TierReview}},
			},
		},
	}

	report := Generate(contacts, result, 1, 1, nil)

	if report.Summary.ReviewCount != 1 {
		t.Errorf("review_count = %d, want 1", report.Summary.ReviewCount)
	}
	if len(report.Review) != 1 {
		t.Fatalf("expected 1 review decision, got %d", len(report.Review))
	}
	if report.Review[0].Decision != "pending" {
		t.Errorf("decision = %q, want 'pending'", report.Review[0].Decision)
	}
}

func TestGenerateDistinctOnly(t *testing.T) {
	contacts := []model.NormalizedContact{
		{Parsed: model.ParsedContact{Source: model.SourceICloud, GivenName: "Alice", FormattedName: "Alice"}},
		{Parsed: model.ParsedContact{Source: model.SourceGoogle, GivenName: "Bob", FormattedName: "Bob"}},
	}

	result := merger.Result{
		Merged: []model.MergedContact{
			{Contact: contacts[0].Parsed, Sources: []model.Source{model.SourceICloud}, MergedFrom: []int{0}},
			{Contact: contacts[1].Parsed, Sources: []model.Source{model.SourceGoogle}, MergedFrom: []int{1}},
		},
	}

	report := Generate(contacts, result, 1, 1, nil)

	if report.Summary.DistinctCount != 2 {
		t.Errorf("distinct_count = %d, want 2", report.Summary.DistinctCount)
	}
	if len(report.Distinct) != 2 {
		t.Fatalf("expected 2 distinct entries, got %d", len(report.Distinct))
	}
}

func TestContactNameFallbackChain(t *testing.T) {
	tests := []struct {
		name string
		c    model.ParsedContact
		want string
	}{
		{"formatted name", model.ParsedContact{FormattedName: "Alice Smith"}, "Alice Smith"},
		{"given+family", model.ParsedContact{GivenName: "Alice", FamilyName: "Smith"}, "Alice Smith"},
		{"email fallback", model.ParsedContact{Emails: []model.Email{{Address: "alice@example.com"}}}, "alice@example.com"},
		{"phone fallback", model.ParsedContact{Phones: []model.Phone{{Number: "555-1234"}}}, "555-1234"},
		{"unknown fallback", model.ParsedContact{}, "(unknown)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contactName(tt.c)
			if got != tt.want {
				t.Errorf("contactName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindConflicts(t *testing.T) {
	contacts := []model.NormalizedContact{
		{Parsed: model.ParsedContact{
			Source: model.SourceICloud, FormattedName: "Robert Smith",
			Org: "Acme", Title: "Engineer",
		}},
		{Parsed: model.ParsedContact{
			Source: model.SourceGoogle, FormattedName: "Bob Smith",
			Org: "Acme Corp", Title: "Senior Engineer",
		}},
	}

	conflicts := findConflicts(contacts, []int{0, 1})
	if len(conflicts) < 2 {
		t.Fatalf("expected at least 2 conflicts (FN, TITLE), got %d", len(conflicts))
	}
	for _, c := range conflicts {
		if c.Winner != "icloud" {
			t.Errorf("expected winner=icloud, got %q for field %s", c.Winner, c.Field)
		}
		if c.Kept == "" {
			t.Errorf("field %s: Kept is empty", c.Field)
		}
		if len(c.Discarded) == 0 {
			t.Errorf("field %s: expected at least one discarded value", c.Field)
		}
	}
}

// TestFindConflictsSameSourcePair: a same-source conflict — two iCloud cards
// in one cluster with differing NOTE, plus a Google card that ties them
// together on a shared phone — used to be structurally invisible, because
// the old comparison only ever looked at the first iCloud contact against
// the first non-iCloud one. It must now be caught and both the kept value
// and the discarded one must be reported, naming the contact it came from.
func TestFindConflictsSameSourcePair(t *testing.T) {
	contacts := []model.NormalizedContact{
		{Parsed: model.ParsedContact{
			Source: model.SourceICloud, FormattedName: "John Smith",
			Note:   "Met at conf",
			Phones: []model.Phone{{Number: "5551234567"}},
		}},
		{Parsed: model.ParsedContact{
			Source: model.SourceICloud, FormattedName: "John Smith",
			Note: "Owes me $500",
		}},
		{Parsed: model.ParsedContact{
			Source: model.SourceGoogle, FormattedName: "John Smith",
			Phones: []model.Phone{{Number: "5551234567"}},
		}},
	}

	conflicts := findConflicts(contacts, []int{0, 1, 2})

	var note *model.Conflict
	for i := range conflicts {
		if conflicts[i].Field == "NOTE" {
			note = &conflicts[i]
		}
	}
	if note == nil {
		t.Fatalf("expected a NOTE conflict between the two same-source iCloud cards, got %+v", conflicts)
	}
	if note.Kept != "Met at conf" {
		t.Errorf("kept = %q, want the first iCloud member's note", note.Kept)
	}
	if note.Winner != model.SourceICloud {
		t.Errorf("winner = %q, want icloud", note.Winner)
	}
	if len(note.Discarded) != 1 || note.Discarded[0].Value != "Owes me $500" {
		t.Fatalf("discarded = %+v, want one entry with the second iCloud card's note", note.Discarded)
	}
	if note.Discarded[0].Source != model.SourceICloud {
		t.Errorf("discarded source = %q, want icloud (the same-source card)", note.Discarded[0].Source)
	}
	if note.Discarded[0].Index != 1 {
		t.Errorf("discarded index = %d, want 1 (the second iCloud contact)", note.Discarded[0].Index)
	}
}

// TestFindConflictsDedupesRepeatedDiscardedValue: a third member with the
// same discarded value as the second is not reported twice.
func TestFindConflictsDedupesRepeatedDiscardedValue(t *testing.T) {
	contacts := []model.NormalizedContact{
		{Parsed: model.ParsedContact{Source: model.SourceICloud, FormattedName: "Jane Doe", Org: "Acme"}},
		{Parsed: model.ParsedContact{Source: model.SourceICloud, FormattedName: "Jane Doe", Org: "Acme Corp"}},
		{Parsed: model.ParsedContact{Source: model.SourceGoogle, FormattedName: "Jane Doe", Org: "Acme Corp"}},
	}
	conflicts := findConflicts(contacts, []int{0, 1, 2})
	var org *model.Conflict
	for i := range conflicts {
		if conflicts[i].Field == "ORG" {
			org = &conflicts[i]
		}
	}
	if org == nil {
		t.Fatalf("expected an ORG conflict, got %+v", conflicts)
	}
	if len(org.Discarded) != 1 {
		t.Errorf("discarded = %+v, want the repeated 'Acme Corp' value reported once", org.Discarded)
	}
}

func TestWriteFileRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "report.json")

	report := model.Report{
		Summary: model.ReportSummary{ICloudTotal: 5, GoogleTotal: 3, AutoMerged: 2},
	}

	if err := WriteFile(path, report); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}

	var loaded model.Report
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshaling: %v", err)
	}

	if loaded.Summary.ICloudTotal != 5 {
		t.Errorf("icloud_total = %d, want 5", loaded.Summary.ICloudTotal)
	}
}

func TestGenerateWarnings(t *testing.T) {
	contacts := []model.NormalizedContact{}
	result := merger.Result{}
	warnings := []model.Warning{
		{Source: model.SourceICloud, Index: 0, Message: "malformed entry"},
	}

	report := Generate(contacts, result, 0, 0, warnings)

	if report.Summary.WarningCount != 1 {
		t.Errorf("warning_count = %d, want 1", report.Summary.WarningCount)
	}
	if len(report.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(report.Warnings))
	}
}

// TestFindConflictsComparesBirthdaysCanonically: the report raw-compared the
// BDAY strings, so a pair the scorer had just merged on a shared birthday
// (iCloud `--10-22`, Google `1989-10-22`) was reported with both
// shared_birthday:true and a BDAY conflict. Two dates that agree are not a
// conflict; two that disagree, or free text against a date, still are.
func TestFindConflictsComparesBirthdaysCanonically(t *testing.T) {
	cases := []struct {
		icloud, google string
		wantConflict   bool
	}{
		{"--10-22", "1989-10-22", false},
		{"1989-10-22", "--10-22", false},
		{"1989-10-22", "1989-10-22", false},
		{"1989-10-22", "1990-10-22", true},
		{"--10-22", "1989-10-23", true},
		{"unknown", "1989-10-22", true},
	}
	for _, tc := range cases {
		contacts := []model.NormalizedContact{
			{Parsed: model.ParsedContact{Source: model.SourceICloud, FormattedName: "Jane Doe", Birthday: tc.icloud}},
			{Parsed: model.ParsedContact{Source: model.SourceGoogle, FormattedName: "Jane Doe", Birthday: tc.google}},
		}
		var got bool
		for _, c := range findConflicts(contacts, []int{0, 1}) {
			if c.Field == "BDAY" {
				got = true
			}
		}
		if got != tc.wantConflict {
			t.Errorf("BDAY %q vs %q: conflict reported = %v, want %v", tc.icloud, tc.google, got, tc.wantConflict)
		}
	}
}

// TestGenerateReportsDeferredPairs: a same-name pair the merger left
// unreviewed (one side already merged on a shared phone) appears in the
// report under "deferred" with both sides' contacts, and is counted in the
// summary, so a namesake shipped as a separate person is never silent.
func TestGenerateReportsDeferredPairs(t *testing.T) {
	mk := func(src model.Source, phone string) model.NormalizedContact {
		return normalize.Contact(model.ParsedContact{Source: src, GivenName: "David", FamilyName: "Lee",
			Phones: []model.Phone{{Number: phone}}})
	}
	contacts := []model.NormalizedContact{
		mk(model.SourceICloud, "3175551111"),
		mk(model.SourceGoogle, "4155552222"),
		mk(model.SourceGoogle, "3175551111"),
	}
	var idx [][2]int
	for i := range contacts {
		for j := i + 1; j < len(contacts); j++ {
			idx = append(idx, [2]int{i, j})
		}
	}
	result := merger.Merge(contacts, scorer.Score(contacts, idx))
	report := Generate(contacts, result, 1, 2, nil)

	if report.Summary.ReviewCount != 0 || report.Summary.AutoMerged != 1 || report.Summary.DistinctCount != 1 {
		t.Fatalf("summary = %+v, want 1 auto-merge, 1 distinct, 0 review", report.Summary)
	}
	if report.Summary.DeferredCount != 1 || len(report.Deferred) != 1 {
		t.Fatalf("deferred_count=%d deferred=%+v, want one deferred pair", report.Summary.DeferredCount, report.Deferred)
	}
	d := report.Deferred[0]
	got := map[int]bool{}
	for _, c := range d.Contacts {
		got[c.Index] = true
		if c.Name == "" {
			t.Errorf("deferred contact %d has no name", c.Index)
		}
	}
	if len(got) != 3 || !got[0] || !got[1] || !got[2] {
		t.Errorf("deferred contacts = %+v, want all three David Lees (the cluster and the namesake)", d.Contacts)
	}
	if d.Score != 0.40 || d.Reason == "" {
		t.Errorf("deferred score=%.2f reason=%q, want 0.40 and an explanation", d.Score, d.Reason)
	}
}

// report.json is written with the same staging guarantees as the .vcf
// outputs: a symlink planted at the old "<path>.tmp" is never written
// through, and a directory at the path is refused, not replaced.
func TestWriteFileStagesLikeTheWriter(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim.txt")
	if err := os.WriteFile(victim, []byte("KEEP ME\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "report.json")
	if err := os.Symlink(victim, out+".tmp"); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(out, model.Report{}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Clean(victim)); string(b) != "KEEP ME\n" {
		t.Fatalf("wrote through the planted symlink: %q", b)
	}
	if fi, err := os.Lstat(out); err != nil || !fi.Mode().IsRegular() {
		t.Errorf("report.json is not a regular file: %v %v", fi, err)
	}
	empty := filepath.Join(dir, "reportdir")
	if err := os.Mkdir(empty, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(empty, model.Report{}); err == nil {
		t.Error("a directory at --report was accepted")
	}
	if fi, err := os.Stat(empty); err != nil || !fi.IsDir() {
		t.Errorf("the directory was replaced: %v %v", fi, err)
	}
}
