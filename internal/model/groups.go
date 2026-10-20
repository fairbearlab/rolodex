package model

import (
	"sort"
	"strconv"
	"strings"
)

// SplitGroup splits an Extra key into its property group and name:
// "item1.X-ABLABEL" is ("item1", "X-ABLABEL"), "NICKNAME" is ("", "NICKNAME").
func SplitGroup(key string) (group, name string) {
	if i := strings.LastIndexByte(key, '.'); i >= 0 {
		return key[:i], key[i+1:]
	}
	return "", key
}

// groups returns every property group c uses, lower-cased.
func (c ParsedContact) groups() []string {
	set := make(map[string]bool)
	for _, e := range c.Emails {
		set[e.Group] = true
	}
	for _, p := range c.Phones {
		set[p.Group] = true
	}
	for _, a := range c.Addresses {
		set[a.Group] = true
	}
	set[c.URLGroup] = true
	for key := range c.Extra {
		g, _ := SplitGroup(key)
		set[g] = true
	}
	delete(set, "")
	lower := make(map[string]bool, len(set))
	for g := range set {
		lower[strings.ToLower(g)] = true
	}
	out := make([]string, 0, len(lower))
	for g := range lower {
		out = append(out, g)
	}
	sort.Strings(out)
	return out
}

// Regroup returns c with each property group already in used renamed to a
// free "itemN", and records c's groups in used (lower-cased: group names are
// case-insensitive). Merging two Apple cards unions their properties, and
// both number their groups from item1, so without this the second card's
// item1.X-ABLabel would be written as a second label on the first card's
// item1 value. c is not modified; its slices and Extra are copied when a
// group is renamed.
func Regroup(c ParsedContact, used map[string]bool) ParsedContact {
	groups := c.groups()
	var colliding []string
	for _, g := range groups {
		if used[g] {
			colliding = append(colliding, g)
		} else {
			used[g] = true
		}
	}
	if len(colliding) == 0 {
		return c
	}
	rename := make(map[string]string, len(colliding))
	n := 1
	for _, g := range colliding {
		for used["item"+strconv.Itoa(n)] {
			n++
		}
		fresh := "item" + strconv.Itoa(n)
		used[fresh] = true
		rename[g] = fresh
	}
	to := func(g string) string {
		if r, ok := rename[strings.ToLower(g)]; ok {
			return r
		}
		return g
	}

	c.Emails = append([]Email(nil), c.Emails...)
	for i := range c.Emails {
		c.Emails[i].Group = to(c.Emails[i].Group)
	}
	c.Phones = append([]Phone(nil), c.Phones...)
	for i := range c.Phones {
		c.Phones[i].Group = to(c.Phones[i].Group)
	}
	c.Addresses = append([]Address(nil), c.Addresses...)
	for i := range c.Addresses {
		c.Addresses[i].Group = to(c.Addresses[i].Group)
	}
	c.URLGroup = to(c.URLGroup)
	extra := make(map[string][]string, len(c.Extra))
	for key, vals := range c.Extra {
		if g, name := SplitGroup(key); g != "" {
			key = to(g) + "." + name
		}
		extra[key] = vals
	}
	c.Extra = extra
	return c
}
