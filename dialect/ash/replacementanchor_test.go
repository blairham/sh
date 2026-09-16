// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/interp"

	"github.com/blairham/sh/dialect/ash"
)

// `${v/#pat/rep}` and `${v/%pat/rep}` have no anchors here (#3272).
//
// `syntax.Dialect.ParamSubstitution`'s comment calls the two halves one
// feature — "`${x/pat/rep}` and its anchored forms" — and in this shell they
// are not one feature: it has the replacement and has no anchors, so a `#` or
// a `%` written after the `/` is the pattern's own first character.
//
// Measured 2026-09-16 against BusyBox v1.37.0 in the digest-pinned Alpine
// image internal/oracle reaches, under `env -i PATH=/usr/bin:/bin LC_ALL=C`,
// `--init`, and stdin from /dev/null.
func TestTheReplacementHasNoAnchors(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The controls first, because they are what make every row
			// below "no anchor" rather than "no replacement". Both are
			// right here and in every column that has the construct.
			"the unanchored replacement works",
			`v=abcabc; printf '%s' "${v/b/X}"`,
			"aXcabc",
		},
		{
			"and the global one",
			`v=abcabc; printf '%s' "${v//b/X}"`,
			"aXcaXc",
		},
		{
			// The rows the issue was filed on. Six columns answer `Xbcabc`
			// and `abcabX`; here the patterns are `#a` and `%c`, which
			// `abcabc` does not contain.
			"a start anchor is the first character of the pattern",
			`v=abcabc; printf '%s' "${v/#a/X}"`,
			"abcabc",
		},
		{
			"and so is an end anchor",
			`v=abcabc; printf '%s' "${v/%c/X}"`,
			"abcabc",
		},
		{
			// **The discriminating probe.** `abcabc` answers `abcabc`
			// under both readings — "the anchor is honored and `a` does
			// not start the value" and "the pattern is `#a` and is not
			// there" — so a value holding the anchor character is what
			// tells them apart. Here the `#a` is found *inside* the value
			// and replaced, which no other reading produces. The six
			// columns leave this value alone.
			"and the pattern is found where it really stands",
			`w='x#ay%bz'; printf '%s' "${w/#a/Q}"`,
			"xQy%bz",
		},
		{
			"at the other spelling too",
			`w='x#ay%bz'; printf '%s' "${w/%b/Q}"`,
			"x#ayQz",
		},
		{
			// An anchor with nothing behind it is the one-character
			// pattern `#`, which is bash's `Xabcabc`, ksh93's `abcabc` and
			// this shell's third answer again.
			"an anchor alone is a one-character pattern",
			`w='x#ay%bz'; printf '%s' "${w/#/Q}"`,
			"xQay%bz",
		},
		{
			// And with `//`, where the parser reads the second slash first
			// and the `#` is a pattern character in bash and ksh93 too.
			"the global spelling reads it the same way",
			`w='x#ay%bz'; printf '%s' "${w//#a/Q}"`,
			"xQy%bz",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runIn(t, tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The value itself, and the axis behind it that this shell can never reach.
func TestTheReplacementAnchorAnswers(t *testing.T) {
	s := ash.Semantics()
	if got, want := s.ReplacementAnchors, interp.No; got != want {
		t.Errorf("ReplacementAnchors = %v, want %v", got, want)
	}
	// No anchor is ever read here, so what an anchored *empty* pattern
	// matches is a question nothing in this shell can put. An answer would
	// be a guess recorded as a measurement.
	if got := s.AnchoredEmptyReplacementPattern; got != interp.Unspecified {
		t.Errorf("AnchoredEmptyReplacementPattern = %v, want unspecified", got)
	}
}
