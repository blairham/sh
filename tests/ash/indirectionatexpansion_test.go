// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
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
// with a scratch `HOME`, against BusyBox `v1.38.0` in
// `busybox:latest`:
//
//	                                  expanded              set -n
//	printf "[%s]" "${!x}"             syntax err: bad …, 2  silent, 0
//	{ printf "[%s]" "${!x}"; }        syntax err: bad …, 2  silent, 0
//	if :; then printf … "${!x}"; fi   syntax err: bad …, 2  silent, 0
//	printf "[%s]" "${9nope}" (ctrl)   syntax err: bad …, 2  silent, 0
//	printf "[%s]" "${x      (ctrl)    —                     refused, 2
//
// **The wording is not evidence about the place**, and this column is where
// that matters most: `syntax error: bad substitution` reads like a parse
// failure, and it is the *same sentence* this shell gives `${9nope}` — an
// expansion every panel column defers. Under `set -n` both are silent at 0,
// so the sentence is this shell's wording for an unreadable expansion rather
// than a statement about when it was refused. The unterminated row is the
// other control and says a silent cell is this shell declining to refuse
// rather than `set -n` being unable to report a parse failure at all.
func TestIndirectionIsRefusedPastTheParse(t *testing.T) {
	if !ash.Dialect().IndirectionRefusedAtExpansion {
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
			_, err := syntax.Parse(tc.src+"\n", ash.Dialect())
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
		// the diagnostic itself: the wording that reads like a parse failure and is not one, since `${9nope}` carries it too.
		out, st := run(t, src)
		if st != 2 || !strings.Contains(out, "syntax error: bad substitution") {
			t.Errorf("%q: out %q at %d, want syntax error: bad substitution at 2", src, out, st)
		}
	}
}
