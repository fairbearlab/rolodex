// Package prune splits a contact list into the contacts that can be reached
// and the ones that cannot. Nothing here deletes: the caller writes both sets.
package prune

import (
	"fmt"
	"strings"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/normalize"
)

// Channel is one way a contact can be reached.
type Channel string

const (
	ChannelEmail   Channel = "email"
	ChannelPhone   Channel = "phone"
	ChannelAddress Channel = "address"
	ChannelURL     Channel = "url"
)

// DefaultChannels is what counts as reachable unless the user says otherwise.
// URL is opt-in: on the author's export it is the residue of an old
// social-network sync (facebook.com, myspace.com), not a way to reach anyone.
var DefaultChannels = []Channel{ChannelEmail, ChannelPhone, ChannelAddress}

// Reachable reports whether any enabled channel is present on the contact.
// Email and phone go through the same plausibility gates the scorer uses, so
// a placeholder ("unknown", "000-000-0000") does not keep a contact.
func Reachable(c model.ParsedContact, by []Channel) bool {
	for _, ch := range by {
		switch ch {
		case ChannelEmail:
			if hasEmail(c) {
				return true
			}
		case ChannelPhone:
			if hasPhone(c) {
				return true
			}
		case ChannelAddress:
			if deliverableAddress(c) {
				return true
			}
		case ChannelURL:
			if hasURL(c) {
				return true
			}
		}
	}
	return false
}

func hasEmail(c model.ParsedContact) bool {
	for _, e := range c.Emails {
		if normalize.PlausibleEmail(normalize.Email(e.Address)) {
			return true
		}
	}
	return false
}

func hasPhone(c model.ParsedContact) bool {
	for _, p := range c.Phones {
		if normalize.PlausiblePhone(normalize.Phone(p.Number)) {
			return true
		}
	}
	return false
}

// deliverableAddress is true when an address is somewhere mail could go: a
// street, a PO box, an extended line (suite, floor) or a postcode. A
// country or a city on its own is what an export carries when a picker was
// set and nothing typed, so it is no more a way to reach someone than a
// blank ADR. This is the reachability rule; hasAddress is the report's.
func deliverableAddress(c model.ParsedContact) bool {
	for _, a := range c.Addresses {
		if anyNonBlank(a.Street, a.POBox, a.Extended, a.PostCode) {
			return true
		}
	}
	return false
}

// hasAddress is true when any address has a non-blank component. The
// report uses it: a card carrying "Paris, France" is not deliverable, but
// it is not "name only" either. Type is not a component: "ADR;TYPE=HOME:;;;;;;"
// is an empty address.
func hasAddress(c model.ParsedContact) bool {
	for _, a := range c.Addresses {
		if anyNonBlank(a.Street, a.City, a.Region, a.PostCode, a.Country, a.POBox, a.Extended) {
			return true
		}
	}
	return false
}

func anyNonBlank(parts ...string) bool {
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			return true
		}
	}
	return false
}

func hasURL(c model.ParsedContact) bool {
	return strings.TrimSpace(c.URL) != ""
}

// ParseChannels reads a comma-separated channel list such as "email,phone".
// Names are case-insensitive and duplicates collapse; an unknown name or an
// empty list is an error, because a typo here would silently move contacts
// into removed.vcf.
func ParseChannels(s string) ([]Channel, error) {
	var out []Channel
	seen := make(map[Channel]bool)
	for _, part := range strings.Split(s, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		if name == "" {
			continue
		}
		ch := Channel(name)
		switch ch {
		case ChannelEmail, ChannelPhone, ChannelAddress, ChannelURL:
		default:
			return nil, fmt.Errorf("unknown channel %q in --reachable-by (valid: email, phone, address, url)", part)
		}
		if !seen[ch] {
			seen[ch] = true
			out = append(out, ch)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--reachable-by is empty (valid: email, phone, address, url)")
	}
	return out, nil
}

// Options controls what counts as reachable.
type Options struct {
	ReachableBy []Channel
}

// Removed describes one unreachable contact, for the report. The Has* flags
// describe presence regardless of which channels are enabled: with address
// enabled, a contact that has one is kept and never appears here. An email
// or phone listed here is one that did not count — it failed the
// plausibility gate or its channel is off — and the report must say so:
// "name only" on a card that carries a phone number is the line a user
// deletes removed.vcf on.
type Removed struct {
	Name        string `json:"name"`
	Index       int    `json:"index"` // zero-based position in the parsed input (malformed entries do not occupy an index)
	HasEmail    bool   `json:"has_email"`
	HasPhone    bool   `json:"has_phone"`
	HasOrg      bool   `json:"has_org"`
	HasTitle    bool   `json:"has_title"`
	HasAddress  bool   `json:"has_address"`
	HasURL      bool   `json:"has_url"`
	HasBirthday bool   `json:"has_birthday"`
	HasNote     bool   `json:"has_note"`
	HasPhoto    bool   `json:"has_photo"`
}

// Result is the split. Kept and Removed keep input order; Detail has one
// entry per Removed contact, in the same order.
type Result struct {
	Total   int
	Kept    []model.ParsedContact
	Removed []model.ParsedContact
	Detail  []Removed
}

// Split partitions contacts into reachable and unreachable.
func Split(contacts []model.ParsedContact, opts Options) Result {
	r := Result{Total: len(contacts)}
	for i, c := range contacts {
		if Reachable(c, opts.ReachableBy) {
			r.Kept = append(r.Kept, c)
			continue
		}
		r.Removed = append(r.Removed, c)
		r.Detail = append(r.Detail, Removed{
			Name:        contactName(c),
			Index:       i,
			HasEmail:    len(c.Emails) > 0,
			HasPhone:    len(c.Phones) > 0,
			HasOrg:      c.Org != "",
			HasTitle:    c.Title != "",
			HasAddress:  hasAddress(c),
			HasURL:      hasURL(c),
			HasBirthday: c.Birthday != "",
			HasNote:     strings.TrimSpace(c.Note) != "",
			HasPhoto:    len(c.Photo) > 0 || c.PhotoURI != "",
		})
	}
	return r
}

func contactName(c model.ParsedContact) string {
	if c.FormattedName != "" {
		return c.FormattedName
	}
	name := c.GivenName
	if c.FamilyName != "" {
		if name != "" {
			name += " "
		}
		name += c.FamilyName
	}
	if name != "" {
		return name
	}
	if c.Org != "" {
		return c.Org
	}
	return "(unknown)"
}
