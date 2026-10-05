package blocker

import (
	"fmt"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

// sharedPhone builds n contacts that share one phone and whose given names
// all start with the same letter, as a company switchboard or a placeholder
// like 000-000-0000 produces.
func sharedPhone(n int) []model.NormalizedContact {
	contacts := make([]model.NormalizedContact, n)
	for i := range contacts {
		contacts[i] = model.NormalizedContact{
			NormalizedPhones:     []string{"0000000000"},
			NormalizedGivenName:  fmt.Sprintf("person%d", i),
			NormalizedFamilyName: fmt.Sprintf("family%d", i),
		}
	}
	return contacts
}

// One shared phone across 4,000 contacts used to produce 7,998,000 candidate
// pairs (9.1s, 3.99 GB RSS; ~8,000 contacts was an OOM kill). An oversized
// identifier bucket is filtered like an oversized last-name bucket, and a
// sub-bucket that is still oversized is dropped and reported.
func TestBlockCapsSharedPhoneBucket(t *testing.T) {
	pairs, truncated := Block(sharedPhone(4000))
	if len(pairs) != 0 {
		t.Errorf("got %d candidate pairs from one shared phone, want 0", len(pairs))
	}
	if len(truncated) != 1 {
		t.Fatalf("truncated = %+v, want one phone bucket", truncated)
	}
	tr := truncated[0]
	if tr.Kind != "phone" || tr.Key != "0000000000" || tr.Size != 4000 || tr.Unpaired != 4000 {
		t.Errorf("truncated = %+v, want phone 0000000000, size 4000, all 4000 unpaired", tr)
	}
}

func TestBlockCapsSharedEmailBucket(t *testing.T) {
	contacts := sharedPhone(maxBlockSize + 1)
	for i := range contacts {
		contacts[i].NormalizedPhones = nil
		contacts[i].NormalizedEmails = []string{"info@example.com"}
	}
	pairs, truncated := Block(contacts)
	if len(pairs) != 0 {
		t.Errorf("got %d candidate pairs from one shared email, want 0", len(pairs))
	}
	if len(truncated) != 1 || truncated[0].Kind != "email" || truncated[0].Size != maxBlockSize+1 {
		t.Errorf("truncated = %+v, want one email bucket of %d", truncated, maxBlockSize+1)
	}
}

// The filter keeps the pairs a person could plausibly be: two contacts on
// the switchboard with the same first initial, or at the same organization.
func TestBlockOversizedPhoneBucketKeepsFilteredPairs(t *testing.T) {
	contacts := sharedPhone(maxBlockSize + 10)
	contacts[3].NormalizedGivenName = "zed"
	contacts[7].NormalizedGivenName = "zoe"
	contacts[11].Parsed.Org = "Acme"
	contacts[12].Parsed.Org = " acme "
	pairs, truncated := Block(contacts)
	want := map[[2]int]bool{{3, 7}: true, {11, 12}: true}
	if len(pairs) != len(want) {
		t.Errorf("pairs = %v, want exactly %v", pairs, want)
	}
	for _, p := range pairs {
		if !want[p] {
			t.Errorf("unexpected pair %v", p)
		}
	}
	// Everyone but the two "z"s and the two at Acme got no pair from the
	// bucket: they share an initial with too many others to filter on it.
	if len(truncated) != 1 || truncated[0].Unpaired != maxBlockSize+6 {
		t.Errorf("truncated = %+v, want one bucket with %d unpaired", truncated, maxBlockSize+6)
	}
}

// A bucket at the cap is paired in full and not reported.
func TestBlockBucketAtCapIsNotTruncated(t *testing.T) {
	pairs, truncated := Block(sharedPhone(maxBlockSize))
	if want := maxBlockSize * (maxBlockSize - 1) / 2; len(pairs) != want {
		t.Errorf("got %d pairs, want %d", len(pairs), want)
	}
	if len(truncated) != 0 {
		t.Errorf("truncated = %+v, want none", truncated)
	}
}
