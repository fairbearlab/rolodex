package scorer

import (
	"fmt"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/normalize"
)

// benchContacts builds n synthetic normalized contacts with birthdays, middle
// names and shared identifiers in the shapes the real scorer sees, so the
// benchmark exercises sharedBirthday/birthdayConflict/birthdayUnknown and the
// sameName given/middle split on every pair, not just the cheap early-exit
// paths. The picks below are deterministic (index arithmetic against
// mutually-prime table sizes, not math/rand) so the benchmark's input is
// reproducible without a PRNG.
func benchContacts(n int) []model.NormalizedContact {
	givens := []string{"John", "Jon", "Bob", "Robert", "Chris", "Christopher", "Maria", "Jimmy"}
	families := []string{"Smith", "Doe", "Rodriguez", "Schuler", "Fielding", "Nguyen", "Petry"}
	middles := []string{"", "A.", "Andrew", "J", "."}
	bdays := []string{"1989-06-29", "--06-29", "1990-01-04", "circa 1950", "", "1970-01-01", "1962-01-04"}

	contacts := make([]model.NormalizedContact, n)
	for i := 0; i < n; i++ {
		p := model.ParsedContact{
			GivenName:  givens[i%len(givens)],
			MiddleName: middles[(i/len(givens))%len(middles)],
			FamilyName: families[i%len(families)],
			Birthday:   bdays[i%len(bdays)],
			Emails:     []model.Email{{Address: fmt.Sprintf("user%d@example.com", i/3)}},
			Phones:     []model.Phone{{Number: fmt.Sprintf("555%07d", i/4)}},
			Org:        families[(i+1)%len(families)] + " Inc",
		}
		contacts[i] = normalize.Contact(p)
	}
	return contacts
}

// benchPairs mimics the blocking fan-out described in TODOS.md: each contact
// lands in a shared-identifier bucket with several others, so it is scored
// against multiple partners and the per-contact work (birthday parse, name
// split) would otherwise be repeated once per pair it appears in.
func benchPairs(n int) [][2]int {
	var pairs [][2]int
	bucket := 6
	for start := 0; start < n; start += bucket {
		end := start + bucket
		if end > n {
			end = n
		}
		for i := start; i < end; i++ {
			for j := i + 1; j < end; j++ {
				pairs = append(pairs, [2]int{i, j})
			}
		}
	}
	return pairs
}

func BenchmarkScore(b *testing.B) {
	contacts := benchContacts(2000)
	pairs := benchPairs(2000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Score(contacts, pairs)
	}
}
