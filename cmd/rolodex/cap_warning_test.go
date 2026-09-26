package main

import (
	"strconv"
	"strings"
	"testing"

	"github.com/fairbearlab/rolodex/internal/blocker"
	"github.com/fairbearlab/rolodex/internal/merger"
	"github.com/fairbearlab/rolodex/internal/model"
)

func TestReportTruncatedBuckets(t *testing.T) {
	stderr := captureStderr(t, func() {
		reportTruncatedBuckets([]blocker.Truncation{{Kind: "phone", Key: "0000000000", Size: 4000, Unpaired: 3990}})
	})
	want := `warning: 4000 contacts share the phone "0000000000", too many to compare in full; ` +
		"only those with the same first initial or organization were compared (3990 not compared)"
	if !strings.Contains(stderr, want) {
		t.Errorf("stderr lacks %q:\n%s", want, stderr)
	}
}

func TestReportOversizedClusters(t *testing.T) {
	contacts := make([]model.NormalizedContact, 12)
	indices := make([]int, len(contacts))
	for i := range contacts {
		contacts[i].Parsed.FormattedName = string(rune('A' + i))
		indices[i] = i
	}
	stderr := captureStderr(t, func() {
		reportOversizedClusters(contacts, []model.Cluster{{Indices: indices}, {Indices: []int{0, 1}}})
	})
	for _, want := range []string{
		"warning: 12 contacts are linked by shared emails or phones",
		"kept as separate people, not merged or reviewed: A, B, C and 9 more\n",
		"reviewed: A, B\n",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
	if !strings.Contains(stderr, "the "+strconv.Itoa(merger.MaxClusterSize)+" one person") {
		t.Errorf("stderr does not name the limit:\n%s", stderr)
	}
}
