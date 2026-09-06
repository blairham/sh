// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// The prelude's diagnostics seam (#603): a function the dialect defined in
// shell speaks for the shell, so what it reports is located and named the way
// a builtin's complaint is.
//
// These name the *rule* and never a shell. Two location styles appear because
// the rule has to hold under both, and each is a field of Diagnostics that
// some dialect sets and others do not.

// preludeRun installs prelude the way a front end installs one, runs src, and
// returns standard error — where every diagnostic goes.
//
// The two Run calls on one runner are what driver.Shell.source does: a prelude
// is not a special entry point, and the script's lines are numbered from one
// because the prelude was never part of its text.
func preludeRun(t *testing.T, dir, prelude, src string, dg Diagnostics) string {
	t.Helper()
	pre, err := syntax.Parse(prelude, syntax.Core())
	if err != nil {
		t.Fatalf("parse prelude: %v", err)
	}
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out, errs bytes.Buffer
	sem := permissive()
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh", Vars: map[string]string{"PATH": dir},
	})
	r.SourcingPrelude(true)
	if _, rerr := r.Run(context.Background(), pre); rerr != nil {
		t.Fatalf("run prelude: %v", rerr)
	}
	r.SourcingPrelude(false)
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return errs.String()
}

// oneLine is the whole rendered line, which is what a diagnostic assertion has
// to compare.
//
// Not strings.Contains of the sentence: a prefix added in front of the text
// named is invisible to that, so a check for `stack: empty` passes whether the
// location is there or not — which is exactly the mistake this seam exists to
// correct.
func oneLine(t *testing.T, got, want string) {
	t.Helper()
	if !slices.Contains(strings.Split(got, "\n"), want) {
		t.Errorf("stderr = %q, want the whole line %q in it", got, want)
	}
}

// A function the prelude defined reaches the location prefix through the seam,
// and the sentence it hands over is the only part it writes.
func TestAPreludeFunctionsOwnDiagnosticIsLocated(t *testing.T) {
	const prelude = "stack() {\n\tdiagnose \"nothing to pop\"\n\treturn 1\n}\n"
	got := preludeRun(t, t.TempDir(), prelude, "true\nstack\n",
		Diagnostics{Location: LocationLineWord})
	oneLine(t, got, "testsh: line 2: stack: nothing to pop")
}

// And under the style that puts the name inside the location instead, the same
// prelude line renders the other way with nothing in the prelude to say so.
// That is the point of the seam: the dialect answers, the shell text does not.
func TestTheSeamRendersUnderEitherLocationStyle(t *testing.T) {
	const prelude = "stack() {\n\tdiagnose \"nothing to pop\"\n\treturn 1\n}\n"
	got := preludeRun(t, t.TempDir(), prelude, "true\nstack\n",
		Diagnostics{Location: LocationTightLine, NamesBuiltinInLocation: true})
	oneLine(t, got, "testsh:stack:2: nothing to pop")
}

// A builtin the function called is renamed and relocated: the script asked for
// `stack`, so `stack` is what refused, at the line the script wrote.
func TestABuiltinInsideAPreludeFunctionSpeaksAsTheFunction(t *testing.T) {
	const prelude = "stack() {\n\tcd /no/such/dir-xyz\n}\n"
	got := preludeRun(t, t.TempDir(), prelude, "true\ntrue\nstack\n",
		Diagnostics{Location: LocationLineWord, CdCannotChange: "cd: %[1]s: %[2]s"})
	oneLine(t, got, "testsh: line 3: stack: /no/such/dir-xyz: No such file or directory")
	if strings.Contains(got, "cd:") {
		t.Errorf("stderr = %q, the builtin behind the function is named", got)
	}
}

// The name is the one the script used. A prelude helper called by another
// prelude function does not take it over, which is what keeps an internal
// split invisible.
func TestTheOutermostPreludeFunctionOwnsTheName(t *testing.T) {
	const prelude = "helper() {\n\tdiagnose \"out of range\"\n}\nstack() {\n\thelper\n}\n"
	got := preludeRun(t, t.TempDir(), prelude, "stack\n",
		Diagnostics{Location: LocationLineWord})
	oneLine(t, got, "testsh: line 1: stack: out of range")
	if strings.Contains(got, "helper") {
		t.Errorf("stderr = %q, the inner function took the name over", got)
	}
}

// A script's own function is not the shell speaking, so it is located as a
// function is and the builtin behind it names itself. Without this the rule
// would be "any function", which is a different and wrong one.
func TestAScriptsFunctionIsNotTheShellSpeaking(t *testing.T) {
	got := preludeRun(t, t.TempDir(), "", "stack() {\n\tcd /no/such/dir-xyz\n}\nstack\n",
		Diagnostics{Location: LocationLineWord, CdCannotChange: "cd: %[1]s: %[2]s"})
	oneLine(t, got, "testsh: line 2: cd: /no/such/dir-xyz: No such file or directory")
}

