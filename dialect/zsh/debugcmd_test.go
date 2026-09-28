// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `ZSH_DEBUG_CMD` holds the command a DEBUG action fired for, written back
// out in **this shell's function layout** — a compound reads over several
// lines, indented with tabs, the way `functions` would say it.
//
// That is the thing that makes it different from bash's `BASH_COMMAND`, which
// prints a compound's *head* on one line. Measured 2026-09-28 against
// /opt/homebrew/bin/zsh — zsh 5.9.2 (aarch64-apple-darwin25.4.0), `go version
// -m`: *not a Go executable* — reading the parameter through `print -r --`
// so nothing is re-split (#5063).
func TestZshDebugCmdHoldsTheCommandAboutToRun(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		// Simple commands, with the whitespace normalized and the quoting
		// kept as written.
		{`echo    one`, "echo one"},
		{`x=1 echo $x "a  b"`, `x=1 echo $x "a  b"`},
		{`echo  hi   >   /dev/null`, "echo hi > /dev/null"},
		{`print "a  b"`, `print "a  b"`},
		{`print 'q'`, `print 'q'`},
		{`[[ 1 ==  1 ]]`, "[[ 1 == 1 ]]"},
		// The two arithmetic spellings keep their own text.
		{`((1+1))`, "((1+1))"},
		{`(( 1+1 ))`, "(( 1+1 ))"},
		// A compound reads over several lines — the half bash does not do.
		{"f() { :; }", "f () {\n\t:\n}"},
		{"{ : ; }", "{\n\t:\n}"},
		{"( : )", "(\n\t:\n)"},
		{"for  w  in  a   b; do :; done", "for w in a b\ndo\n\t:\ndone"},
		{"while false; do :; done", "while false\ndo\n\t:\ndone"},
		{"if true; then :; fi", "if true\nthen\n\t:\nfi"},
		{"repeat 1; do :; done", "repeat 1\ndo\n\t:\ndone"},
		{"case    x    in\n  x) :;;\nesac", "case x in\n\t(x) : ;;\nesac"},
	} {
		src := "trap 'print -r -- \"C=[$ZSH_DEBUG_CMD]\"' DEBUG\n" + tc.body + "\ntrap - DEBUG\n"
		out, _ := runZsh(t, t.TempDir(), src)
		if !strings.Contains(out, "C=["+tc.want+"]") {
			t.Errorf("%q\n  got  %q\n  want C=[%s] in it", tc.body, out, tc.want)
		}
	}
}

// TestZshDebugCmdIsSetBeforeEachCommand: it fires for the command that
// **removes** the trap as well, which is what says the parameter is set ahead
// of each command rather than after the one that has just run.
func TestZshDebugCmdIsSetBeforeEachCommand(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"trap 'print -r -- \"C=[$ZSH_DEBUG_CMD]\"' DEBUG\nprint A\ntrap - DEBUG\n")
	if !strings.Contains(out, "C=[print A]") {
		t.Errorf("= %q, want the command ahead of it", out)
	}
	if !strings.Contains(out, "C=[trap - DEBUG]") {
		t.Errorf("= %q, want the trap's own removal to be the last value read", out)
	}
}

// TestZshDebugCmdIsEmptyBeforeAnythingRuns, and an assignment to it goes
// nowhere — the next command records over it before anything reads it, and a
// stored value would answer ahead of the producer and stop it tracking.
func TestZshDebugCmdIsEmptyBeforeAnythingRunsAndTakesNoAssignment(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `print -r -- "C=[$ZSH_DEBUG_CMD]"`)
	if strings.TrimSpace(out) != "C=[]" {
		t.Errorf("before anything ran = %q, want C=[]", out)
	}
	out, _ = runZsh(t, t.TempDir(),
		"ZSH_DEBUG_CMD=zzz\ntrap 'print -r -- \"C=[$ZSH_DEBUG_CMD]\"' DEBUG\nprint A\ntrap - DEBUG\n")
	if strings.Contains(out, "zzz") {
		t.Errorf("after an assignment = %q, want the assignment to go nowhere", out)
	}
}
