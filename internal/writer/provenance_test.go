package writer

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

func writeOne(t *testing.T, mc model.MergedContact) string {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(&buf, []model.MergedContact{mc}); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func countLines(s, prefix string) int {
	n := 0
	for _, line := range strings.Split(s, "\r\n") {
		if strings.HasPrefix(line, prefix) {
			n++
		}
	}
	return n
}

// A card from a foreign export has no provenance to record. Stamping
// X-ROLODEX-SOURCE:unknown on every card of a file prune re-writes would be
// noise the next read-back then treats as a fact.
func TestWriteNoStampForUnknownOrEmptySource(t *testing.T) {
	base := model.ParsedContact{FormattedName: "Foreign Card"}
	for name, sources := range map[string][]model.Source{
		"nil":     nil,
		"empty":   {},
		"unknown": {model.SourceUnknown},
		"blank":   {""},
	} {
		out := writeOne(t, model.MergedContact{Contact: base, Sources: sources})
		if strings.Contains(out, "X-ROLODEX-SOURCE") {
			t.Errorf("%s sources: stamped provenance:\n%s", name, out)
		}
	}
	out := writeOne(t, model.MergedContact{Contact: base, Sources: []model.Source{model.SourceGoogle}})
	if countLines(out, "X-ROLODEX-SOURCE:google") != 1 {
		t.Errorf("known source not stamped once:\n%s", out)
	}
}

// A card read back from rolodex output carries X-ROLODEX-SOURCE in Extra.
// The writer regenerates the line from Sources, so the Extra copy must be
// dropped or every resolve doubles it (measured: 1,053 cards with two lines).
func TestWriteDropsStaleSourceFromExtra(t *testing.T) {
	c := model.ParsedContact{
		FormattedName: "Read Back",
		Extra:         map[string][]string{"X-ROLODEX-SOURCE": {"google"}},
	}
	out := writeOne(t, model.MergedContact{Contact: c, Sources: []model.Source{model.SourceICloud, model.SourceGoogle}})
	if n := countLines(out, "X-ROLODEX-SOURCE"); n != 1 {
		t.Errorf("want exactly one X-ROLODEX-SOURCE line, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "X-ROLODEX-SOURCE:merged(icloud+google)\r\n") {
		t.Errorf("provenance should come from Sources, not Extra:\n%s", out)
	}
	// With no Sources at all the stale line is dropped too, not passed through.
	out = writeOne(t, model.MergedContact{Contact: c})
	if strings.Contains(out, "X-ROLODEX-SOURCE") {
		t.Errorf("stale Extra provenance passed through with no Sources:\n%s", out)
	}
}

// SCORE and REVIEW in Extra are replaced only when the writer emits its own
// value; a card that arrives with them and no new score keeps them.
func TestWriteScoreAndReviewNotDoubled(t *testing.T) {
	c := model.ParsedContact{
		FormattedName: "Scored",
		Extra: map[string][]string{
			"X-ROLODEX-SCORE":  {"0.50"},
			"X-ROLODEX-REVIEW": {"true"},
		},
	}
	out := writeOne(t, model.MergedContact{Contact: c, Score: 0.91, ReviewFlag: true})
	if n := countLines(out, "X-ROLODEX-SCORE:"); n != 1 || !strings.Contains(out, "X-ROLODEX-SCORE:0.91\r\n") {
		t.Errorf("want one SCORE line with the new value, got %d:\n%s", n, out)
	}
	if n := countLines(out, "X-ROLODEX-REVIEW:"); n != 1 {
		t.Errorf("want one REVIEW line, got %d:\n%s", n, out)
	}

	out = writeOne(t, model.MergedContact{Contact: c})
	if !strings.Contains(out, "X-ROLODEX-SCORE:0.50\r\n") || countLines(out, "X-ROLODEX-REVIEW:true") != 1 {
		t.Errorf("without a new score the Extra values must pass through:\n%s", out)
	}
}

// The merger sets X-ROLODEX-CLUSTER in Extra on purpose; it still passes.
func TestWriteClusterPassesThrough(t *testing.T) {
	c := model.ParsedContact{
		FormattedName: "Clustered",
		Extra:         map[string][]string{"X-ROLODEX-CLUSTER": {"abc123"}},
	}
	out := writeOne(t, model.MergedContact{Contact: c, Sources: []model.Source{model.SourceICloud}, ReviewFlag: true})
	if countLines(out, "X-ROLODEX-CLUSTER:abc123") != 1 {
		t.Errorf("cluster id dropped:\n%s", out)
	}
}
