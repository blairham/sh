// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// One dialect answers a failed expansion with a different status depending on
// how the shell was started, and on nothing else — the same two lines given
// with `-c` and read from a file exit differently.
//
// Both doors to it: an unset parameter under `set -u`, and one `${x?}` was
// asked about.
func TestAFailedExpansionMayAnswerByHowTheShellStarted(t *testing.T) {
	for _, src := range []string{
		"set -u\nunset V\necho \"$V\"\n",
		"unset V\necho \"${V?}\"\n",
	} {
		for _, c := range []struct {
			name          string
			commandString bool
			answer        int
			want          int
		}{
			{"from a file, the ordinary fatal status", false, 127, 1},
			{"from a command string, this dialect's own", true, 127, 127},
			{"and with no answer, the ordinary one either way", true, 0, 1},
			{"which is what a file gets too", false, 0, 1},
		} {
			t.Run(c.name, func(t *testing.T) {
				if got := cmdStringStatus(t, src, c.commandString, c.answer); got != c.want {
					t.Errorf("%q: status = %d, want %d", src, got, c.want)
				}
			})
		}
	}
}

// It is the expansion that answers this way and not every way the shell
// stops. Measured over eight other ways bash stops, none of which cares how
// it was invoked — so a shell that applied this everywhere would be wrong
// about all of them.
func TestOnlyAFailedExpansionAnswersByHowTheShellStarted(t *testing.T) {
	// A command that does not exist stops nothing and reports its own
	// status, whatever the invocation was.
	for _, cs := range []bool{true, false} {
		if got := cmdStringStatus(t, "nosuchcmd\n", cs, 127); got == 0 {
			t.Errorf("commandString=%v: status = %d, want a failure", cs, got)
		}
		if got := cmdStringStatus(t, "exit 3\n", cs, 127); got != 3 {
			t.Errorf("commandString=%v: status = %d, want 3", cs, got)
		}
	}
}

func cmdStringStatus(t *testing.T, src string, commandString bool, answer int) int {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	sem.FatalErrorStatusIsOne = Yes
	dg := Diagnostics{ExpansionFailureStatusFromCommandString: answer}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "sh", Route: routeFor(commandString),
		Stdout: &buf, Stderr: &buf,
	})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// A readonly reassignment answers the **same** whichever route the program
// arrived by, and the separator between the statements is what decides how
// much it ends.
//
// This replaces a field that said otherwise. `ReadonlyReassignmentFatalFromCommandString`
// was measured from two cells — `-c` with a `;` against a file with newlines
// — which is the *diagonal* of route-by-separator and is confirmatory for
// either reading. The full square says the route decides nothing (#1182):
//
//	                 file+`;`   -c+`;`   file+NL   -c+NL
//	bash 5.3            1         1         0        0
//	bash-as-`sh`        1         1         0        0
//	bash 3.2            1         1         0        0
//	dash                2         2         2        2
//	ksh93               1         1         1        1
//	zsh                 1         1         1        1
//
// So both routes are asserted here against the same program, and the pair is
// the shape: a single cell cannot tell a route rule from a separator one.
func TestAReadonlyReassignmentAnswersTheSameByEitherRoute(t *testing.T) {
	for _, c := range []struct {
		name    string
		src     string
		fatal   Answer
		carried bool
	}{
		{"fatal, on separate lines", "readonly x=1\nx=2\necho after\n", Yes, false},
		{
			"not fatal, on separate lines — the line is given up and the next runs",
			"readonly x=1\nx=2\necho after\n", No, true,
		},
		{"fatal, in one list", "readonly x=1; x=2; echo after\n", Yes, false},
		{
			"not fatal, in one list — the rest of the list goes with it",
			"readonly x=1; x=2; echo after\n", No, false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, fromCommand := range []bool{false, true} {
				var buf strings.Builder
				sem := PosixSemantics()
				sem.ReadonlyReassignmentFatal = c.fatal
				sem.FatalErrorStatusIsOne = Yes
				r := newTestRunner(t, &Runner{
					Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
					Route: routeFor(fromCommand), Stdout: &buf, Stderr: &buf,
				})
				f, err := syntax.Parse(c.src, syntax.Core())
				if err != nil {
					t.Fatal(err)
				}
				if _, err := r.Run(context.Background(), f); err != nil {
					t.Fatal(err)
				}
				if carried := strings.Contains(buf.String(), "after"); carried != c.carried {
					t.Errorf("fromCommand=%v: said %q; carried on = %v, want %v",
						fromCommand, buf.String(), carried, c.carried)
				}
			}
		})
	}
}

// An assignment standing as a command of its own and a *declaration*
// assigning to the same name are two questions, which is what is left of the
// "two questions" the removed field was thought to make three of.
func TestOnlyAnAssignmentStandingAloneAsksTheOrdinaryQuestion(t *testing.T) {
	for _, c := range []struct {
		name, src           string
		base, byDeclaration Answer
		carried             bool
	}{
		{
			"a bare assignment reads the ordinary answer",
			"readonly x=1\nx=2\necho after\n", Yes, No, false,
		},
		{
			// The same two answers, and the declaration takes the other one:
			// if it read the ordinary answer this row would stop.
			"a declaration reads its own, even when the ordinary one is fatal",
			"readonly x=1\nexport x=2\necho after\n", Yes, No, true,
		},
		{
			"and the other way round, so neither is a default",
			"readonly x=1\nexport x=2\necho after\n", No, Yes, false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var buf strings.Builder
			sem := PosixSemantics()
			sem.ReadonlyReassignmentFatal = c.base
			sem.ReadonlyReassignmentByDeclarationFatal = c.byDeclaration
			sem.FatalErrorStatusIsOne = Yes
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
				Route: RouteCommandString, Stdout: &buf, Stderr: &buf,
			})
			f, err := syntax.Parse(c.src, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if carried := strings.Contains(buf.String(), "after"); carried != c.carried {
				t.Errorf("said %q; carried on = %v, want %v", buf.String(), carried, c.carried)
			}
		})
	}
}

