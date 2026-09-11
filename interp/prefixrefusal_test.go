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

// What a refused assignment prefix *costs*, by axis and never by shell.
//
// Three axes rather than one, because the panel answers three questions here
// and no two shells answer them alike: whether the refusal happens at all in
// front of a regular builtin, whether it ends the script, and whether the
// command it stood in front of still runs. See #1219.
//
// The command kind is the variable held loose here and fixed everywhere else,
// which is the correction the issue records: an earlier reading held `true`
// fixed throughout and got one column's answer backwards for five of the nine
// kinds it was later measured over.

type prefixCost struct {
	refusedBeforeARegularBuiltin Answer
	fatality                     PrefixRefusalFatalityPolicy
	costsTheCommand              Answer
}

func prefixCostRun(t *testing.T, c prefixCost, src string) (stdout, stderr string, status int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.PrefixToARegularBuiltinIsRefused = c.refusedBeforeARegularBuiltin
	sem.PrefixRefusalFatality = c.fatality
	sem.PrefixRefusalCostsTheCommand = c.costsTheCommand
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem,
		Diagnostics: &Diagnostics{
			Location:         LocationLineWord,
			ReadonlyVariable: "%s: readonly variable",
			NotFound:         "%[1]s: command not found",
		},
		Dir: t.TempDir(), Name: "testsh",
		Vars: map[string]string{"PATH": "/bin:/usr/bin"},
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v\nstderr: %s", src, rerr, errs.String())
	}
	return out.String(), errs.String(), st
}

// The three readings the panel has, on the command kind each of them is
// distinguished by. `;` separators throughout, because that is the only shape
// that can tell "did not give the list up" from "gave it up": on separate
// lines every reading reaches the line after.
func TestWhatARefusedAssignmentPrefixCosts(t *testing.T) {
	// The status a fatal refusal leaves is the dialect's for a fatal error
	// and not this axis's — 1 in ksh93 and zsh, 2 in dash, and this preset's
	// is the standard's.
	const fatalStatus = 2
	costsNothing := prefixCost{Yes, PrefixRefusalNeverFatal, No}
	alwaysFatal := prefixCost{Yes, PrefixRefusalAlwaysFatal, Unspecified}
	byResolvedKind := prefixCost{No, PrefixRefusalFatalOnASpecialBuiltinOrFunction, Yes}
	byWrittenWord := prefixCost{Yes, PrefixRefusalFatalOnACommandThisShellRuns, Yes}

	for _, tc := range []struct {
		name   string
		c      prefixCost
		src    string
		out    string
		errs   string
		status int
	}{
		// Costs nothing: reported, the command runs, the list carries on and
		// the status is the command's.
		{
			"nothing: an external command still runs",
			costsNothing,
			"readonly x=1\nx=2 /bin/echo RAN; echo after",
			"RAN\nafter\n", "testsh: line 2: x: readonly variable\n", 0,
		},
		{
			"nothing: a builtin still runs",
			costsNothing,
			"readonly x=1\nx=2 echo RAN; echo after",
			"RAN\nafter\n", "testsh: line 2: x: readonly variable\n", 0,
		},
		// Always fatal: the script ends wherever the prefix was, whatever it
		// stood in front of.
		{
			"always: an external command ends the script",
			alwaysFatal,
			"readonly x=1\nx=2 /bin/echo RAN; echo after",
			"", "testsh: line 2: x: readonly variable\n", fatalStatus,
		},
		{
			"always: so does a regular builtin",
			alwaysFatal,
			"readonly x=1\nx=2 echo RAN; echo after",
			"", "testsh: line 2: x: readonly variable\n", fatalStatus,
		},
		// By the resolved kind: silent in front of a regular builtin, the
		// command lost in front of an external one, the script lost in front
		// of a special builtin or a function.
		{
			"resolved: a regular builtin is not refused at all",
			byResolvedKind,
			"readonly x=1\nx=2 echo RAN; echo after",
			"RAN\nafter\n", "", 0,
		},
		{
			"resolved: and `command` naming one is not either",
			byResolvedKind,
			"readonly x=1\nx=2 command echo RAN; echo after",
			"RAN\nafter\n", "", 0,
		},
		{
			"resolved: an external command is lost and the list is not",
			byResolvedKind,
			"readonly x=1\nx=2 /bin/echo RAN; echo after",
			"after\n", "testsh: line 2: x: readonly variable\n", 0,
		},
		{
			"resolved: `command` naming an external one is the same",
			byResolvedKind,
			"readonly x=1\nx=2 command /bin/echo RAN; echo after",
			"after\n", "testsh: line 2: x: readonly variable\n", 0,
		},
		{
			"resolved: a special builtin ends the script",
			byResolvedKind,
			"readonly x=1\nx=2 :; echo after",
			"", "testsh: line 2: x: readonly variable\n", fatalStatus,
		},
		{
			"resolved: and so does a function",
			byResolvedKind,
			"readonly x=1\nf() { echo INFUNC; }\nx=2 f; echo after",
			"", "testsh: line 3: x: readonly variable\n", fatalStatus,
		},
		// By the written word: everything the shell runs itself ends the
		// script, and `command` is not looked through.
		{
			"written: a regular builtin ends the script",
			byWrittenWord,
			"readonly x=1\nx=2 echo RAN; echo after",
			"", "testsh: line 2: x: readonly variable\n", fatalStatus,
		},
		{
			"written: `command` naming one does not",
			byWrittenWord,
			"readonly x=1\nx=2 command echo RAN; echo after",
			"after\n", "testsh: line 2: x: readonly variable\n", 0,
		},
		{
			"written: an external command loses the command alone",
			byWrittenWord,
			"readonly x=1\nx=2 /bin/echo RAN; echo after",
			"after\n", "testsh: line 2: x: readonly variable\n", 0,
		},
		{
			"written: a function ends the script",
			byWrittenWord,
			"readonly x=1\nf() { echo INFUNC; }\nx=2 f; echo after",
			"", "testsh: line 3: x: readonly variable\n", fatalStatus,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := prefixCostRun(t, tc.c, tc.src)
			if out != tc.out {
				t.Errorf("stdout = %q, want %q", out, tc.out)
			}
			if errs != tc.errs {
				t.Errorf("stderr = %q, want %q", errs, tc.errs)
			}
			if st != tc.status {
				t.Errorf("status = %d, want %d", st, tc.status)
			}
		})
	}
}

