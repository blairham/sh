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
		// A **statement that is not a command** reads back whole. The trap
		// fires once for a pipeline and once for an `&&`/`||` list, and at
		// that firing the parameter is the statement — where each of these
		// used to leave the *previous* command's text standing, because a
		// pipeline and a list are syntax.Expr and the record held only a
		// syntax.Command.
		{`print a && print b`, "print a && print b"},
		{`print a || print b`, "print a || print b"},
		{`print c | cat`, "print c | cat"},
		{"{ print c } | cat", "{\n\tprint c\n} | cat"},
		// A negation is part of the statement's text. The single-element
		// rows are the ones that need the pipeline carried to them: nothing
		// dispatches a pipeline of one, so the `!` is in no node the
		// command-level record can see.
		{`! true`, "! true"},
		{"! { print x }", "! {\n\tprint x\n}"},
		{`! print a | cat`, "! print a | cat"},
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

// The commands **inside** a statement still record themselves, which is the
// half a single-string assertion cannot make.
//
// Without it the fix would be indistinguishable from one that pinned the
// statement's text for everything the statement contains: `{ print c } | cat`
// has to read back as the pipeline at one firing *and* as `print c` at
// another, and a test asserting only the first passes either way.
func TestZshDebugCmdStillNamesTheCommandsInsideAStatement(t *testing.T) {
	// The group is the **last** element deliberately, so its own stdout is
	// the pipeline's and the inner firing's output is readable. With the
	// group first, that output goes into the pipe and the row cannot see it —
	// which is a property of the probe and not of the shell. Both shapes were
	// measured; this one is the observable half.
	out, _ := runZsh(t, t.TempDir(),
		"trap 'print -r -- \"C=[$ZSH_DEBUG_CMD]\"' DEBUG\n"+
			"print a | { print c }\ntrap - DEBUG\n")
	for _, want := range []string{
		"C=[print a | {\n\tprint c\n}]", // the pipeline's own firing
		"C=[print c]",                   // the command inside the group
	} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q\n  want %q in it", out, want)
		}
	}
}

// A command **inside** a negated element records itself, and not the negation.
//
// This is the row a surviving mutant asked for. The negated pipeline of one
// is carried across its element's dispatch, so the carry has to be pinned to
// that one command by identity: dropping the identity comparison and letting
// it apply to whatever is running next passed every other row here, because
// none of them looked inside a negated body.
func TestZshDebugCmdInsideANegatedGroupNamesTheCommand(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"trap 'print -r -- \"C=[$ZSH_DEBUG_CMD]\"' DEBUG\n"+
			"! { print x; print y }\ntrap - DEBUG\n")
	for _, want := range []string{
		"C=[! {\n\tprint x\n\tprint y\n}]", // the negated statement
		"C=[print x]",                      // and each command in it, undecorated
		"C=[print y]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q\n  want %q in it", out, want)
		}
	}
	// Two commands rather than one, so that a carry leaking onto "whatever
	// runs next" is caught wherever it lands rather than only at the first.
	if n := strings.Count(out, "C=[! {"); n != 1 {
		t.Errorf("the negation was named %d times, want once, in %q", n, out)
	}
}

// A statement's firing leaves the record naming **no command**, which is the
// claim the two fields make together: a reader asking for a command at that
// firing is told there is not one, because `print a && print b` is not a
// command.
//
// Asserted through the parameter rather than the field, since that is the
// only view a script has: the text is the list's and not its first operand's,
// so an implementation filling the command field with `print a` would be
// caught here.
func TestZshDebugCmdAtAListIsNotTheFirstOperand(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"trap 'print -r -- \"C=[$ZSH_DEBUG_CMD]\"' DEBUG\n"+
			"print a && print b\ntrap - DEBUG\n")
	first := strings.Index(out, "C=[")
	if first < 0 {
		t.Fatalf("no firing at all in %q", out)
	}
	if got := out[first:]; !strings.HasPrefix(got, "C=[print a && print b]") {
		t.Errorf("the list's own firing named %q, want the whole list", got)
	}
}
