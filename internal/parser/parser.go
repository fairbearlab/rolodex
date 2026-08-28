package parser

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	vcard "github.com/emersion/go-vcard"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/normalize"
)

// ParseFile reads a .vcf file and returns parsed contacts.
func ParseFile(path string, source model.Source) ([]model.ParsedContact, []model.Warning, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return Parse(bytes.NewReader(data), source)
}

// Parse reads vCard data from a reader and returns parsed contacts.
func Parse(r io.Reader, source model.Source) ([]model.ParsedContact, []model.Warning, error) {
	dec := vcard.NewDecoder(r)
	var contacts []model.ParsedContact
	var warnings []model.Warning
	idx := 0

	for {
		card, err := dec.Decode()
		if err == io.EOF {
			break
		}
		if err != nil {
			warnings = append(warnings, model.Warning{
				Source:  source,
				Index:   idx,
				Message: fmt.Sprintf("malformed vCard entry: %v", err),
			})
			idx++
			continue
		}

		c := cardToContact(card, source)
		contacts = append(contacts, c)
		idx++
	}

	return contacts, warnings, nil
}

func cardToContact(card vcard.Card, source model.Source) model.ParsedContact {
	c := model.ParsedContact{
		Source: source,
		Extra:  make(map[string][]string),
	}

	// Provenance written by an earlier rolodex run (review.vcf / merged.vcf).
	// On read-back paths the caller only has a generic label ("review",
	// "merged", unknown) and a single known source restores which card is
	// which. On the ingest path the --icloud/--google flag is authoritative,
	// so a stray X-ROLODEX-SOURCE in a re-exported file must not relabel the
	// contact (that would also hide it from conflict reporting). The field is
	// kept in Extra either way: merged provenance like "merged(icloud+google)"
	// is read from there by resolve.
	if source != model.SourceICloud && source != model.SourceGoogle {
		if src := provenanceSource(card); src != "" {
			c.Source = src
		}
	}

	// Structured name (N field)
	if names := card[vcard.FieldName]; len(names) > 0 {
		// The N field has components: family;given;middle;prefix;suffix
		parts := splitN(names[0])
		c.FamilyName = parts[0]
		c.GivenName = parts[1]
		c.MiddleName = parts[2]
		c.Prefix = parts[3]
		c.Suffix = parts[4]
	}

	// Formatted name
	if fn := card.PreferredValue(vcard.FieldFormattedName); fn != "" {
		c.FormattedName = fn
	}

	// Emails
	for _, field := range card[vcard.FieldEmail] {
		if field.Value == "" {
			continue
		}
		emailType := fieldType(field)
		c.Emails = append(c.Emails, model.Email{
			Address: field.Value,
			Type:    emailType,
		})
	}

	// Phones
	for _, field := range card[vcard.FieldTelephone] {
		if field.Value == "" {
			continue
		}
		phoneType := fieldType(field)
		c.Phones = append(c.Phones, model.Phone{
			Number: field.Value,
			Type:   phoneType,
		})
	}

	// Org (structured; iCloud leaves an empty trailing unit, e.g. "Acme;")
	if org := normalize.Org(card.PreferredValue(vcard.FieldOrganization)); org != "" {
		c.Org = org
	}

	// Title
	if title := card.PreferredValue(vcard.FieldTitle); title != "" {
		c.Title = title
	}

	// Birthday, canonicalized to YYYY-MM-DD / --MM-DD so iCloud ("1989-10-22")
	// and Google ("19891022") agree. iCloud marks a no-year birthday with a
	// placeholder year and X-APPLE-OMIT-YEAR=<that year>.
	if f := card.Preferred(vcard.FieldBirthday); f != nil && f.Value != "" {
		bday := normalize.Birthday(f.Value)
		if omit := f.Params.Get("X-APPLE-OMIT-YEAR"); omit != "" && strings.HasPrefix(bday, omit+"-") {
			bday = normalize.BirthdayWithoutYear(bday)
		}
		c.Birthday = bday
	}

	// Addresses
	for _, field := range card[vcard.FieldAddress] {
		addr := parseAddress(field)
		if addr != (model.Address{}) {
			c.Addresses = append(c.Addresses, addr)
		}
	}

	// Note
	if note := card.PreferredValue(vcard.FieldNote); note != "" {
		c.Note = note
	}

	// URL
	if url := card.PreferredValue(vcard.FieldURL); url != "" {
		c.URL = url
	}

	// Photo
	if photos := card[vcard.FieldPhoto]; len(photos) > 0 {
		photo := photos[0]
		c.PhotoType = photo.Params.Get("TYPE")
		encoding := photo.Params.Get("ENCODING")
		if strings.EqualFold(encoding, "b") || strings.EqualFold(encoding, "BASE64") {
			decoded, err := base64.StdEncoding.DecodeString(photo.Value)
			if err == nil {
				c.Photo = decoded
			}
		} else if photo.Value != "" {
			c.Photo = []byte(photo.Value)
		}
	}

	// Collect extra fields we don't explicitly model
	modeled := map[string]bool{
		"VERSION": true, "N": true, "FN": true, "EMAIL": true, "TEL": true,
		"ORG": true, "TITLE": true, "BDAY": true, "ADR": true, "NOTE": true,
		"URL": true, "PHOTO": true, "BEGIN": true, "END": true, "UID": true,
		"PRODID": true, "REV": true,
	}
	for key, fields := range card {
		upperKey := strings.ToUpper(key)
		if modeled[upperKey] {
			continue
		}
		for _, f := range fields {
			c.Extra[key] = append(c.Extra[key], f.Value)
		}
	}

	return c
}

