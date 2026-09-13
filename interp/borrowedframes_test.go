// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// These name the axis and never the shell, per the rule for this package.
// Diagnostics.LocationRendersTheBorrowedStack is the axis: whether every `.`
// and `eval` the shell is inside stands in front of a diagnostic, or only the
// innermost text is named, or none of them. What the dialect that answers Yes
// writes is measured in dialect/ksh.

// framed is the diagnostics a chain is rendered under: the two bracket-and-
// word styles the axis composes with, and nothing else moved.
func framed(stack bool) Diagnostics {
	return Diagnostics{
		Location:                        LocationLineWordAfterFirst,
		BuiltinLocation:                 LocationBracketLineAfterFirst,
		LocationRendersTheBorrowedStack: stack,
		SourceFileIsTheBuiltin:          true,
		EvalNaming:                      SourceBeforeLocation,
		SourceFileNaming:                SourceBeforeLocation,
	}
}

// errLine is the first line of src's output that is not the script's own
// echo, which is where the located diagnostic is.
func errLine(t *testing.T, out string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "testsh") {
			return line
		}
	}
	t.Fatalf("no located diagnostic in %q", out)
	return ""
}

// TestTheBorrowedStackIsRenderedIntoALocation: with the axis on, a failure
// inside a sourced file carries the name of the text it is inside and the line
// the shell was on when it borrowed it.
func TestTheBorrowedStackIsRenderedIntoALocation(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "inc.sh", "nosuchcmd-xyz\n")
	out, _ := sourceRun(t, dir, "echo pad\n. "+dir+"/inc.sh\n", permissive(), framed(true))
	if got, want := errLine(t, out), "testsh[2]: .: line 1: "; !strings.HasPrefix(got, want) {
		t.Errorf("located %q, want a %q prefix", got, want)
	}
}

// TestWithoutTheAxisOnlyTheInnermostLineIsNamed is the same run with the axis
// off, which is what says the rendering is the axis's and not something the
// stack does on its own.
func TestWithoutTheAxisOnlyTheInnermostLineIsNamed(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "inc.sh", "nosuchcmd-xyz\n")
	out, _ := sourceRun(t, dir, "echo pad\n. "+dir+"/inc.sh\n", permissive(), framed(false))
	if got := errLine(t, out); strings.Contains(got, "[2]") || strings.Contains(got, ": .: ") {
		t.Errorf("located %q, want no frame in it", got)
	}
}

// TestEachFrameCarriesTheLineItEnteredTheNextFrom: a file sourced by a sourced
// file is two frames, and the number in each is read from that frame rather
// than from the failure.
func TestEachFrameCarriesTheLineItEnteredTheNextFrom(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "inner.sh", "nosuchcmd-xyz\n")
	write(t, dir, "mid.sh", "echo mid\necho mid\n. "+dir+"/inner.sh\n")
	out, _ := sourceRun(t, dir, "echo pad\n. "+dir+"/mid.sh\n", permissive(), framed(true))
	if got, want := errLine(t, out), "testsh[2]: .[3]: .: line 1: "; !strings.HasPrefix(got, want) {
		t.Errorf("located %q, want a %q prefix", got, want)
	}
}

// TestOnlyTheFirstFrameLeavesLineOneUnwritten is the discriminating pair. The
// AfterFirst styles are about the first line of what the shell was *given*, so
// a frame entered after that names its line however small it is — a rule that
// suppressed in every frame, or in none, passes one of these two and fails the
// other.
func TestOnlyTheFirstFrameLeavesLineOneUnwritten(t *testing.T) {
	dir := t.TempDir()
	out, _ := sourceRun(t, dir, `eval 'eval "nosuchcmd-xyz"'`, permissive(), framed(true))
	if got, want := errLine(t, out), "testsh: eval[1]: eval: line 1: "; !strings.HasPrefix(got, want) {
		t.Errorf("located %q, want a %q prefix", got, want)
	}
}

// TestABuiltinsComplaintEndsTheChainItsOwnWay: the last component is located
// the way an ordinary diagnostic is, so a builtin's complaint takes the
// builtin style there while a message the shell speaks takes the word style.
func TestABuiltinsComplaintEndsTheChainItsOwnWay(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "inc.sh", "echo one\ncd /nonexistent-xyz\n")
	out, _ := sourceRun(t, dir, "echo pad\n. "+dir+"/inc.sh\n", permissive(), framed(true))
	if got, want := errLine(t, out), "testsh[2]: .[2]: "; !strings.HasPrefix(got, want) {
		t.Errorf("located %q, want a %q prefix", got, want)
	}
}

// TestAFunctionAddsNothingToTheChain: the innermost borrowed text answers,
// with no test that the failing line came from it — the same rule
// Runner.borrowedAtLocation already holds for naming one.
func TestAFunctionAddsNothingToTheChain(t *testing.T) {
	dir := t.TempDir()
	out, _ := sourceRun(t, dir, "echo pad\neval 'f() { nosuchcmd-xyz; }; f'\n", permissive(), framed(true))
	if got, want := errLine(t, out), "testsh[2]: eval: line 1: "; !strings.HasPrefix(got, want) {
		t.Errorf("located %q, want a %q prefix", got, want)
	}
}

// TestNothingBorrowedRendersNoChain is the control the rest are read against:
// the axis changes nothing at the top level, where there is no frame.
func TestNothingBorrowedRendersNoChain(t *testing.T) {
	dir := t.TempDir()
	on, _ := sourceRun(t, dir, "echo pad\nnosuchcmd-xyz\n", permissive(), framed(true))
	off, _ := sourceRun(t, dir, "echo pad\nnosuchcmd-xyz\n", permissive(), framed(false))
	if errLine(t, on) != errLine(t, off) {
		t.Errorf("with the axis on %q, off %q: a location with no frame in it must not move",
			errLine(t, on), errLine(t, off))
	}
	if got, want := errLine(t, on), "testsh: line 2: "; !strings.HasPrefix(got, want) {
		t.Errorf("located %q, want a %q prefix", got, want)
	}
}
