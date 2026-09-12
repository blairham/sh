// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// `-c` with `-s`, where one dialect goes on to read standard input as a
// program once the command string has run.
//
// The axis is Semantics.StdinOptionSurvivesTheCommandString and the corpus
// grades it — `invoke/standard-input-still-follows-the-command-string` and the
// two rows beside it. These are here for the halves a corpus row cannot see:
// that the second program is run on the *same runner*, and that nothing else
// about the invocation reaches it.

// stdinAfter runs an invocation with src on a real descriptor and reports what
// the shell wrote.
//
// A real descriptor rather than a string, because that is the whole of the
// route: the program is read as it runs, off the same descriptor the script's
// own `read` would use.
func stdinAfter(t *testing.T, survives bool, stdin string, argv ...string) string {
	t.Helper()
	sh := shell()
	sem := interp.CoreSemantics()
	sem.StdinOptionSurvivesTheCommandString = survives
	sem.StdinOptionNamesTheOperands = interp.No
	// The EXIT trap below needs it, and it is not what is under test here.
	sem.TrapBodyRunsWhatParsed = interp.Yes
	sh.Semantics = sem
	path := writeScript(t, stdin)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	sh.Stdin = f
	out, errs, _ := runArgs(t, sh, argv...)
	if errs != "" {
		t.Fatalf("stderr: %s", errs)
	}
	return out
}

// TestTheStandardInputOptionMaySurviveTheCommandString is the axis, both ways.
//
// Measured 2026-09-12: `printf 'echo LINE1\necho LINE2\n' | <shell> -sc 'echo
// FROM-C'` writes all three lines in dash and FROM-C alone in bash 5.3, bash
// 3.2, bash-as-`sh`, ksh93 and zsh. Plain `-c` without `-s` stops in every
// column, which is what makes this the two options together.
func TestTheStandardInputOptionMaySurviveTheCommandString(t *testing.T) {
	const program = "echo LINE1\necho LINE2\n"
	for _, tc := range []struct {
		name     string
		survives bool
		argv     []string
		want     string
	}{
		{
			"it survives where the dialect says so",
			true,
			[]string{"testsh", "-sc", "echo FROM-C"},
			"FROM-C\nLINE1\nLINE2\n",
		},
		{
			"and stops where it does not",
			false,
			[]string{"testsh", "-sc", "echo FROM-C"},
			"FROM-C\n",
		},
		{
			// The control, and the reason this is the two options together
			// rather than a rule about `-c`: with the same program waiting
			// and no `-s`, even the dialect that goes on stops here.
			"without the standard-input option it stops either way",
			true,
			[]string{"testsh", "-c", "echo FROM-C"},
			"FROM-C\n",
		},
		{
			// And the order the two letters are written in changes nothing,
			// which is measured: `-cs`, `-sc` and `-s -c` all go on.
			"the order of the two options makes no difference",
			true,
			[]string{"testsh", "-cs", "echo FROM-C"},
			"FROM-C\nLINE1\nLINE2\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := stdinAfter(t, tc.survives, program, tc.argv...); got != tc.want {
				t.Errorf("out = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestTheTwoHalvesAreOneShell is what a corpus row can only see the edge of,
// and the reason the second program runs on the runner the first one used.
//
// Measured in dash: a variable, a function and a `cd` from the command string
// are all in effect for the standard-input half, `$0` and the positional
// parameters are the ones the invocation named, and one EXIT trap fires at the
// end of both rather than once per program.
//
// The `cd` is the one of the three that a *fresh runner* would not merely get
// wrong but get wrong invisibly: a second runner starts in the shell's own
// directory, which is where the first one started too, so the working
// directory only discriminates once something has moved it.
func TestTheTwoHalvesAreOneShell(t *testing.T) {
	const program = `echo "in: x=[$x] 0=$0 1=$1"
f
echo "in: pwd=$PWD"
`
	got := stdinAfter(t, true,
		program,
		"testsh", "-sc",
		`x=1; f() { echo "in: the function ran"; }; cd /; trap 'echo BYE' EXIT; echo "c: x=[$x] 0=$0 1=$1"`,
		"NAME", "a")
	for _, want := range []string{
		"c: x=[1] 0=NAME 1=a",
		"in: x=[1] 0=NAME 1=a",
		"in: the function ran",
		"in: pwd=/\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("out = %q, want it to hold %q", got, want)
		}
	}
	// One trap, at the end of both, and not one per program.
	if n := strings.Count(got, "BYE"); n != 1 {
		t.Errorf("out = %q, want exactly one EXIT trap and got %d", got, n)
	}
	if !strings.HasSuffix(got, "BYE\n") {
		t.Errorf("out = %q, want the trap last of all", got)
	}
}

// TestAnExitInTheCommandStringLeavesStandardInputUnread is the ending the axis
// deliberately does not model: it falls out of the shell having exited.
func TestAnExitInTheCommandStringLeavesStandardInputUnread(t *testing.T) {
	got := stdinAfter(t, true, "echo LINE1\n", "testsh", "-sc", "echo C; exit 7")
	if got != "C\n" {
		t.Errorf("out = %q, want the command string alone", got)
	}
}

// TestTheStandardInputHalfIsTheOrdinaryStandardInputRoute pins that it is not
// a third way of running a program.
//
// Measured in dash: the half numbers its own lines from 1 and reports through
// the standard-input diagnostics, exactly as a plain `sh -s` does. The line
// number is the observable — a half that carried the command string's own
// numbering would report 2 where a plain `-s` reports 1.
func TestTheStandardInputHalfIsTheOrdinaryStandardInputRoute(t *testing.T) {
	sh := shell()
	sem := interp.CoreSemantics()
	sem.StdinOptionSurvivesTheCommandString = true
	sem.StdinOptionNamesTheOperands = interp.No
	sh.Semantics = sem
	const program = "echo one\necho \"L=$LINENO\"\n"
	got := stdinAfterWith(t, sh, program, "testsh", "-sc", "echo \"c: L=$LINENO\"")
	if !strings.Contains(got, "c: L=1") {
		t.Errorf("out = %q, want the command string's own first line", got)
	}
	if !strings.Contains(got, "L=2") {
		t.Errorf("out = %q, want the half to number its own lines", got)
	}
}

// stdinAfterWith is stdinAfter for a caller that built the shell itself.
func stdinAfterWith(t *testing.T, sh driver.Shell, stdin string, argv ...string) string {
	t.Helper()
	path := writeScript(t, stdin)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	sh.Stdin = f
	out, _, _ := runArgs(t, sh, argv...)
	return out
}
