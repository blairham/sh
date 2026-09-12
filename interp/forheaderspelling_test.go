// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A C-style `for` header's three parts are quoted back as the script wrote
// them, blanks and all.
//
// The clause's InitText, CondText and PostText are trimmed, and have to be:
// they are what syntax/print.go lays a header out from, and untrimmed parts
// print back with their blanks doubled. So the spelling comes from Header,
// which the parser already keeps verbatim and which SameProgram already skips
// (#2164).
//
// Two assertions rather than one, because the two halves fail differently: a
// *parse* failure quotes the text, and an *evaluation* failure slices it at an
// offset the parser recorded — so a tree built from the trimmed part and
// quoted from the untrimmed one is a character wide rather than a blank short.

func runForHeader(t *testing.T, src string) string {
	t.Helper()
	out, _ := runGrammar(t, src,
		func(d *syntax.Dialect) { d.CStyleFor = true },
		func(r *Runner) {
			// The wrapper is written so the test can read both halves at
			// once: the expression as quoted, then the token blamed.
			r.Diagnostics = &Diagnostics{
				ArithError:            `%[1]s|%[3]s`,
				ArithOperatorExpected: "operator",
				DivisionByZero:        "divide",
			}
		})
	return out
}

func TestAForHeaderPartIsQuotedAsItWasWritten(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// A part the parser could read, failing when it ran: the blank after
		// the divisor is what the offset has to land in front of.
		{"an evaluation failure", `for (( i=1/0 ;; )); do :; done`, "i=1/0 |0 "},
		// The same part with nothing around it, which is the control: with no
		// blanks the two readings agree and the row says nothing.
		{"one written tight", `for ((i=1/0;;)); do :; done`, "i=1/0|0"},
		// A part holding an expansion, which has no tree until it runs.
		{"a parse failure", `x="echo hi"; for (( $x ;;)); do :; done`, "echo hi |hi "},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out := runForHeader(t, c.src); !strings.Contains(out, c.want) {
				t.Errorf("got %q, want it to hold %q", out, c.want)
			}
		})
	}
}

// And the same parts reach the trace, which is where the *other* end of the
// blanks shows: both shells that trace a header part keep what followed it and
// drop what came before. Measured 2026-09-12 on `set -x; for (( i=0 ; i<1 ;
// i++ ))` — bash 5.3.15 writes `+ (( i=0  ))`, two blanks before the close,
// and zsh 5.9.2 `+zsh:1> i=0 `.
func TestAForHeaderPartIsTracedWithWhatFollowedItAndNotWhatCameBefore(t *testing.T) {
	out, _ := runGrammar(t, "set -x; for (( i=0 ; i<1 ; i++ )); do :; done",
		func(d *syntax.Dialect) { d.CStyleFor = true },
		func(r *Runner) {
			r.Diagnostics = &Diagnostics{TraceArithForPart: TraceArithBare}
		})
	for _, want := range []string{"i=0 \n", "i<1 \n", "i++ \n"} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q, want it to hold %q", out, want)
		}
	}
}
