// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `$(<file)` whose *read* fails after the open worked, which a directory is
// the reachable shape of.
//
// Measured 2026-09-12, `mkdir dir; v=$(<dir); echo "st=$? v=[$v]"`:
//
//	zsh 5.9.2   st=1, and `error when reading dir: is a directory`
//	bash 3.2    st=1, silent
//	bash 5.3    st=0, silent
//	ksh93       st=0, silent
//	dash        no such form
//
// Two questions rather than one, and bash 3.2 is what separates them: it
// fails the substitution and says nothing, so neither the status nor the
// sentence can carry the other (#1778).
//
// An open that fails is a *different* event and is answered elsewhere — see
// the last case here, where the same shell that leaves a failed read at 0
// still reports 1 for a name that would not open.
const readADirectory = `mkdir dir; v=$(<dir); printf "st=%s v=[%s]" "$?" "$v"`

func TestAFailedReadInAFileSubstitutionIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name  string
		fails Answer
		want  string
	}{
		{"the substitution fails", Yes, "st=1 v=[]"},
		{"the substitution succeeds with nothing", No, "st=0 v=[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := permissive()
			sem.ReadFailureInAFileSubstitutionFailsIt = tc.fails
			out, st := run(t, readADirectory, withSem(sem))
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The sentence is the dialect's and moves on its own, which is the half bash
// 3.2 proves has to be separate: here it is said over a substitution the axis
// leaves succeeding.
func TestTheReadFailuresSentenceIsIndependentOfTheStatus(t *testing.T) {
	sem := permissive()
	sem.ReadFailureInAFileSubstitutionFailsIt = No
	out, _ := run(t, readADirectory, func(r *Runner) {
		r.Semantics = &sem
		r.Diagnostics = &Diagnostics{FileSubstitutionReadError: "cannot read %[1]s: %[2]s"}
	})
	if !strings.Contains(out, "cannot read dir: ") {
		t.Errorf("got %q, want the wording naming the operand", out)
	}
	if !strings.HasSuffix(out, "st=0 v=[]") {
		t.Errorf("got %q, want the status still 0 after it", out)
	}
}

// The name in the sentence is the word as it *expanded*, not as it was
// written and not the path the working directory made of it — which is what
// the operand being a variable is here to show.
func TestTheReadFailureNamesTheOperandAsItExpanded(t *testing.T) {
	sem := permissive()
	sem.ReadFailureInAFileSubstitutionFailsIt = Yes
	out, _ := run(t, `mkdir dir; d=dir; v=$(<$d); printf "[%s]" "$v"`, func(r *Runner) {
		r.Semantics = &sem
		r.Diagnostics = &Diagnostics{FileSubstitutionReadError: "reading %[1]s: %[2]s"}
	})
	if !strings.Contains(out, "reading dir: ") {
		t.Errorf("got %q, want the expanded name `dir`", out)
	}
}

// The axis is asked where the read fails and nowhere else, which keeps it off
// the path every `$(<file)` takes: a Runner with no answer still reads a file.
func TestTheReadFailureAxisIsAskedOnlyWhenTheReadFails(t *testing.T) {
	sem := permissive()
	sem.ReadFailureInAFileSubstitutionFailsIt = Unspecified

	out, st := run(t, `printf 'hello\n' > f; printf "[%s]" "$(<f)"`, withSem(sem))
	if want := "[hello]"; out != want || st != 0 {
		t.Errorf("a file that reads: got %q (status %d), want %q at 0", out, st, want)
	}

	out, _ = run(t, readADirectory, withSem(sem))
	if !strings.Contains(out, "`$(<file)`") {
		t.Errorf("a directory: %q does not name the axis", out)
	}
}

// An open that failed is not this, and the two answers are independent: the
// status a name that will not open leaves is redirectFailureStatus, which is
// already answered and is unmoved by this axis.
func TestAnOpenThatFailedIsNotAReadThatFailed(t *testing.T) {
	sem := permissive()
	sem.ReadFailureInAFileSubstitutionFailsIt = No
	out, st := run(t, `v=$(<nosuch); printf "st=%s" "$?"`, func(r *Runner) {
		r.Semantics = &sem
		r.Diagnostics = &Diagnostics{FileNotFound: "no such file"}
	})
	if !strings.HasSuffix(out, "st=1") || st != 0 {
		t.Errorf("got %q (status %d), want the substitution reported 1", out, st)
	}
}
