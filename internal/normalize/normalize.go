package normalize

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/fairbearlab/rolodex/internal/model"
)

var (
	nonDigitRe   = regexp.MustCompile(`\D`)
	whitespaceRe = regexp.MustCompile(`\s+`)
	// The optional tail is an ISO time, not "anything at all": the old
	// `(?:[T ].*)?$` matched "1989-10-22 or 23" and handed back a date the
	// caller then trusted as confirming evidence.
	bdayFullRe    = regexp.MustCompile(`^(\d{4})-?(\d{2})-?(\d{2})(?:[T ]\d{2}:\d{2}(?::\d{2})?(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?)?$`)
	bdayNoYearRe  = regexp.MustCompile(`^--(\d{2})-?(\d{2})$`)
	bdayYMDSepRe  = regexp.MustCompile(`^(\d{4})[/.](\d{1,2})[/.](\d{1,2})$`)
	bdaySlashRe   = regexp.MustCompile(`^(\d{1,2})/(\d{1,2})(?:/(\d{4}))?$`)
	bdayDottedRe  = regexp.MustCompile(`^(\d{1,2})\.(\d{1,2})\.(\d{4})$`)
	bdayMonthDMRe = regexp.MustCompile(`^(\d{1,2})(?:st|nd|rd|th)?\.? +([A-Za-z]+)\.?(?:,? +(\d{4}))?$`)
	bdayMonthMDRe = regexp.MustCompile(`^([A-Za-z]+)\.? +(\d{1,2})(?:st|nd|rd|th)?(?:,? +(\d{4}))?$`)
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
		NormalizedMiddleName: Name(c.MiddleName),
		StrictFamilyName:     NameStrict(c.FamilyName),
		StrictGivenName:      NameStrict(c.GivenName),
		StrictMiddleName:     NameStrict(c.MiddleName),
		NormalizedSuffix:     GenerationalSuffix(c),
		NormalizedEmails:     normalizeEmails(c.Emails),
		NormalizedPhones:     normalizePhones(c.Phones),
	}
}

var generationalSuffixes = map[string]string{
	"jr": "jr", "junior": "jr", "sr": "sr", "senior": "sr",
	"ii": "ii", "iii": "iii", "iv": "iv", "v": "v",
}

