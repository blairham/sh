// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runIndirectSource runs src with `${!x}` taken as an indirection and both
// refusal wordings in place, so a case can say *which* of the two sentences a
// source earns — or that it earns neither.
func runIndirectSource(t *testing.T, src string) (string, int) {
	t.Helper()
	sem := namerefAimSemantics()
	sem.IndirectionYieldsName = No
	sem.FailedExpansionAbandonsTheLine = Yes
	sem.FatalErrorStatusIsOne = Yes
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ParamIndirection = true
		// A digit carries the name on after `${!`, which is what makes the
		// positional the *source* of an indirection rather than making the
		// `!` the parameter. See syntax.Dialect.ParamBangNameContinues.
		d.ParamBangNameContinues = "0123456789#?@*["
	}, func(r *Runner) {
		s := sem
		r.Semantics = &s
		r.Diagnostics = &Diagnostics{
			IndirectionUndeclared: "%[1]s: undeclared",
			IndirectionNotAName:   "%[1]s: not a name",
			UnboundVariable:       "%[1]s: unbound",
		}
	})
}

// Only a **name** can be undeclared, so an unset positional parameter is not
// the undeclared refusal's subject — it is the ordinary unset road, with the
// indirection's own `!` on the subject (#3985).
//
// Measured 2026-09-21 from a script file, one expansion per line, with no
// positional parameters at all: bash 5.3.20 answers `${!1}` under `set -u`
// with `!1: unbound variable`, answers it without `set -u` with the empty
// string at status 0, and gives `${!1-D}` the word `D`. Every one of those
// was `1: invalid indirect expansion` here, which cost the operators their
// word and `set -u` its own sentence.
//
// The sentences are the dialect's and the digit is the subject, so this names
// two Diagnostics fields rather than a shell: what it holds is that the
// undeclared wording is not the one a digit reaches.
func TestAnUnsetPositionalIsNotAnUndeclaredName(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
		absent          string
	}{
		{
			name:   "under nounset the unset road, with the sigil",
			src:    "set -u\necho \"A[${!1}]\"\n",
			want:   "!1: unbound",
			absent: "undeclared",
			status: 1,
		},
		{
			name:   "a second position is the same road",
			src:    "set -u\necho \"B[${!9}]\"\n",
			want:   "!9: unbound",
			absent: "undeclared",
			status: 1,
		},
		{
			name:   "with a positional present and its value unset",
			src:    "set -- nope\nset -u\necho \"C[${!1}]\"\n",
			want:   "!1: unbound",
			absent: "undeclared",
			status: 1,
		},
		{
			// The row that says this is the undeclared *sentence* and not
			// the whole road: an operator supplies the value, exactly as it
			// does for any other unset parameter.
			name:   "an operator supplies the word",
			src:    "set -u\necho \"D[${!1-DEF}]\"\n",
			want:   "D[DEF]",
			absent: "undeclared",
			status: 0,
		},
		{
			// And the control that keeps the refusal: a name nothing ever
			// declared is still refused, under the same vector and the same
			// wording.
			name:   "a name nothing declared is still refused",
			src:    "echo \"E[${!nodecl}]\"\n",
			want:   "nodecl: undeclared",
			absent: "E[",
			status: 1,
		},
		{
			// The second control. A positional that *is* set and holds
			// something that is not a parameter reference reaches the other
			// sentence, so the digit is not simply exempt from both.
			name:   "a positional holding nothing is not a name",
			src:    "set -- \"\"\necho \"F[${!1}]\"\n",
			want:   ": not a name",
			absent: "undeclared",
			status: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runIndirectSource(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q at %d, want %q in it", out, st, tc.want)
			}
			if strings.Contains(out, tc.absent) {
				t.Errorf("got %q, want no %q", out, tc.absent)
			}
			if st != tc.status {
				t.Errorf("got status %d, want %d (output %q)", st, tc.status, out)
			}
		})
	}

	// Without `set -u` the line is not refused at all, which is the row the
	// undeclared sentence was costing outright: the expansion is empty and
	// the script carries on.
	out, st := runIndirectSource(t, "echo \"G[${!1}]\"\necho after\n")
	if !strings.Contains(out, "G[]") || !strings.Contains(out, "after") || st != 0 {
		t.Errorf("got %q at %d, want an empty expansion at 0 and the script running on", out, st)
	}
}
