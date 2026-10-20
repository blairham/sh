// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A here-document **body** that failed is located at the line the whole
// command ended on here, for a compound command spelled with a reserved
// word — `{ … }`, `while`, `until`, `if`, `for`, `case`, `select`.
//
// This shell's counter is where the input has got to, and one of those
// constructs does not set it back, so a body that failed is reported past the
// command rather than at it. A wrong line in a diagnostic sends a reader to
// the wrong line of a template, and a here-document body in a script is often
// dozens of lines long — so the gap is not one line but the whole of the
// document.
//
// Measured 2026-09-26 with `-c` against /opt/homebrew/bin/bash 5.3.20 (`go
// version -m` says *not a Go executable*) and `cmd/bash` built under its own
// name. The command is on line 1, `$(( 1/0 ))` is its body on line 2, the
// delimiter is on line 3, and `echo done` is under it:
//
//	command                        bash   before
//	{ :; } <<END                      3        1
//	while read x; do :; done <<END    3        1
//	until :; do :; done <<END         3        1
//	if :; then :; fi <<END            3        1
//	for i in 1; do :; done <<END      3        1
//	case x in x) :;; esac <<END       3        1
//	select i in a; do break; done     3        1
//	( : ) <<END                       1        1
//	[[ x = x ]] <<END                 1        1
//	(( 1 )) <<END                     1        1
//	: <<END                           1        1
//	cat <<END                         1        1
//	f <<END                           2        2
//
// The last six are the controls, and they are why the rule is keyed on the
// **spelling**: a `( … )`, a `[[ … ]]` and an arithmetic command are all
// compound commands and all report the line they began on.
//
// See interp/heredoccommandend.go (#4690, #4712).
func TestAFailingHeredocBodyIsLocatedWhereTheCommandEndedHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, c := range []struct {
		command string
		want    string
	}{
		{"{ :; }", "line 3:"},
		{"while read x; do :; done", "line 3:"},
		{"until :; do :; done", "line 3:"},
		{"if :; then :; fi", "line 3:"},
		{"for i in 1; do :; done", "line 3:"},
		{"case x in x) :;; esac", "line 3:"},
		{"select i in a; do break; done", "line 3:"},
		// The controls.
		{"( : )", "line 1:"},
		{"[[ x = x ]]", "line 1:"},
		{"(( 1 ))", "line 1:"},
		{":", "line 1:"},
		{"cat", "line 1:"},
	} {
		out, _ := runBash(t, dir, c.command+" <<END\n$(( 1/0 ))\nEND\necho done\n")
		if !strings.Contains(out, c.want) {
			t.Errorf("%s said %q, want %q in it", c.command, out, c.want)
		}
		if !strings.Contains(out, "done") {
			t.Errorf("%s said %q, want the script carried on — only the line moves", c.command, out)
		}
	}
}

// The number is where the whole command ended and not the delimiter's line,
// which these four separate: the command's own text can run on past its
// here-document, and a second here-document closes below the first.
//
//	program                                            bash
//	{ :; } <<END |⏎ <body> ⏎ END ⏎ cat                    4
//	{ :; } <<A <<B ⏎ a1 ⏎ A ⏎ <body> ⏎ B                  5
//	if :; then ⏎ { :; } <<END ⏎ <body> ⏎ END ⏎ fi         5
//	for i in 1 ⏎ do : ⏎ done <<END ⏎ <body> ⏎ END         5
func TestTheLineIsWhereTheWholeCommandEndedHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, c := range []struct{ src, want string }{
		{"{ :; } <<END |\n$(( 1/0 ))\nEND\ncat\necho done\n", "line 4:"},
		{"{ :; } <<A <<B\na1\nA\n$(( 2/0 ))\nB\necho done\n", "line 5:"},
		{"if :; then\n{ :; } <<END\n$(( 1/0 ))\nEND\nfi\necho done\n", "line 5:"},
		{"for i in 1\ndo :\ndone <<END\n$(( 1/0 ))\nEND\necho done\n", "line 5:"},
	} {
		if out, _ := runBash(t, dir, c.src); !strings.Contains(out, c.want) {
			t.Errorf("%q said %q, want %q in it", c.src, out, c.want)
		}
	}
}

// Only the body moves. A redirection that could not be **opened** on the same
// group is reported at the redirect's own line here, which is what says this
// is not a rule about compound redirections at large.
func TestOnlyTheBodyTakesTheCommandsEndHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, _ := runBash(t, dir, "{ :; } < /nonexistent/x\necho done\n")
	if !strings.Contains(out, "line 1:") {
		t.Errorf("a failed open said %q, want the line the redirect is on", out)
	}
}

// And a refusal written **inside** the body moves with it: bash names line 5
// for a body on line 3 of a `{ f; } <<END` opened on line 2 — one past the
// delimiter, because the refusal counts from where the reader had got to and
// the body's first line is the next one. The same body on a command of its
// own is at the body's own line in both shells (#4712).
func TestARefusalInTheBodyMovesWithTheCommandsEndHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, c := range []struct{ src, want string }{
		{"f() { cat; }\n{ f; } <<END\n$(echo hi; for)\nEND\n", "line 5:"},
		{"while read x; do :; done <<END\n$(echo hi; for)\nEND\n", "line 4:"},
		// The control: no reserved word, so nothing moves.
		{"cat <<END\n$(echo hi; for)\nEND\n", "line 2:"},
	} {
		if out, _ := runBash(t, dir, c.src); !strings.Contains(out, c.want) {
			t.Errorf("%q said %q, want %q in it", c.src, out, c.want)
		}
	}
}
