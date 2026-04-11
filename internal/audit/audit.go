package audit

import "github.com/fairbearlab/rolodex/internal/model"

// AuditOptions controls which contacts are flagged.
type AuditOptions struct {
	IncludeNamesOnly bool // also flag contacts with only a name (no email, phone, or org)
}

// AuditResult holds the output of a contact quality audit.
type AuditResult struct {
	Total            int
	UnreachableCount int
	Unreachable      []UnreachableContact
}

// UnreachableContact describes a contact that cannot be reached.
type UnreachableContact struct {
	Name       string `json:"name"`
	HasOrg     bool   `json:"has_org"`
	HasAddress bool   `json:"has_address"`
	HasTitle   bool   `json:"has_title"`
	Index      int    `json:"index"`
}

// Audit checks each contact for reachability (has email or phone).
func Audit(contacts []model.ParsedContact, opts AuditOptions) AuditResult {
	result := AuditResult{
		Total: len(contacts),
	}

	for i, c := range contacts {
		hasEmail := len(c.Emails) > 0
		hasPhone := len(c.Phones) > 0

		if hasEmail || hasPhone {
			continue // reachable
		}

		// No email and no phone — unreachable
		hasOrg := c.Org != ""
		hasTitle := c.Title != ""
		hasAddress := len(c.Addresses) > 0

		// Contacts with only a name (no org, title, or address) are
		// low-signal noise. Skip them unless --include-names-only is set.
		if !opts.IncludeNamesOnly && !hasOrg && !hasTitle && !hasAddress {
			continue
		}

		name := contactName(c)

		result.Unreachable = append(result.Unreachable, UnreachableContact{
			Name:       name,
			HasOrg:     hasOrg,
			HasAddress: hasAddress,
			HasTitle:   hasTitle,
			Index:      i,
		})
	}

	result.UnreachableCount = len(result.Unreachable)
	return result
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
