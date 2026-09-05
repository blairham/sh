// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
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
			out, _ := runGrammar(t, c.src, func(d *syntax.Dialect) { d.ParamCaseChange = true }, nil)
			if strings.TrimSpace(out) != c.want {
				t.Errorf("said %q, want %q", strings.TrimSpace(out), c.want)
			}
		})
	}
}

// TestCaseConversionConsultsTheLocale — the policy docs/spec/semantics.md
// records: an explicit C or POSIX locale narrows case to ASCII, anything
// else, unset included, is Unicode-aware. LC_ALL outranks LC_CTYPE outranks
// LANG.
func TestCaseConversionConsultsTheLocale(t *testing.T) {
	for _, tc := range []struct {
		name string
		vars map[string]string
		want string
	}{
		{"explicit C is ASCII alone", map[string]string{"LC_ALL": "C"}, "CAFé"},
		{"POSIX is the same narrowing", map[string]string{"LANG": "POSIX"}, "CAFé"},
		{"a UTF-8 locale cases beyond it", map[string]string{"LC_ALL": "en_US.UTF-8"}, "CAFÉ"},
		{"unset is not C", nil, "CAFÉ"},
		{"LC_ALL outranks LANG", map[string]string{"LC_ALL": "en_US.UTF-8", "LANG": "C"}, "CAFÉ"},
		{"LC_CTYPE outranks LANG", map[string]string{"LC_CTYPE": "C", "LANG": "en_US.UTF-8"}, "CAFé"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := syntax.Core()
			d.ParamCaseChange = true
			f, err := syntax.Parse(`x=café; echo "${x^^}"`, d)
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			sem := permissive()
			vars := map[string]string{}
			for k, v := range tc.vars {
				vars[k] = v
			}
			r := &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Vars: vars, Env: []string{}}
			if _, rerr := r.Run(context.Background(), f); rerr != nil {
				t.Fatal(rerr)
			}
			if got := strings.TrimSpace(buf.String()); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