// A command whose prefix is lost reports 1 and not the status the command
// would have earned: the lookup never happens, so a name nothing can run is
// not "not found".
func TestALostCommandReportsTheRefusalsStatus(t *testing.T) {
	out, errs, st := prefixCostRun(t,
		prefixCost{Yes, PrefixRefusalNeverFatal, Yes},
		"readonly x=1\nx=2 nosuchcmd_zz; echo after")
	if want := "testsh: line 2: x: readonly variable\n"; errs != want {
		t.Errorf("stderr = %q, want %q — no `command not found` follows a lookup that never happened", errs, want)
	}
	if out != "after\n" || st != 0 {
		t.Errorf("out = %q (status %d), want the list to carry on", out, st)
	}
}

// No answer here gives up the rest of the *command list* without also ending
// the script. One column does — bash invoked as `sh` — and no preset answers
// for it, so no value produces it; the state was ours in two dialects, which
// is what the `;` cells of #1219 measured.
func TestNoAnswerAbandonsTheListAlone(t *testing.T) {
	for _, c := range []prefixCost{
		{Yes, PrefixRefusalNeverFatal, No},
		{Yes, PrefixRefusalNeverFatal, Yes},
		{No, PrefixRefusalFatalOnASpecialBuiltinOrFunction, Yes},
		{Yes, PrefixRefusalFatalOnACommandThisShellRuns, Yes},
	} {
		out, _, _ := prefixCostRun(t, c,
			"readonly x=1\nx=2 /bin/echo RAN; echo after")
		if !strings.Contains(out, "after") {
			t.Errorf("with %+v the rest of the list went too: %q", c, out)
		}
	}
}

// The three axes are only asked where a name in the prefix is actually
// frozen, which is what keeps them off the common path. With no answers at
// all, an ordinary prefix still works.
func TestAnUnfrozenPrefixAsksNoAxis(t *testing.T) {
	none := prefixCost{Unspecified, PrefixRefusalFatalityUnspecified, Unspecified}
	for _, src := range []string{
		"y=2 /bin/echo RAN",
		"y=2 echo RAN",
		"f() { echo RAN; }\ny=2 f",
		"y=2 :; echo RAN",
	} {
		out, errs, st := prefixCostRun(t, none, src)
		if errs != "" || st != 0 || !strings.Contains(out, "RAN") {
			t.Errorf("%q = %q / %q (status %d), want it to run with nothing said",
				src, out, errs, st)
		}
	}
}

// And with a frozen name they are refused by name rather than guessed at:
// four answers for the fatality, none of which contains another.
func TestAnUnansweredPrefixRefusalIsRefused(t *testing.T) {
	none := prefixCost{Unspecified, PrefixRefusalFatalityUnspecified, Unspecified}
	out, errs, _ := prefixCostRun(t, none, "readonly x=1\nx=2 /bin/echo RAN; echo after")
	if !strings.Contains(errs, "what a refused assignment prefix costs") ||
		!strings.Contains(errs, "no dialect was chosen") {
		t.Errorf("stderr = %q, want the axis named in the refusal", errs)
	}
	// The command does not run under an axis nobody answered, which is the
	// half of it a status the next command overwrites cannot show.
	if strings.Contains(out, "RAN") {
		t.Errorf("stdout = %q, want the command left unrun", out)
	}
}
