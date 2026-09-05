// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"path"
	"strings"
)

// Patterns are globs over path *components*, which is the shape a policy
// wants and the shape neither path.Match nor a string prefix gives.
//
// A string prefix is what the -deny debug flag does, and it is wrong for a
// rule: `/etc` as a prefix refuses `/etcetera` because the name starts the
// same way. path.Match is right about components but has no way to say
// "and everything beneath", and `*` there does not cross a separator — which
// is correct and is exactly why something more is needed.
//
// So: each component is matched by path.Match, which brings `*`, `?` and
// character classes with the meanings everyone already knows, and `**` is
// added as a whole component meaning zero or more components. Zero matters:
// `/srv/**` has to match `/srv` itself, or every rule about a subtree would
// need a second rule about its root, and everyone would forget it.

// match reports whether an absolute, cleaned path is selected by a pattern.
func match(pattern, name string) bool {
	return matchParts(split(pattern), split(name))
}

// split breaks an absolute path into its components. The leading empty
// component from the root slash is dropped, so "/" is no components at all
// and "/srv/x" is two.
func split(p string) []string {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func matchParts(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			rest := pat[1:]
			if len(rest) == 0 {
				// Trailing `**` takes whatever is left, including nothing.
				return true
			}
			// Try every split point, shortest first. `<=` rather than `<` is
			// the zero-component case, which is what makes `/a/**/b` match
			// `/a/b` — and leaving it out is the bug that would make a rule
			// silently narrower than it reads.
			for i := 0; i <= len(name); i++ {
				if matchParts(rest, name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		ok, err := path.Match(pat[0], name[0])
		// A malformed pattern cannot get here — parse rejects one — and if it
		// somehow did, "matches nothing" is the fail-closed reading for an
		// allow and the fail-open one for a deny, which is why it is rejected
		// up front instead.
		if err != nil || !ok {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}

// validPattern reports whether every component of a pattern is one path.Match
// can read, so a typo is a parse error rather than a rule that never fires.
func validPattern(pattern string) error {
	for _, part := range split(pattern) {
		if part == "**" {
			continue
		}
		if _, err := path.Match(part, ""); err != nil {
			return err
		}
	}
	return nil
}