// And redefining one takes the voice away with the name, because what is
// remembered is the declaration rather than the name.
func TestRedefiningAPreludeFunctionTakesTheVoiceAway(t *testing.T) {
	const prelude = "stack() {\n\tdiagnose \"nothing to pop\"\n}\n"
	got := preludeRun(t, t.TempDir(), prelude, "stack() {\n\tcd /no/such/dir-xyz\n}\nstack\n",
		Diagnostics{Location: LocationLineWord, CdCannotChange: "cd: %[1]s: %[2]s"})
	oneLine(t, got, "testsh: line 2: cd: /no/such/dir-xyz: No such file or directory")
	if strings.Contains(got, "stack:") {
		t.Errorf("stderr = %q, the redefined function still speaks for the shell", got)
	}
}

// The seam is the prelude's and not the language's: a script running the word
// finds no such command, so a dialect gains no builtin by having a prelude.
func TestTheSeamIsNotACommandAScriptCanRun(t *testing.T) {
	dg := Diagnostics{Location: LocationLineWord}
	got := preludeRun(t, t.TempDir(), "stack() { :; }\n", "diagnose oops\n", dg)
	if !strings.Contains(got, "diagnose") || !strings.Contains(got, "not found") {
		t.Errorf("stderr = %q, want the word refused as a command that is not there", got)
	}
	// And the resolution agrees with the refusal, which is the half a
	// completer and `command -v` read.
	out, _ := run(t, "command -v diagnose; echo st=$?", nil)
	if out != "st=1\n" {
		t.Errorf("command -v diagnose = %q, want nothing found", out)
	}
}

// With no operands there is nothing to report, so nothing is written. Asserted
// because a seam that printed a bare `name: ` for this would be a diagnostic
// nobody wrote.
func TestTheSeamWithNothingToSayIsSilent(t *testing.T) {
	const prelude = "stack() {\n\tdiagnose\n\tdiagnose two words\n}\n"
	got := preludeRun(t, t.TempDir(), prelude, "stack\n",
		Diagnostics{Location: LocationLineWord})
	if got != "testsh: line 1: stack: two words\n" {
		t.Errorf("stderr = %q, want the empty call silent and the operands joined", got)
	}
}

// A dialect with a location style reserved for a builtin's own messages uses
// it here: a prelude function is a builtin speaking, so the composition is the
// one BuiltinLocation already describes rather than a fourth case.
func TestTheShellSpeakingIsLocatedTheWayABuiltinIs(t *testing.T) {
	const prelude = "stack() {\n\tdiagnose \"nothing to pop\"\n}\n"
	dg := Diagnostics{Location: LocationLineWord, BuiltinLocation: LocationBracketLine}
	got := preludeRun(t, t.TempDir(), prelude, "true\nstack\n", dg)
	oneLine(t, got, "testsh[2]: stack: nothing to pop")

	// And the script's own function does not borrow it, which is what makes
	// this a rule about who is speaking rather than about where the line is.
	other := preludeRun(t, t.TempDir(), "", "true\nnosuchcmd-xyz\n", dg)
	if strings.Contains(other, "[2]") {
		t.Errorf("stderr = %q, the builtin style leaked onto the shell's own message", other)
	}
}

// And "a builtin is speaking" holds for everything the function reports, not
// only for a message some inner builtin happened to write. Without that the
// location would depend on which command the prelude reached for, which is the
// implementation detail this seam exists to hide.
func TestTheShellSpeakingIsABuiltinWhateverRaisedTheMessage(t *testing.T) {
	const prelude = "stack() {\n\tnosuchcmd-xyz\n}\n"
	dg := Diagnostics{Location: LocationLineWord, BuiltinLocation: LocationBracketLine}
	got := preludeRun(t, t.TempDir(), prelude, "stack\n", dg)
	// A command that was not found is the shell's own failure and no builtin
	// wrote it, so this is the case the two answers differ on.
	oneLine(t, got, "testsh[1]: nosuchcmd-xyz: not found")
}

// A dialect that names the *function* a message came from names the builtin
// instead when the shell is speaking, because to the script there is no
// function there to name.
func TestTheShellSpeakingIsNeverLocatedAsAFunction(t *testing.T) {
	const prelude = "stack() {\n\tdiagnose \"nothing to pop\"\n}\n"
	got := preludeRun(t, t.TempDir(), prelude, "true\nstack\n", Diagnostics{
		Location: LocationTightLine, LocationNamesTheFunction: true,
		NamesBuiltinInLocation: true,
	})
	oneLine(t, got, "testsh:stack:2: nothing to pop")
}
