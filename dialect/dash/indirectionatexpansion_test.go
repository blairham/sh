// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/syntax"
)

// `${!name}` is refused **past the parse** here, not while reading.
//
// This shell has no indirection, so the expansion is unreadable either way —
// what parts the two readings is *where* the refusal lands, and `set -n` is
// the only instrument that can see it: a construct the parser refuses is
// refused wherever it stands, and one the expander refuses only where the
// words are read.
//
// Measured 2026-09-28 from a script file under `env -i PATH=/usr/bin:/bin`
// with a scratch `HOME`, against `/bin/dash` `dash 0.5.12` (`go version -m`:
// *not a Go executable*):
//
//	                                  expanded        set -n
//	printf "[%s]" "${!x}"             Bad subst, 2    silent, 0
//	{ printf "[%s]" "${!x}"; }        Bad subst, 2    silent, 0
//	if :; then printf … "${!x}"; fi   Bad subst, 2    silent, 0
//	printf "[%s]" "${9nope}" (ctrl)   Bad subst, 2    silent, 0
//	printf "[%s]" "${x      (ctrl)    —               refused, 2
//
// The `${9nope}` row is the control and it is the whole argument: an
// expansion this grammar has always deferred answers every position exactly
// as `${!x}` does. The unterminated row is the other control and says a
// silent cell is this shell declining to refuse rather than `set -n` being
// unable to report a parse failure at all.
func TestIndirectionIsRefusedPastTheParse(t *testing.T) {
	if !dash.Dialect().IndirectionRefusedAtExpansion {
		t.Error("IndirectionRefusedAtExpansion = false, want true")
	}
	for _, tc := range []struct {
		name, src string
		parses    bool
	}{
		{"an indirection at the top level", `printf "[%s]" "${!x}"`, true},
		{"one in a group", `{ printf "[%s]" "${!x}"; }`, true},
		{"one inside an if", `if :; then printf "[%s]" "${!x}"; fi`, true},
		// The control: an expansion this grammar already deferred. If the
		// rows above were a parse refusal, this one would have to be too.
		{"a name no expansion can read", `printf "[%s]" "${9nope}"`, true},
		// And the falsifying control: something the parser really does
		// refuse, so a `parses` of true above is a reading rather than a
		// test that cannot fail.
		{"an unterminated expansion", `printf "[%s]" "${x`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := syntax.Parse(tc.src+"\n", dash.Dialect())
			if got := err == nil; got != tc.parses {
				t.Errorf("%q: parses = %v, want %v (err %v)", tc.src, got, tc.parses, err)
			}
		})
	}
}

// And the refusal still arrives, with its wording and status unchanged, where
// the words *are* read. Deferring it must not lose it — a change that stopped
// refusing everywhere would pass the test above and fail every one of these.
func TestIndirectionStillRefusesWhereItIsExpanded(t *testing.T) {
	for _, src := range []string{
		`x=y; y=V; printf "[%s]" "${!x}"`,
		`x=y; y=V; { printf "[%s]" "${!x}"; }`,
		`x=y; y=V; if :; then printf "[%s]" "${!x}"; fi`,
	} {
		// The helper hands back the combined output, so what is asserted is
		// the diagnostic itself: this shell's wording for an unreadable expansion.
		out, st := runDash(t, t.TempDir(), src)
		if st != 2 || !strings.Contains(out, "Bad substitution") {
			t.Errorf("%q: out %q at %d, want Bad substitution at 2", src, out, st)
		}
	}
}
