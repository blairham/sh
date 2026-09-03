// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// The case-change operators carry a *pattern* saying which characters to
// convert, and it is matched one character at a time.
//
// Found by the wild sweep. Three faults in one operator, and the worst was
// silent: the doubled forms parsed, discarded the pattern, and converted the
// whole string with status 0 — so a script asking for a subset got the lot.
func TestCaseChangeAppliesItsPattern(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// The silent one: only the characters the pattern matches.
		{"upper, a pattern", `x=abc; echo "${x^^[ab]}"`, "ABc"},
		{"lower, a pattern", `x=ABC; echo "${x,,[AB]}"`, "abC"},
		{"a pattern matching nothing", `x=abc; echo "${x^^[xyz]}"`, "abc"},
		{"a range", `x=a1b2; echo "${x^^[a-z]}"`, "A1B2"},
		{"a negated class", `x=abc; echo "${x^^[!a]}"`, "aBC"},
		// No pattern is every character, which is what `?` would say.
		{"upper, no pattern", `x=abc; echo "${x^^}"`, "ABC"},
		{"lower, no pattern", `x=ABC; echo "${x,,}"`, "abc"},
		{"an explicit ?", `x=abc; echo "${x^^?}"`, "ABC"},

		// The single forms: the first character, and only if it matches.
		{"upper first", `x=abc; echo "${x^}"`, "Abc"},
		{"lower first", `x=ABC; echo "${x,}"`, "aBC"},
		{"first, matching", `x=abc; echo "${x^a}"`, "Abc"},
		{"first, not matching", `x=abc; echo "${x^b}"`, "abc"},
		{"first, a class", `x=abc; echo "${x^[ab]}"`, "Abc"},
		// Only the first, even where a later character matches as well.
		{"first is not every", `x=hello; echo "${x^l}"`, "hello"},
		{"doubled is every", `x=hello; echo "${x^^l}"`, "heLLo"},

		// Toggle, both forms.
		{"toggle first", `x=abc; echo "${x~}"`, "Abc"},
		{"toggle all", `x=aBc; echo "${x~~}"`, "AbC"},
		{"toggle back", `x=ABC; echo "${x~~}"`, "abc"},

		// The pattern is expanded, so it may come from a variable.
		{"a pattern from a variable", `x=abc; p=b; echo "${x^^$p}"`, "aBc"},

		// Nothing to convert stays nothing rather than becoming empty.
		{"an empty value", `x=; echo "[${x^}]"`, "[]"},
		{"an unset name", `echo "[${undef^^}]"`, "[]"},

		// And it reaches an array's elements, each on its own.
		{"every element", `a=(ab cd); echo "${a[@]^^}"`, "AB CD"},
		{"one element", `a=(ab cd); echo "${a[0]^}"`, "Ab"},
		{"elements with a pattern", `a=(ab cd); echo "${a[@]^^[a]}"`, "Ab cd"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runBash(t, c.src)
			if strings.TrimSpace(out) != c.want {
				t.Errorf("said %q, want %q", strings.TrimSpace(out), c.want)
			}
		})
	}
}
