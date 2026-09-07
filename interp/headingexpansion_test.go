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

// A failed expansion in a compound command's **heading** ends the construct,
// and does not let it choose or iterate.
//
// The heading is what is expanded *before* the construct decides what to run:
// the `for` word list, the `select` menu, the `case` subject and the parts of
// an arithmetic `for`. #1171 fixed the simple-command path and left these,
// because they never tested the flag at all — so the loop ran its body and
// the `case` chose an arm over a word the shell had just reported it could
// not read, at status 0 (#1215).
//
// The operand is a division by zero on purpose. It is a failed expansion in
// **all six** panel columns, where the two obvious alternatives are not:
// `${(P)x}` is a real parameter flag in zsh and expands quietly there, so it
// only shows this against the bash dialect; and `set -u` on an unset name is
// fatalExpansion, which ends the shell in every dialect and was never the
// bug. Three plausible probes, one discriminates.

func headingRun(t *testing.T, src string, abandons Answer) (string, string, int) {
	t.Helper()
	d := syntax.Core()
	d.CStyleFor, d.Select = true, true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.FailedExpansionAbandonsTheLine = abandons
	var out, errs bytes.Buffer
	dir := t.TempDir()
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem,
		Diagnostics: &Diagnostics{Location: LocationLineWord},
		Dir:         dir, Name: "testsh", Vars: map[string]string{"PATH": dir},
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}

func TestAFailedHeadingDoesNotRunTheConstruct(t *testing.T) {
	for _, tc := range []struct{ name, src, mustNot string }{
		{
			"a for word list",
			"echo pre\nfor i in \"$((1/0))\"; do echo A; done\necho after",
			"A",
		},
		{
			// The word list is a list: a failure anywhere in it costs the
			// loop rather than the one pass it would have been. This ran
			// three times.
			"a for word list with good words around the bad one",
			"echo pre\nfor i in a \"$((1/0))\" b; do echo \"A$i\"; done\necho after",
			"A",
		},
		{
			"a case subject",
			"echo pre\ncase \"$((1/0))\" in *) echo A;; esac\necho after",
			"A",
		},
		{
			// The worst face of it. A subject that failed is empty, and
			// empty *matches*, so the shell chose a branch from a value it
			// had just said it could not compute — and exited 0.
			"a case subject where the empty arm would match",
			"echo pre\ncase \"$((1/0))\" in \"\") echo E;; *) echo A;; esac\necho after",
			"E",
		},
		{
			// A part whose expansion failed leaves text the arithmetic
			// cannot parse either, so this reported the division and then a
			// second complaint about the `i=` that was left behind.
			"an arithmetic for heading",
			"echo pre\nfor (( i=$((1/0)); i<2; i++ )); do echo A; done\necho after",
			"A",
		},
		{
			// Not a wrong answer but a hang: it printed a numbered menu and
			// blocked at the prompt for a choice among words the shell had
			// said it could not read.
			"a select menu",
			"echo pre\nselect i in \"$((1/0))\"; do echo A; break; done\necho after",
			"A",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := headingRun(t, tc.src, No)
			if !strings.Contains(out, "pre") {
				t.Fatalf("output %q, want the line before it to have run", out)
			}
			if errs == "" {
				t.Fatalf("nothing was reported, so this row is not the failure it names")
			}
			// Exactly one complaint. The arithmetic heading wrote two.
			if n := strings.Count(errs, "\n"); n != 1 {
				t.Errorf("reported %d lines, want one: %q", n, errs)
			}
			if strings.Contains(out, tc.mustNot) {
				t.Errorf("output %q, want no %q — the construct ran over a word it could not read",
					out, tc.mustNot)
			}
			// Ending the shell, which is what three of the four dialects do.
			if strings.Contains(out, "after") {
				t.Errorf("output %q, want the shell to have stopped", out)
			}
			if st == 0 {
				t.Errorf("status %d, want the failure's — 0 with a diagnostic on stderr "+
					"is what nothing downstream can detect", st)
			}
		})
	}
}

