package normalize

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/fairbearlab/rolodex/internal/model"
)

var (
	nonDigitRe    = regexp.MustCompile(`\D`)
	whitespaceRe  = regexp.MustCompile(`\s+`)
	titlePrefixes = []string{
		"dr.", "dr", "mr.", "mr", "mrs.", "mrs", "ms.", "ms",
		"prof.", "prof", "rev.", "rev", "sir", "dame",
	}
	nameSuffixes = []string{
		"jr.", "jr", "sr.", "sr", "ii", "iii", "iv", "v",
		"esq.", "esq", "md", "m.d.", "phd", "ph.d.",
		"dds", "d.d.s.",
	}
)

// Contact normalizes a ParsedContact for matching.
func Contact(c model.ParsedContact) model.NormalizedContact {
	return model.NormalizedContact{
		Parsed:               c,
		NormalizedFamilyName: Name(c.FamilyName),
		NormalizedGivenName:  Name(c.GivenName),
		NormalizedEmails:     normalizeEmails(c.Emails),
		NormalizedPhones:     normalizePhones(c.Phones),
	}
}

// Name applies Unicode NFKD normalization, case folding, whitespace collapse,
// and strips titles/suffixes for comparison purposes.
func Name(s string) string {
	if s == "" {
		return ""
	}

	// NFKD normalization decomposes characters
	s = norm.NFKD.String(s)

	// Remove combining marks (accents) after decomposition
	var b strings.Builder
	for _, r := range s {
		if !unicode.Is(unicode.Mn, r) { // Mn = nonspacing mark
			b.WriteRune(r)
		}
	}
	s = b.String()

	// Case fold
	s = strings.ToLower(s)

	// Collapse whitespace
	s = whitespaceRe.ReplaceAllString(strings.TrimSpace(s), " ")

	// Strip titles and suffixes
	words := strings.Fields(s)
	var filtered []string
	for _, w := range words {
		if isTitleOrSuffix(w) {
			continue
		}
		filtered = append(filtered, w)
	}
	if len(filtered) == 0 {
		return s // don't strip everything
	}
	return strings.Join(filtered, " ")
}

func isTitleOrSuffix(word string) bool {
	lower := strings.ToLower(strings.TrimRight(word, ".,"))
	for _, t := range titlePrefixes {
		if lower == t {
			return true
		}
	}
	for _, s := range nameSuffixes {
		if lower == s {
			return true
		}
	}
	return false
}

// Phone normalizes a phone number to digits only.
func Phone(number string) string {
	digits := nonDigitRe.ReplaceAllString(number, "")
	// Strip leading 1 for US numbers if 11 digits
	if len(digits) == 11 && digits[0] == '1' {
		digits = digits[1:]
	}
	return digits
}

// Email normalizes an email address.
func Email(addr string) string {
	return strings.ToLower(strings.TrimSpace(addr))
}

func normalizeEmails(emails []model.Email) []string {
	seen := make(map[string]bool)
	var result []string
	for _, e := range emails {
		norm := Email(e.Address)
		if norm != "" && !seen[norm] {
			seen[norm] = true
			result = append(result, norm)
		}
	}
	return result
}

func normalizePhones(phones []model.Phone) []string {
	seen := make(map[string]bool)
	var result []string
	for _, p := range phones {
		norm := Phone(p.Number)
		if norm != "" && !seen[norm] {
			seen[norm] = true
			result = append(result, norm)
		}
	}
	return result
}