// GenerationalSuffix returns the contact's generational suffix (jr, sr, ii,
// iii, iv, v) in canonical lowercase form, or "". It is taken from the N
// suffix component, or from a trailing token of the family/given name when
// an export folded it in there ("Smith Jr."). Credentials such as MD or PhD
// are not generational and are ignored: they never distinguish two people.
func GenerationalSuffix(c model.ParsedContact) string {
	// The dedicated N suffix component is a suffix by definition, so any
	// token in it counts ("Jr.", "MD, Jr.").
	for _, w := range strings.Fields(strings.ToLower(c.Suffix)) {
		if g, ok := generationalSuffixes[strings.Trim(w, ".,")]; ok {
			return g
		}
	}
	// In the name fields only a TRAILING token counts, and only when a name
	// precedes it. A field that is nothing but the token is the name itself:
	// a contact whose given name is the initial "V" is not a fifth-generation
	// namesake, and treating it as one silently blocks every match against
	// the same person recorded with a full given name. A trailing single
	// letter is not a suffix either: Google folds the middle initial into
	// the given name ("John V") where iCloud keeps it in the middle slot,
	// and that is the common cross-source shape of one person, not the rare
	// fifth of a line. (A real "V" belongs in the N suffix component, where
	// it is still honoured.)
	for _, field := range []string{c.FamilyName, c.GivenName} {
		words := strings.Fields(strings.ToLower(field))
		if len(words) < 2 {
			continue
		}
		last := strings.Trim(words[len(words)-1], ".,")
		if utf8.RuneCountInString(last) < 2 {
			continue
		}
		if g, ok := generationalSuffixes[last]; ok {
			return g
		}
	}
	return ""
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

// NameStrict normalizes like Name but KEEPS diacritics. Name applies NFKD and
// drops every combining mark, so "Nguyên" and "Nguyễn" — different Vietnamese
// names — both fold to "nguyen". That folding is right for blocking and for
// similarity scoring, but on its own it let the exact-name rule auto-merge two
// people who share nothing but a phone. Callers that need identity, not
// similarity, compare this form as well.
func NameStrict(s string) string {
	if s == "" {
		return ""
	}
	// NFKC, not NFC: compatibility folding collapses halfwidth kana and
	// fullwidth Latin, which are a routine iCloud-vs-Google divergence for
	// Japanese contacts, while leaving the combining marks that distinguish
	// "Nguyên" from "Nguyễn" — the only thing this form exists to see.
	s = norm.NFKC.String(s)
	s = strings.ToLower(s)
	s = whitespaceRe.ReplaceAllString(strings.TrimSpace(s), " ")
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
	// A single letter is an initial, not a suffix. Google folds the middle
	// initial into the given name ("John V") where iCloud keeps it in the
	// middle slot. Stripping the "v" left an empty middle name, and an empty
	// middle name compares compatible with every other initial, so
	// "John V Doe" and "John W Doe" on one shared identifier merged unseen.
	// GenerationalSuffix already declines to read a trailing single letter as
	// generational; a real "V" belongs in the N suffix component, where it is
	// still honoured.
	if utf8.RuneCountInString(lower) < 2 {
		return false
	}
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

// Org cleans a vCard ORG value. ORG is a positional structured field
// (organization;unit;sub-unit...) and iCloud emits an empty trailing unit —
// "Acme;" — where Google emits "Acme". Trailing empty components are
// dropped so the two compare equal. Leading and interior empties are kept
// because they carry position: Apple writes ";Engineering" for a contact
// with a department but no company, and collapsing it would promote the
// department into the company slot on round-trip.
//
// A literal semicolon inside a component is escaped as "\;" on the wire
// ("ORG:Acme\; Inc."), and go-vcard hands that through undecoded. Only an
// unescaped ";" separates components, so the escape survives intact and is
// emitted as "\;" again by the writer.
func Org(s string) string {
	parts := splitUnescaped(s, ';')
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, ";")
}

// splitUnescaped splits s on sep, ignoring occurrences preceded by a
// backslash. The backslashes are kept: the value stays in wire form.
func splitUnescaped(s string, sep rune) []string {
	var parts []string
	var b strings.Builder
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\':
			b.WriteRune(r)
			escaped = true
		case r == sep:
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	return append(parts, b.String())
}

// DisplayComponents splits a structured wire-form value on unescaped
// separators and unescapes each component. The parser uses it to decode N and
// ADR, and the review TUI to read ORG, which the model keeps in wire form: an
// escaped "\;" is part of its component, so "Acme\; Inc." is ONE organization
// that should read "Acme; Inc." — splitting it naively showed the reviewer
// "Acme\, Inc." while they decided whether to merge two records.
func DisplayComponents(s string, sep rune) []string {
	parts := splitUnescaped(s, sep)
	for i, p := range parts {
		parts[i] = Unescape(p)
	}
	return parts
}

// Escape encodes a decoded value for the wire: backslash, semicolon, comma
// and newline become "\\", "\;", "\," and "\n" (RFC 6350 §3.4). It is the
// inverse of Unescape and is applied per component, so the caller adds a
// structured value's own separators after escaping.
func Escape(s string) string {
	return escaper.Replace(s)
}

var escaper = strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`)

// Unescape resolves vCard backslash escapes in a single component value.
func Unescape(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			// "\n" is the vCard encoding of a newline; everything else
			// stands for the character itself.
			if r == 'n' || r == 'N' {
				b.WriteRune('\n')
			} else {
				b.WriteRune(r)
			}
			escaped = false
		case r == '\\':
			escaped = true
		default:
			b.WriteRune(r)
		}
	}
	if escaped {
		b.WriteRune('\\')
	}
	return b.String()
}

// canonicalBirthdayRe matches the forms Birthday produces (YYYY-MM-DD or
// --MM-DD). Anything else that survived normalization is free text.
var canonicalBirthdayRe = regexp.MustCompile(`^(\d{4}|-)-(\d{2})-(\d{2})$`)

// ParseCanonicalBirthday splits a canonical birthday into its year ("" when
// the year is unknown) and month-day. ok is false for anything that is not a
// real date, including a placeholder that merely looks canonical
// ("0000-00-00", "1989-02-31") — Birthday passes those through untouched.
//
// This lives beside canonicalBirthday deliberately. The scorer used to repeat
// the check with its own month/day bounds, and when canonicalBirthday learned
// about real calendar dates the copy did not: February 31 was rejected by the
// normalizer and then accepted by the scorer as evidence to merge on. One
// definition, one answer.
func ParseCanonicalBirthday(s string) (year, monthDay string, ok bool) {
	m := canonicalBirthdayRe.FindStringSubmatch(s)
	if m == nil {
		return "", "", false
	}
	month, day := atoi(m[2]), atoi(m[3])
	y := 2000 // a leap year, so --02-29 stays legal when no year is given
	if m[1] != "-" {
		y = atoi(m[1])
		if y < 1 {
			return "", "", false
		}
	}
	if !validDate(y, month, day) {
		return "", "", false
	}
	if m[1] != "-" {
		year = m[1]
	}
	return year, m[2] + "-" + m[3], true
}

// BirthdaysAgree reports whether two canonical birthdays are the same date
// as far as both can tell: equal month and day, and equal year unless one
// side has none — iCloud may omit the year that Google keeps. Anything that
// is not a real canonical date agrees with nothing. The scorer, the merger
// and the report all decide "same birthday" here, so a pair cannot be
// merged on a shared birthday and then reported as a birthday conflict.
func BirthdaysAgree(a, b string) bool {
	yearA, mdA, okA := ParseCanonicalBirthday(a)
	yearB, mdB, okB := ParseCanonicalBirthday(b)
	if !okA || !okB || mdA != mdB {
		return false
	}
	return yearA == "" || yearB == "" || yearA == yearB
}

// PlausibleBirthday reports whether a canonical birthday could be evidence
// that two contacts are one person. A January 1st — with any year or none —
// is the date an export carries when nobody entered one: 1970-01-01 (the
// Unix epoch), 1900-01-01 and 2000-01-01 (form and spreadsheet defaults),
// and Apple's 1604-01-01, which arrives here as "--01-01". People are born
// on January 1st, so a same-named pair that really shares one goes to
// review instead of auto-merging; that costs a review card, never a person.
// The same "only a well-formed value is evidence" rule as PlausiblePhone
// and PlausibleEmail.
func PlausibleBirthday(s string) bool {
	_, md, ok := ParseCanonicalBirthday(s)
	return ok && md != "01-01"
}

// PreferBirthday chooses the birthday for a merged contact. The priority
// source's value wins, except that an empty value is filled from the other
// side and a no-year date is completed by a full date it agrees with:
// iCloud's "--10-22" and Google's "1989-10-22" are one birthday, and keeping
// the yearless form lost the year with no warning. A disagreement is left
// to the priority source and shows up in the report as a conflict.
func PreferBirthday(priority, other string) string {
	if priority == "" {
		return other
	}
	yearP, _, okP := ParseCanonicalBirthday(priority)
	yearO, _, okO := ParseCanonicalBirthday(other)
	if okP && okO && yearP == "" && yearO != "" && BirthdaysAgree(priority, other) {
		return other
	}
	return priority
}

// minPhoneDigits is the shortest real subscriber number.
const minPhoneDigits = 7

// PlausiblePhone reports whether a normalized phone could be a real number.
// A shared identifier is what promotes an identical name to auto_merge, so a
// placeholder two contacts happen to share must not count: "0", a truncated
// field, or "000-000-0000" are not numbers anyone can be reached on.
func PlausiblePhone(p string) bool {
	if len(p) < minPhoneDigits {
		return false
	}
	for i := 0; i < len(p); i++ {
		if p[i] != p[0] {
			return true
		}
	}
	return false
}

// PlausibleEmail reports whether a normalized email could be a real address.
// "unknown" and "user@localhost" are not addresses two people can share.
func PlausibleEmail(e string) bool {
	at := strings.IndexByte(e, '@')
	if at <= 0 || at == len(e)-1 {
		return false
	}
	domain := e[at+1:]
	dot := strings.IndexByte(domain, '.')
	return dot > 0 && dot < len(domain)-1
}

// applePlaceholderYear is the year Apple Contacts stores for a birthday
// entered without a year. iCloud exports it with X-APPLE-OMIT-YEAR=1604;
// contacts synced onward to Google keep the year but lose the parameter.
const applePlaceholderYear = "1604"

var monthNames = map[string]int{
	"jan": 1, "january": 1, "feb": 2, "february": 2, "mar": 3, "march": 3,
	"apr": 4, "april": 4, "may": 5, "jun": 6, "june": 6, "jul": 7, "july": 7,
	"aug": 8, "august": 8, "sep": 9, "sept": 9, "september": 9, "oct": 10,
	"october": 10, "nov": 11, "november": 11, "dec": 12, "december": 12,
}

// Birthday canonicalizes a BDAY value to YYYY-MM-DD, or --MM-DD when the
// year is unknown. Recognized inputs: "1989-10-22", "19891022" (Google),
// "--1022" / "--10-22" (no year), the Apple placeholder year 1604, any of
// these with a trailing time, and the hand-typed forms "1989/10/22",
// "10/22/1989" (month first, as typed in the US), "22.10.1989" (day first,
// as typed in Europe), "10/22", "October 22, 1989", "22 Oct 1989" and
// "October 22". A slash or dotted date whose first number cannot be the
// month (or day) it is read as is flipped, so "22/10/1989" still works.
// Anything else — including a bare year, a two-digit year, and a month or
// day out of range — is returned trimmed but otherwise untouched, and the
// scorer treats it as unreadable rather than as a date.
func Birthday(s string) string {
	s = strings.TrimSpace(s)
	if m := bdayFullRe.FindStringSubmatch(s); m != nil {
		return canonicalBirthday(m[1], m[2], m[3], s)
	}
	if m := bdayNoYearRe.FindStringSubmatch(s); m != nil {
		return canonicalBirthday("", m[1], m[2], s)
	}
	if m := bdayYMDSepRe.FindStringSubmatch(s); m != nil {
		return canonicalBirthday(m[1], m[2], m[3], s)
	}
	if m := bdaySlashRe.FindStringSubmatch(s); m != nil {
		month, day := m[1], m[2]
		if atoi(month) > 12 && atoi(day) <= 12 {
			month, day = day, month
		}
		return canonicalBirthday(m[3], month, day, s)
	}
	if m := bdayDottedRe.FindStringSubmatch(s); m != nil {
		day, month := m[1], m[2]
		if atoi(month) > 12 && atoi(day) <= 12 {
			month, day = day, month
		}
		return canonicalBirthday(m[3], month, day, s)
	}
	if m := bdayMonthDMRe.FindStringSubmatch(s); m != nil {
		if mon, ok := monthNames[strings.ToLower(m[2])]; ok {
			return canonicalBirthday(m[3], strconv.Itoa(mon), m[1], s)
		}
		return s
	}
	if m := bdayMonthMDRe.FindStringSubmatch(s); m != nil {
		if mon, ok := monthNames[strings.ToLower(m[1])]; ok {
			return canonicalBirthday(m[3], strconv.Itoa(mon), m[2], s)
		}
		return s
	}
	return s
}

// canonicalBirthday assembles YYYY-MM-DD (or --MM-DD when year is empty),
// mapping the Apple placeholder year to "no year". It returns raw for anything
// that is not a real calendar date: "0000-00-00", "1989-13-45" and "1989-02-31"
// are placeholders or typos, not dates, and must not compare equal to anything.
// A birthday is confirming evidence for the exact-name rule, so a placeholder
// that two contacts happen to share would auto-merge them on the name alone.
func canonicalBirthday(year, month, day, raw string) string {
	mo, d := atoi(month), atoi(day)
	if mo < 1 || mo > 12 || d < 1 || d > 31 {
		return raw
	}
	md := fmt.Sprintf("%02d-%02d", mo, d)
	if year == "" || year == applePlaceholderYear {
		// No year: validate against a leap year so 02-29 stays legal.
		if !validDate(2000, mo, d) {
			return raw
		}
		return "--" + md
	}
	y := atoi(year)
	if y < 1 || !validDate(y, mo, d) {
		return raw
	}
	return year + "-" + md
}

// validDate reports whether y-m-d is a real Gregorian date. time.Date
// normalizes out-of-range components (February 31 becomes March 3), so
// comparing the round trip is what actually rejects them.
func validDate(y, m, d int) bool {
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	return t.Year() == y && int(t.Month()) == m && t.Day() == d
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return n
}

// BirthdayWithoutYear drops the year from a canonical YYYY-MM-DD value,
// for exports that mark the year as a placeholder (iCloud's
// X-APPLE-OMIT-YEAR=1604). Values that are not canonical are returned as-is.
func BirthdayWithoutYear(s string) string {
	if m := bdayFullRe.FindStringSubmatch(s); m != nil {
		return "--" + m[2] + "-" + m[3]
	}
	return s
}
