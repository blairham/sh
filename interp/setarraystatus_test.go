// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `set -A` refusing a name that is not one is fatal on both routes and leaves
// a different number behind on each, in the dialect that answers so.
//
// The pairing is the whole test. The refusal is fatal either way and its
// sentence is byte-identical either way, so a probe that checked the
// diagnostic — or that ran only one route — cannot see this at all; the only
// observable is the status the shell exits with.
func TestSetArrayBadNameLeavesZeroFromACommandStringWhereAnswered(t *testing.T) {
	for _, tc := range []struct {
		why     string
		answer  Answer
		route   Route
		want    int
		wantErr bool
	}{
		{"answered yes, from a command string: 0", Yes, RouteCommandString, 0, true},
		{"answered yes, from a script file: the refusal's own 1", Yes, RouteScriptFile, 1, true},
		{"answered no, from a command string: 1 like every other route", No, RouteCommandString, 1, true},
		{"answered no, from a script file: 1", No, RouteScriptFile, 1, true},
	} {
		out, errs, st := setArrayStatusRun(t, "set -A 1bad v\necho after\n", tc.answer, tc.route)
		if st != tc.want {
			t.Errorf("%s: status %d, want %d", tc.why, st, tc.want)
		}
		// Fatal on every row: the words after it never run. A 0 that came
		// from the shell carrying on and succeeding would be a different
		// answer wearing the same number, and this is what tells them apart.
		if out != "" {
			t.Errorf("%s: output %q, want the refusal to have ended the shell", tc.why, out)
		}
		if (errs != "") != tc.wantErr {
			t.Errorf("%s: stderr %q, want a complaint = %v", tc.why, errs, tc.wantErr)
		}
	}
}

// The other refusals from the same builtin and the same shell keep their
// status on both routes, which is what makes the field above this one
// refusal's rather than a rule about `set`, about bad names, or about the
// route. Each was measured from `-c` in the shell that answers yes and each
// leaves 1.
func TestOnlySetArraysBadNameTakesTheZero(t *testing.T) {
	for _, tc := range []struct{ why, src string }{
		{"a bad option letter", "set -q\necho after\n"},
		{"an unknown `set -o` name", "set -o zzznosuch\necho after\n"},
		{"a bad name to another builtin", "unset 1x\necho after\n"},
	} {
		_, _, st := setArrayStatusRun(t, tc.src, Yes, RouteCommandString)
		if st == 0 {
			t.Errorf("%s: status 0 — the `set -A` answer has reached a refusal that is not it", tc.why)
		}
	}
}

// The zero is for a refusal that ended the shell, and only for that.
//
// The guard has two halves — the route, and the refusal having been fatal —
// and the second is what this covers: in a dialect where a bad name is *not*
// fatal, `set -A` still refuses the name and returns, and what it returns is
// the refusal's own status rather than 0. A guard that only asked about the
// route would zero that too, which is a `set` reporting a name it would not
// take and telling its caller it succeeded.
//
// Found by mutation: dropping `r.ctl == controlExit` from the guard survived
// every test in the tree.
func TestTheZeroIsOnlyForARefusalThatEndedTheShell(t *testing.T) {
	for _, tc := range []struct {
		why   string
		fatal Answer
		want  int
	}{
		{"fatal, so the shell ends and leaves 0 behind", Yes, 0},
		{"not fatal, so the refusal's own status stands", No, 1},
	} {
		_, errs, st := setArrayStatusRunFatality(t, "set -A 1bad v\n", Yes, RouteCommandString, tc.fatal)
		if errs == "" {
			t.Fatalf("%s: nothing was reported, so this row is not the refusal it names", tc.why)
		}
		if st != tc.want {
			t.Errorf("%s: status %d, want %d", tc.why, st, tc.want)
		}
	}
}

func setArrayStatusRun(t *testing.T, src string, answer Answer, route Route) (string, string, int) {
	t.Helper()
	return setArrayStatusRunFatality(t, src, answer, route, Yes)
}

func setArrayStatusRunFatality(t *testing.T, src string, answer Answer, route Route, fatal Answer) (string, string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.SetArrayLetter = Yes
	// The axes `set -A` reads on the way to the name, answered so the run
	// reaches the refusal rather than stopping at an unanswered question.
	sem.SetArrayOptionsContinuePastTheName = No
	sem.SetArrayWithNoValuesUnsetsTheName = No
	sem.ArrayBaseIsZero = Yes
	sem.BadNameToDeclarationFatal = fatal
	sem.BadSetOptionNameFatal = Yes
	sem.FatalErrorStatusIsOne = Yes
	sem.SetArrayBadNameLeavesZeroFromCommandString = answer
	var out, errs bytes.Buffer
	dir := t.TempDir()
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Route: route,
		Diagnostics: &Diagnostics{
			Location:               LocationColonLine,
			BuiltinBadName:         map[string]string{"set": "not an identifier: %[2]s"},
			BuiltinBadNameStatus:   1,
			SetInvalidOptionStatus: 1,
		},
		Dir: dir, Name: "testsh", Vars: map[string]string{"PATH": dir},
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}