// Under the answer that gives up the *line*, the construct still does not run
// and the next line still does — which is the pairing that says this is the
// existing axis rather than a rule of its own.
func TestAFailedHeadingGivesUpTheLineWhereTheDialectSaysSo(t *testing.T) {
	for _, tc := range []struct{ name, src, mustNot string }{
		{"a for word list", "echo pre\nfor i in \"$((1/0))\"; do echo A; done\necho after", "A"},
		{"a case subject", "echo pre\ncase \"$((1/0))\" in \"\") echo E;; *) echo A;; esac\necho after", "E"},
		{"an arithmetic for heading", "echo pre\nfor (( i=$((1/0)); i<2; i++ )); do echo A; done\necho after", "A"},
		{"a select menu", "echo pre\nselect i in \"$((1/0))\"; do echo A; break; done\necho after", "A"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := headingRun(t, tc.src, Yes)
			if errs == "" {
				t.Fatalf("nothing was reported")
			}
			if strings.Contains(out, tc.mustNot) {
				t.Errorf("output %q, want no %q", out, tc.mustNot)
			}
			if !strings.Contains(out, "after") {
				t.Errorf("output %q, want the next line to run", out)
			}
			if st != 0 {
				t.Errorf("status %d, want the next line's 0", st)
			}
		})
	}
}

// The heading is cleared before it is expanded, and that is load-bearing
// rather than tidiness: a failed expansion that gave up its line leaves the
// flag set, and the statement loop that consumed the give-up does not clear
// it. Without the clear, the *next* line's heading abandoned itself over the
// previous line's failure — so a good loop after a bad command never ran.
func TestAHeadingIsNotAbandonedByThePreviousLinesFailure(t *testing.T) {
	out, _, st := headingRun(t,
		"echo pre\necho \"$((1/0))\"\nfor i in a b; do echo \"A$i\"; done\necho after", Yes)
	for _, want := range []string{"pre", "Aa", "Ab", "after"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q, want %q in it — the loop was given up over the line before it", out, want)
		}
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
}

// The control that says it is the heading and not the construct: the same
// failure in a loop **body** is the simple-command path #1171 already fixed,
// and it is per-line there under the answer that gives up a line.
func TestAFailureInTheBodyIsStillTheSimpleCommandsQuestion(t *testing.T) {
	out, _, st := headingRun(t,
		"echo pre\nfor i in 1 2; do echo \"$((1/0))\"; echo B; done\necho after", Yes)
	if strings.Contains(out, "B") {
		t.Errorf("output %q, want the rest of the body given up", out)
	}
	if !strings.Contains(out, "after") || st != 0 {
		t.Errorf("output %q status %d, want the shell to carry on at the next line", out, st)
	}
}

// A heading with nothing wrong in it is untouched, in every construct the
// change reached — the rows that would catch a guard that fired too often.
func TestAGoodHeadingIsUnaffected(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a for word list", "for i in a b; do echo \"A$i\"; done", "Aa\nAb\n"},
		{"a for over the positional parameters", "set -- x y\nfor i; do echo \"A$i\"; done", "Ax\nAy\n"},
		{"an empty for word list runs nothing", "for i in; do echo A; done\necho after", "after\n"},
		{"a case subject", "case b in a) echo A;; b) echo B;; esac", "B\n"},
		{"a case subject that is genuinely empty still matches the empty arm", "case \"\" in \"\") echo E;; *) echo A;; esac", "E\n"},
		{"an arithmetic for heading", "for (( i=0; i<2; i++ )); do echo \"A$i\"; done", "A0\nA1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := headingRun(t, tc.src, Yes)
			if out != tc.want {
				t.Errorf("output %q, want %q", out, tc.want)
			}
			if errs != "" {
				t.Errorf("said %q, want nothing", errs)
			}
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
		})
	}
}
