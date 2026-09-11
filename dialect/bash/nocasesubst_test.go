// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `nocasematch` folds the **substitution** operators of parameter expansion
// and leaves the trims beside them exact.
//
// The seam is *inside* `${ }`, which is why it went unnoticed: the comment on
// the option this dialect sets cited `${x#a}` as its measurement and concluded
// that parameter expansion was exempt. `${x#a}` cannot tell a trim from a
// substitution, and the two do not agree.
//
// Measured 2026-09-11 on bash 5.3.15 with `v=ABC`, after `shopt -s
// nocasematch`. Written as one table so the two halves cannot drift apart:
// a fix that folded everything under `${ }` passes the first five rows and
// fails the last three, and the old behavior does the reverse (#1969).
func TestNocasematchFoldsTheSubstitutionOperatorsAndNotTheTrims(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a global substitution", `shopt -s nocasematch; v=ABC; echo ${v//b/X}`, "AXC"},
		{"a single substitution", `shopt -s nocasematch; v=ABC; echo ${v/b/X}`, "AXC"},
		{"anchored at the front", `shopt -s nocasematch; v=ABC; echo ${v/#a/Y}`, "YBC"},
		{"anchored at the end", `shopt -s nocasematch; v=ABC; echo ${v/%c/Z}`, "ABZ"},
		{"with no replacement at all", `shopt -s nocasematch; v=ABC; echo ${v//b}`, "AC"},

		{"not a prefix trim", `shopt -s nocasematch; v=ABC; echo ${v#a}`, "ABC"},
		{"not a suffix trim", `shopt -s nocasematch; v=ABC; echo ${v%c}`, "ABC"},
		{"not the case-change operator", `shopt -s nocasematch; v=ABC; echo ${v^^b}`, "ABC"},

		// The option is what does it, so the same lines with it off are the
		// subject unchanged. A substitution that had simply started folding
		// unconditionally passes every row above and fails here.
		{"and the option is what decides", `v=ABC; echo ${v//b/X}`, "ABC"},
		{"turned back off again", `shopt -s nocasematch; shopt -u nocasematch; v=ABC; echo ${v//b/X}`, "ABC"},

		// `nocaseglob` is the other option and reaches none of it — measured,
		// so that the fold is read off the matching option and not whichever
		// one happened to be on.
		{"and nocaseglob is not that option", `shopt -s nocaseglob; v=ABC; echo ${v//b/X}`, "ABC"},

		// Symmetric, and the whole matcher rather than a prefix test: a
		// bracket and a `?` fold alongside the plain letter, which is what
		// says the fold is in the comparison rather than in the pattern's
		// text.
		{"an upper-case pattern against a lower-case subject", `shopt -s nocasematch; v=abc; echo ${v//B/X}`, "aXc"},
		{"a bracket expression folds too", `shopt -s nocasematch; v=abc; echo ${v//[B]/X}`, "aXc"},
		{"a wildcard beside the folded letter", `shopt -s nocasematch; v=abc; echo ${v//b?/X}`, "aX"},

		// The fold decides what the pattern matches and not what the
		// replacement stands for: `&` is still the text the subject held,
		// case and all.
		{"an ampersand is the subject's own text", `shopt -s nocasematch; v=ABC; echo ${v//b/<&>}`, "A<B>C"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), tc.src)
			if got := strings.TrimRight(out, "\n"); got != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q", tc.src, got, st, tc.want)
			}
		})
	}
}
