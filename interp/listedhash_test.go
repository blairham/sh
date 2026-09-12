// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Which `#` a listed value has to be quoted for (#1271), and the bases at the
// two ends of an output alphabet (#1308).

func listingSem(hash Answer) Semantics {
	s := testSemantics()
	s.DeclareValueQuoting = ListingQuoteWhenNeededEscaped
	s.ListedHashIsBareAfterANonName = hash
	return s
}

// The axis is asked only where the two answers differ: a `#` with a name in
// front of it, and a value with something else in it needing quotes, are
// quoted under both.
func TestAListedHashIsBareWhereNoNamePrecedesIt(t *testing.T) {
	for _, tc := range []struct{ value, bare, quoted string }{
		{"16#ff", "16#ff", "'16#ff'"},
		{"99#zz", "99#zz", "'99#zz'"},
		{"16#gg", "16#gg", "'16#gg'"},
		{"16#", "16#", "'16#'"},
		{"1a#b", "1a#b", "'1a#b'"},
		{"a.b#c", "a.b#c", "'a.b#c'"},
		{"1#b#c", "1#b#c", "'1#b#c'"},
		// A name in front of the first `#`, which is quoted either way — and
		// so is the empty prefix, where a comment would begin.
		{"a#b", "'a#b'", "'a#b'"},
		{"ab#", "'ab#'", "'ab#'"},
		{"_1#a", "'_1#a'", "'_1#a'"},
		{"#lead", "'#lead'", "'#lead'"},
		{"#", "'#'", "'#'"},
		// The first `#` decides for the whole value, so a name in front of
		// it quotes the value however the later ones read.
		{"a#b#c", "'a#b#c'", "'a#b#c'"},
		// Something else needing quotes is not this question.
		{"16#ff x", "'16#ff x'", "'16#ff x'"},
		{"16#ff*", "'16#ff*'", "'16#ff*'"},
		// And a value with no `#` at all asks nothing.
		{"plain", "plain", "plain"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			src := "v=" + shellSingleQuoted(tc.value) + "; typeset -p v"
			out, _ := run(t, src, withSem(listingSem(Yes)))
			if got := strings.TrimSpace(out); got != "declare -- v="+tc.bare {
				t.Errorf("bare: got %q, want %q", got, "declare -- v="+tc.bare)
			}
			out, _ = run(t, src, withSem(listingSem(No)))
			if got := strings.TrimSpace(out); got != "declare -- v="+tc.quoted {
				t.Errorf("quoted: got %q, want %q", got, "declare -- v="+tc.quoted)
			}
		})
	}
}

// shellSingleQuoted wraps text so a snippet can carry it whatever is in it.
func shellSingleQuoted(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}
