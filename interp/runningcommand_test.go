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

// The record of what the shell is running, which a dialect with a parameter
// naming it reads. See interp.RunningCommand for the measurements the sites
// were chosen from; what is asserted here is the *core's* half — where the
// record is written and where it is held still — with a neutral name over it,
// because which shell has such a parameter and how it renders one are not
// questions this package answers.

// runningName is the parameter these tests read the record through. It has no
// shell's spelling on purpose: the rendering below is the plainest one there
// is, so a row that moves is the record moving rather than a deparse changing.
const runningName = "RUNNING"

// runningRun runs src with the record produced under runningName.
func runningRun(t *testing.T, src string, set func(*Semantics)) string {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.TrapHasDebugCondition = Yes
	sem.DebugTrapRunsBeforeTheCommand = Yes
	sem.DebugTrapRunsInsideCalls = No
	sem.DebugTrapRunsInSubshells = No
	sem.DebugTrapRefiresOnEnteringAFunction = No
	if set != nil {
		set(&sem)
	}
	var out, errs bytes.Buffer
	dir := t.TempDir()
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh", Vars: map[string]string{"PATH": dir},
	})
	r.SetDynamic(runningName, func(rr *Runner) string {
		rc := rr.RunningCommand()
		if rc.Cmd == nil {
			return ""
		}
		return runningText(rc)
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	if errs.Len() != 0 {
		t.Fatalf("ran %q: stderr %q", src, errs.String())
	}
	return out.String()
}

// runningText is the neutral rendering: the node the record holds, plus the
// part where a command's parts run on their own.
func runningText(rc RunningCommand) string {
	switch rc.Part {
	case ArithInit:
		return "init"
	case ArithCond:
		return "cond"
	case ArithPost:
		return "post"
	}
	if _, ok := rc.Cmd.(*syntax.SimpleCmd); ok {
		return syntax.PrintCommand(rc.Cmd)
	}
	// A compound is named by its kind rather than printed, because what is
	// recorded is its *head* and printing the node whole would print the body
	// with it — which is the asking dialect's problem and not this one's.
	kind := strings.TrimPrefix(strings.TrimPrefix(syntax.PrintCommand(rc.Cmd), "("), "[")
	if i := strings.IndexAny(kind, " \t\n"); i > 0 {
		kind = kind[:i]
	}
	return "head:" + kind
}

// TestTheRunningCommandIsRecordedWhereTheDebugTrapFires pins the sites. The
// two questions are the same question — measured, not assumed — so a firing
// site added or moved without the record moving with it fails here.
func TestTheRunningCommandIsRecordedWhereTheDebugTrapFires(t *testing.T) {
	const act = "trap 'echo \"D:[$RUNNING]\"' DEBUG\n"
	for _, c := range []struct {
		name, src, want string
	}{
		{"each simple command", act + "echo one\necho two", "D:[echo one]\none\nD:[echo two]\ntwo\n"},
		{"a head that fires is the head", act + "case x in x) :;; esac", "D:[head:case]\nD:[:]\n"},
		{
			"a list loop's head, once per pass", act + "for w in a b; do :; done",
			"D:[head:for]\nD:[:]\nD:[head:for]\nD:[:]\n",
		},
		{
			"an arithmetic loop's three parts", act + "for ((i=0;i<1;i++)); do :; done",
			"D:[init]\nD:[cond]\nD:[:]\nD:[post]\nD:[cond]\n",
		},
		// The heads that fire nothing in this reading record nothing either,
		// which is what says the record follows the firing sites rather than
		// every node the dispatcher sees.
		{"an if records nothing of its own", act + "if true; then echo y; fi", "D:[true]\nD:[echo y]\ny\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := runningRun(t, c.src, nil); got != c.want {
				t.Errorf("ran %q: got %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// TestTheRunningCommandFollowsTheReading is the half that says the record is
// tied to the *dialect's* reading of which heads fire rather than to a fixed
// set: the reading that fires no head at all records none.
func TestTheRunningCommandFollowsTheReading(t *testing.T) {
	const src = "trap 'echo \"D:[$RUNNING]\"' DEBUG\ncase x in x) :;; esac"
	for _, c := range []struct {
		name  string
		heads DebugTrapHeads
		want  string
	}{
		{"heads that fire are recorded", DebugTrapHeadsWordAndArithmetic, "D:[head:case]\nD:[:]\n"},
		{"and a reading with no heads records none", DebugTrapHeadsNone, "D:[:]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := runningRun(t, src, func(s *Semantics) { s.DebugTrapCompoundHeads = c.heads })
			if got != c.want {
				t.Errorf("under %v: got %q, want %q", c.heads, got, c.want)
			}
		})
	}
}

// TestTheRunningCommandIsRecordedWithNoTrapSet is the rule that makes the
// record a fact about running commands rather than about the trap: it is
// written before a command's own words are expanded, whether anything is
// watching or not.
func TestTheRunningCommandIsRecordedWithNoTrapSet(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{"a command reading it reads itself", `echo "[$RUNNING]"`, `[echo "[$RUNNING]"]` + "\n"},
		{"and not the command before it", `true; echo "[$RUNNING]"`, `[echo "[$RUNNING]"]` + "\n"},
		// The list loop's head is the one site recorded *after* the head's
		// own work, so the word list still expands against the command
		// before the loop. That is where its firing is, and the two agreeing
		// is the point of recording at the firing sites.
		{
			"a list loop's head is recorded after its list is expanded",
			`true; for w in "$RUNNING"; do echo "[$w]"; done`, "[true]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := runningRun(t, c.src, nil); got != c.want {
				t.Errorf("ran %q: got %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// TestATrapBodyDoesNotMoveTheRunningCommand is the suspension, and it is what
// makes the record usable at all: an action that could not run a command of
// its own without losing the one it fired for could say nothing about it.
func TestATrapBodyDoesNotMoveTheRunningCommand(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{
			"a command in the body moves nothing",
			"trap 'true; echo \"D:[$RUNNING]\"' DEBUG\necho one", "D:[echo one]\none\n",
		},
		{
			"nor does a function the body calls",
			"f(){ echo inner; }\ntrap 'f; echo \"D:[$RUNNING]\"' DEBUG\necho one",
			"inner\nD:[echo one]\none\n",
		},
		// Every body and not only the DEBUG one, which is what lets an EXIT
		// action name where the script had got to.
		{
			"an exit body sees where the script reached",
			"trap 'echo \"X:[$RUNNING]\"' EXIT\necho one", "one\nX:[echo one]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := runningRun(t, c.src, nil); got != c.want {
				t.Errorf("ran %q: got %q, want %q", c.src, got, c.want)
			}
		})
	}
}