// provenanceSource returns the single source recorded in X-ROLODEX-SOURCE,
// or "" when the field is absent or names a merged/unknown source.
func provenanceSource(card vcard.Card) model.Source {
	src := model.Source(strings.TrimSpace(card.PreferredValue("X-ROLODEX-SOURCE")))
	switch src {
	case model.SourceICloud, model.SourceGoogle:
		return src
	default:
		return ""
	}
}

// splitN extracts the 5 components of the N field.
func splitN(field *vcard.Field) [5]string {
	var parts [5]string
	// The N field value is "family;given;middle;prefix;suffix"
	components := strings.Split(field.Value, ";")
	for i := 0; i < len(components) && i < 5; i++ {
		parts[i] = strings.TrimSpace(components[i])
	}
	return parts
}

func fieldType(field *vcard.Field) string {
	if types := field.Params["TYPE"]; len(types) > 0 {
		return strings.ToUpper(types[0])
	}
	return ""
}

func parseAddress(field *vcard.Field) model.Address {
	// ADR value is: POBox;Extended;Street;City;Region;PostCode;Country
	components := strings.Split(field.Value, ";")
	addr := model.Address{
		Type: fieldType(field),
	}
	if len(components) > 0 {
		addr.POBox = strings.TrimSpace(components[0])
	}
	if len(components) > 1 {
		addr.Extended = strings.TrimSpace(components[1])
	}
	if len(components) > 2 {
		addr.Street = strings.TrimSpace(components[2])
	}
	if len(components) > 3 {
		addr.City = strings.TrimSpace(components[3])
	}
	if len(components) > 4 {
		addr.Region = strings.TrimSpace(components[4])
	}
	if len(components) > 5 {
		addr.PostCode = strings.TrimSpace(components[5])
	}
	if len(components) > 6 {
		addr.Country = strings.TrimSpace(components[6])
	}

	// Return zero value if completely empty
	if addr.Street == "" && addr.City == "" && addr.Region == "" &&
		addr.PostCode == "" && addr.Country == "" && addr.POBox == "" && addr.Extended == "" {
		return model.Address{}
	}
	return addr
}