// A declaration utility assigning to a readonly name is a third question:
// a different set of shells from either of the other two, and the same
// answer by both invocation routes.
func TestAReadonlyReassignmentByADeclaration(t *testing.T) {
	for _, src := range []string{
		"readonly x=1\nexport x=2\necho after\n",
		"readonly x=1\ntypeset x=2\necho after\n",
		"readonly x=1\nreadonly x=2\necho after\n",
	} {
		for _, c := range []struct {
			name          string
			byDeclaration Answer
			commandString bool
			carried       bool
		}{
			{"fatal, from a file", Yes, false, false},
			{"fatal, from an argument", Yes, true, false},
			{"not fatal, from a file", No, false, true},
			{"and not from an argument either", No, true, true},
		} {
			t.Run(c.name, func(t *testing.T) {
				var buf strings.Builder
				sem := PosixSemantics()
				// The other answer says nothing about this one, so it is
				// set the other way round to prove it.
				sem.ReadonlyReassignmentFatal = No
				sem.ReadonlyReassignmentByDeclarationFatal = c.byDeclaration
				sem.FatalErrorStatusIsOne = Yes
				r := newTestRunner(t, &Runner{
					Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
					Route: routeFor(c.commandString), Stdout: &buf, Stderr: &buf,
				})
				f, err := syntax.Parse(src, syntax.Core())
				if err != nil {
					t.Fatal(err)
				}
				if _, err := r.Run(context.Background(), f); err != nil {
					t.Fatal(err)
				}
				if carried := strings.Contains(buf.String(), "after"); carried != c.carried {
					t.Errorf("%q said %q; carried on = %v, want %v", src, buf.String(), carried, c.carried)
				}
			})
		}
	}
}

// And where it is fatal the status is the failure's, not the builtin's own —
// which is the half a builtin that returns a status of its own can undo.
func TestADeclarationYieldsTheFailuresStatus(t *testing.T) {
	var buf strings.Builder
	sem := PosixSemantics()
	sem.ReadonlyReassignmentByDeclarationFatal = Yes
	sem.FatalErrorStatusIsOne = No // so the failure's status is 2, not 0 or 1
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: &buf, Stderr: &buf,
	})
	f, err := syntax.Parse("readonly x=1\nexport x=2\necho after\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if st != 2 {
		t.Errorf("status = %d, want the failure's 2 rather than the builtin's own", st)
	}
}

// What a declaration says, which is not what a plain assignment says in
// every dialect — and where the builtin's name goes.
func TestWhatADeclarationSaysAboutAReadonlyName(t *testing.T) {
	for _, c := range []struct {
		name string
		dg   Diagnostics
		want string
		gone string
	}{
		{
			// Without a wording of its own the plain one stands, which is
			// three of the panel.
			"the plain wording by default",
			Diagnostics{ReadonlyVariable: "%[1]s: is read only"},
			"x: is read only", "export",
		},
		{
			// One dialect puts the builtin in front of the name, and which
			// builtins do is a set of its own: the wording alone is not
			// enough, because another dialect uses it for two of its four
			// declaration spellings and not the other two.
			"or one that names the builtin",
			Diagnostics{
				ReadonlyVariable:              "%[1]s: is read only",
				ReadonlyVariableInDeclaration: "%[2]s: %[1]s: is read only",
				ReadonlyRefusalNamesBuiltin:   map[string]bool{"export": true},
			},
			"export: x: is read only", "",
		},
		{
			// And the dialect that names the builtin in the *location* for
			// everything else does not name it here.
			"and never in the location",
			Diagnostics{
				Location:               LocationTightLine,
				NamesBuiltinInLocation: true,
				ReadonlyVariable:       "read-only variable: %[1]s",
			},
			"sh:2: read-only variable: x", "sh:export",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var buf strings.Builder
			sem := PosixSemantics()
			sem.ReadonlyReassignmentByDeclarationFatal = No
			dg := c.dg
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &dg, Name: "sh",
				Stdout: &strings.Builder{}, Stderr: &buf,
			})
			f, err := syntax.Parse("readonly x=1\nexport x=2\n", syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(buf.String(), c.want) {
				t.Errorf("said %q, want %q in it", buf.String(), c.want)
			}
			if c.gone != "" && strings.Contains(buf.String(), c.gone) {
				t.Errorf("said %q, want %q not in it", buf.String(), c.gone)
			}
		})
	}
}

// routeFor turns the two-way question these tests ask — was the program a
// command string — into the route the runner carries. A script file is the
// other side rather than "no route": the axes below are the difference
// between `-c` and a file, and a Runner nobody told is neither.
func routeFor(commandString bool) Route {
	if commandString {
		return RouteCommandString
	}
	return RouteScriptFile
}
