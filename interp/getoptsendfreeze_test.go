// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A freeze on the `getopts` name operand, and what it costs — which one
// dialect answers by *which path inside the builtin* reached it (#3183).
//
// One name, one freeze, and two answers separated by nothing but whether the
// scan had a letter to report: the run that reports "no more options" ends
// the script there, and the same freeze over a letter the scan found only
// reports and returns. No other axis here splits a fatality that way, which
// is why this is a question of its own rather than a reading of
// ReadonlyRefusalInABuiltinIsFatal — the dialect that answers this Yes
// answers that one No, and is right to.

func endFreezeSem(a Answer) Semantics {
	s := getoptsFrozenSem()
	s.GetoptsFrozenNameAtTheEndOfTheOptionsIsFatal = a
	return s
}

const endFreezeAxis = "a freeze on the `getopts` name ending the script where the scan had run out"

// The two answers, over the shape the issue was filed with.
func TestGetoptsFrozenNameAtTheEndOfTheOptionsIsAnAxis(t *testing.T) {
	const src = `set -- x; N=kept; readonly N; getopts "a:" N; echo "end st=$? N=[$N]"; echo after`
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
		status int
	}{
		// bash 5.3.20, bash 3.2.57, dash 0.5.12 and BusyBox ash 1.37.0: the
		// refusal is written and the rest of the script runs. The 1 is the
		// "no more options" status, which the refused name does not carry
		// away — see getoptsEnd.
		{"reports and carries on", No, "sh: N: frozen\nend st=1 N=[kept]\nafter\n", 0},
		// ksh93u+ 2012-08-01, measured 2026-09-17 over a script file, through
		// `-c` and through standard input: nothing after the refusal runs,
		// and the status carried out is the builtin's own 2 rather than the
		// shell's fatal status.
		{"ends the script", Yes, "sh: N: frozen\n", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := endFreezeSem(tc.answer)
			dg := Diagnostics{ReadonlyVariable: "%s: frozen"}
			out, status := run(t, src, func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if status != tc.status {
				t.Errorf("status = %d, want %d", status, tc.status)
			}
		})
	}
}

// The status the fatal answer carries out is the builtin's own and not the
// shell's, which is a second claim and not the same one: a shell whose fatal
// errors are 1 exits 2 from this line. A model that let the fatal path set
// the status would pass the test above and fail here.
func TestGetoptsFrozenNameAtTheEndOfTheOptionsCarriesTheBuiltinStatus(t *testing.T) {
	sem := endFreezeSem(Yes)
	sem.FatalErrorStatusIsOne = Yes
	dg := Diagnostics{ReadonlyVariable: "%s: frozen"}
	_, status := run(t, `set -- x; N=kept; readonly N; getopts "a:" N; echo after`,
		func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
	if status != 2 {
		t.Errorf("status = %d, want 2 — the builtin's refused status rather than the shell's fatal one", status)
	}
}

// The other half of the split, and the half that makes this an axis of its
// own: a letter the scan *found* is answered the same way whichever way this
// one is set, so a dialect that ends the script at the end of the options
// still reports and returns over a letter.
func TestGetoptsFrozenNameOverALetterIsNotAsked(t *testing.T) {
	const src = `set -- -a v; N=kept; readonly N; getopts "a:" N; echo "letter st=$? N=[$N]"; echo after`
	const want = "sh: N: frozen\nletter st=2 N=[kept]\nafter\n"
	for _, answer := range []Answer{No, Yes, Unspecified} {
		t.Run(answer.String(), func(t *testing.T) {
			sem := endFreezeSem(answer)
			dg := Diagnostics{ReadonlyVariable: "%s: frozen"}
			out, _ := run(t, src, func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
			if out != want {
				t.Errorf("got %q, want %q", out, want)
			}
		})
	}
}

// Every way of running out is the same question. The scan ends on a word that
// is not an option, on a `--`, and on there being no words left at all, and
// the silent form ends on all three as well — so the axis is about the path
// and not about the shape that ended it.
func TestGetoptsFrozenNameAtTheEndOfTheOptionsOverEveryWayOfRunningOut(t *testing.T) {
	for _, tc := range []struct{ name, params, spec string }{
		{"a non-option word", `set -- x`, `"a:"`},
		{"a double dash", `set -- --`, `"a:"`},
		{"no words at all", `set --`, `"a:"`},
		{"the silent form", `set -- x`, `":a:"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.params + `; N=kept; readonly N; getopts ` + tc.spec + ` N; echo after`
			dg := Diagnostics{ReadonlyVariable: "%s: frozen"}
			sem := endFreezeSem(Yes)
			out, status := run(t, src, func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
			if want := "sh: N: frozen\n"; out != want {
				t.Errorf("fatal: got %q, want %q", out, want)
			}
			if status != 2 {
				t.Errorf("fatal: status = %d, want 2", status)
			}
			sem = endFreezeSem(No)
			out, _ = run(t, src, func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
			if want := "sh: N: frozen\nafter\n"; out != want {
				t.Errorf("reporting: got %q, want %q", out, want)
			}
		})
	}
}

// An unanswered axis is reported rather than guessed — but only where a
// freeze actually refused that one write. An ordinary `getopts` loop over
// names nothing has frozen reaches no question at all, which is what keeps
// this off the path every use of the builtin takes.
func TestGetoptsFrozenNameAtTheEndOfTheOptionsIsAskedOnlyOnARefusal(t *testing.T) {
	dg := Diagnostics{ReadonlyVariable: "%s: frozen"}
	t.Run("refused", func(t *testing.T) {
		sem := endFreezeSem(Unspecified)
		out, _ := run(t, `set -- x; N=kept; readonly N; getopts "a:" N; echo after`,
			func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
		if !strings.Contains(out, endFreezeAxis) {
			t.Fatalf("got %q, want it to carry %q", out, endFreezeAxis)
		}
	})
	t.Run("nothing frozen", func(t *testing.T) {
		sem := endFreezeSem(Unspecified)
		out, _ := run(t, `set -- -a v; N=kept; while getopts "a:" N; do printf '[%s]' "$N"; done; echo "[$N] after"`,
			func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
		if want := "[a][?] after\n"; out != want {
			t.Errorf("got %q, want %q", out, want)
		}
	})
}

// The dialect that leaves the name alone when the scan runs out never reaches
// this at all: there is no write for the freeze to refuse, so the unanswered
// axis is quiet and the builtin reports 1.
func TestGetoptsFrozenNameIsNotAskedWhereTheEndWritesNoName(t *testing.T) {
	sem := endFreezeSem(Unspecified)
	sem.GetoptsEndOfOptionsNamesIt = No
	dg := Diagnostics{ReadonlyVariable: "%s: frozen"}
	out, _ := run(t, `set -- x; N=kept; readonly N; getopts "a:" N; echo "st=$? N=[$N]"; echo after`,
		func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
	if want := "st=1 N=[kept]\nafter\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
