// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// One dialect names the place two ways in the one script: a bracketed line
// when a builtin is speaking and the word form for everything else. These
// name the axis rather than the shell.

// TestABuiltinCanNameThePlaceItsOwnWay is the axis in both directions, and
// the assertions are two-sided because the interesting failure is the style
// leaking onto the messages a builtin is *not* speaking.
func TestABuiltinCanNameThePlaceItsOwnWay(t *testing.T) {
	dg := Diagnostics{
		Location:        LocationLineWord,
		BuiltinLocation: LocationBracketLine,
	}
	dir := t.TempDir()

	spoken := prefixRun(t, dir, "cd /no/such/dir-xyz", dg)
	if !strings.HasPrefix(spoken, "testsh[1]: ") {
		t.Errorf("a builtin speaking = %q, want the bracketed line", spoken)
	}

	// A command that could not be found is the shell's own failure, so it
	// keeps the other style. Without this the field would be "any message
	// from a line a builtin happens to be on", which is a different rule.
	other := prefixRun(t, dir, "nosuchcmd-xyz", dg)
	if !strings.HasPrefix(other, "testsh: line 1: ") {
		t.Errorf("the shell speaking = %q, want the word form", other)
	}
	if strings.Contains(other, "[1]") {
		t.Errorf("the shell speaking = %q, bracketed style leaked", other)
	}
}

// TestOneStyleIsTheDefault covers the zero value, which is what three of the
// four dialects want: a builtin names the place the same way as everything
// else, and the second field is simply not set.
func TestOneStyleIsTheDefault(t *testing.T) {
	dg := Diagnostics{Location: LocationLineWord}
	out := prefixRun(t, t.TempDir(), "cd /no/such/dir-xyz", dg)
	if !strings.HasPrefix(out, "testsh: line 1: ") {
		t.Errorf("output = %q, want the one style for both", out)
	}
	if strings.Contains(out, "[1]") {
		t.Errorf("output = %q, unset BuiltinLocation still bracketed", out)
	}
}

// TestTheBracketedLineIsAlsoLeftOutOnTheFirstLine is the invocation-dependent
// half: under `-c` the dialect names no line on line 1, in either of its two
// styles, so the bracketed one needs its own "after the first" spelling.
func TestTheBracketedLineIsAlsoLeftOutOnTheFirstLine(t *testing.T) {
	dg := Diagnostics{
		Location:        LocationLineWordAfterFirst,
		BuiltinLocation: LocationBracketLineAfterFirst,
	}
	dir := t.TempDir()

	first := prefixRun(t, dir, "cd /no/such/dir-xyz", dg)
	if !strings.HasPrefix(first, "testsh: ") {
		t.Errorf("line 1 = %q, want no line named", first)
	}
	if strings.Contains(first, "[1]") {
		t.Errorf("line 1 = %q, want no line named", first)
	}

	later := prefixRun(t, dir, "true\ncd /no/such/dir-xyz", dg)
	if !strings.HasPrefix(later, "testsh[2]: ") {
		t.Errorf("line 2 = %q, want the bracketed line", later)
	}
}

// TestAScriptCanNameThePlaceDifferentlyAgain is ScriptBuiltinLocation, which
// exists for the same reason ScriptLocation does: the dialect that leaves the
// line out on line 1 leaves it out only under `-c`, and a script file names
// line 1 like any other — in both of its styles, which is why resolving one
// of them was not enough.
func TestAScriptCanNameThePlaceDifferentlyAgain(t *testing.T) {
	dg := Diagnostics{
		Location:              LocationLineWordAfterFirst,
		BuiltinLocation:       LocationBracketLineAfterFirst,
		ScriptLocation:        LocationLineWord,
		ScriptBuiltinLocation: LocationBracketLine,
	}
	dir := t.TempDir()

	// Before: line 1 names no line.
	if out := prefixRun(t, dir, "cd /no/such/dir-xyz", dg); strings.Contains(out, "[1]") {
		t.Errorf("as invoked = %q, want no line named", out)
	}

	script := dg.ForScript()
	after := prefixRun(t, dir, "cd /no/such/dir-xyz", script)
	if !strings.HasPrefix(after, "testsh[1]: ") {
		t.Errorf("as a script = %q, want line 1 bracketed", after)
	}
	// And the other style was resolved too, rather than one of the two.
	other := prefixRun(t, dir, "nosuchcmd-xyz", script)
	if !strings.HasPrefix(other, "testsh: line 1: ") {
		t.Errorf("as a script = %q, want line 1 in the word form", other)
	}
}

// TestARedirectionOpenedForABuiltinCountsAsTheBuiltins is the half that
// distinguishes the two dialects asking about builtins, and it is why the
// question is "is a builtin speaking" rather than "is the builtin named".
func TestARedirectionOpenedForABuiltinCountsAsTheBuiltins(t *testing.T) {
	dg := Diagnostics{
		Location:        LocationLineWord,
		BuiltinLocation: LocationBracketLine,
	}
	dir := t.TempDir()

	forBuiltin := prefixRun(t, dir, "echo hi > /nonexistent-dir-xyz/x", dg)
	if !strings.HasPrefix(forBuiltin, "testsh[1]: ") {
		t.Errorf("a builtin's redirection = %q, want the bracketed line", forBuiltin)
	}

	// An external command's redirection is not, which is the measurement
	// that ruled out "any redirection" as the rule.
	external := prefixRun(t, dir, "/bin/echo hi > /nonexistent-dir-xyz/x", dg)
	if !strings.HasPrefix(external, "testsh: line 1: ") {
		t.Errorf("an external command's redirection = %q, want the word form", external)
	}
	if strings.Contains(external, "[1]") {
		t.Errorf("an external command's redirection = %q, bracketed style leaked", external)
	}
}

// TestTheBuiltinIsStillNotNamedForItsRedirection is the other dialect's half
// of the same message, and the reason the two are separate questions: the
// dialect that writes the builtin's name into the location leaves it out
// here, so answering "the builtin is speaking" by setting the name would
// have been wrong for it.
func TestTheBuiltinIsStillNotNamedForItsRedirection(t *testing.T) {
	dg := Diagnostics{Location: LocationTightLine, NamesBuiltinInLocation: true}
	out := prefixRun(t, t.TempDir(), "echo hi > /nonexistent-dir-xyz/x", dg)
	if !strings.HasPrefix(out, "testsh:1:") {
		t.Errorf("output = %q, want the plain location", out)
	}
	if strings.Contains(out, ":echo:") {
		t.Errorf("output = %q, want the builtin not named", out)
	}
}

// TestTheMarkerIsPutBackAfterTheRedirections, so the command that runs after
// one does not inherit it — the redirection is open by then and the builtin
// itself is on the record instead.
func TestTheMarkerIsPutBackAfterTheRedirections(t *testing.T) {
	dg := Diagnostics{
		Location:        LocationLineWord,
		BuiltinLocation: LocationBracketLine,
	}
	dir := t.TempDir()
	out := prefixRun(t, dir, "echo hi > /dev/null\nnosuchcmd-xyz", dg)
	if !strings.Contains(out, "testsh: line 2: ") {
		t.Errorf("output = %q, want the next command to name the place its own way", out)
	}
	if strings.Contains(out, "[2]") {
		t.Errorf("output = %q, the marker outlived the redirections", out)
	}
}
