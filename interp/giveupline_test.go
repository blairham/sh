// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A give-up gives up the **input line the shell is running**, not the line
// the failing command was written on. The two are the same number only when
// the failure is at the top level, which is where every one of the ten
// give-up sites was measured — so a give-up inside a function body took the
// line inside the body and left the caller's line running (#3503).
//
// Every row here needs a *pair* of lines with a second command on the
// caller's own line, because on one line "gave up this line" and "ended the
// script" print the same nothing, and with nothing after the call "gave up
// the callee's line" and "gave up the caller's" do too. So the shape is
// always: the give-up somewhere inside, `; echo "same=$?"` behind the
// **call**, and `echo "next=$?"` on the line after it.

// giveUpLineSemantics answers the axes every row here needs, so that a row is
// asserting on how much was given up rather than on an axis nobody answered.
func giveUpLineSemantics(r *Runner) {
	sem := *r.Semantics
	// The give-up answer for a failed expansion, for an element assignment
	// and for a reassignment to a readonly name — the three routes the rows
	// below reach, each of which has a column that ends the script instead.
	sem.FailedExpansionAbandonsTheLine = Yes
	sem.ReadonlyReassignmentFatal = No
	// And for a subscript handed to `unset`, which is a policy rather than
	// an Answer.
	sem.BadSubscriptToUnset = BadSubscriptAbandonsTheCommand
	// The status a give-up leaves, so `next=1` is a number this test may
	// assert rather than the core's refusal.
	sem.FatalErrorStatusIsOne = Yes
	r.Semantics = &sem
}

// giveUpRoutes are the ways a script reaches controlAbandon, as the statement
// that does it. They share one field and one door, so a fix that reached only
// one of them is what this list is against.
var giveUpRoutes = []struct{ name, stmt string }{
	{"a subscript handed to unset", `unset "a[1+]"`},
	{"a failed expansion", `echo "$((1/0))"`},
	{"an element assignment the shell refused", `a[1+]=v`},
	{"a reassignment to a readonly name", `r=2`},
}

// giveUpProgram builds a program whose give-up happens inside `inside` and
// whose caller's line carries a second command.
func giveUpProgram(inside string) string {
	return "a=(x y z)\nreadonly r=1\n" + inside + "\necho \"next=$?\"\necho end\n"
}

// The row the issue is about: the give-up is inside a function body and the
// line that is given up is the one the **call** was written on.
func TestAGiveUpInsideAFunctionGivesUpTheCallersLine(t *testing.T) {
	for _, route := range giveUpRoutes {
		t.Run(route.name, func(t *testing.T) {
			src := giveUpProgram("f() { " + route.stmt + `; echo inside; }` + "\n" + `f; echo "same=$?"`)
			out, st := runGrammar(t, src, nil, giveUpLineSemantics)
			if strings.Contains(out, "inside") {
				t.Errorf("output %q ran the rest of the function body", out)
			}
			if strings.Contains(out, "same=") {
				t.Errorf("output %q ran the rest of the *caller's* line, which is the line given up", out)
			}
			if !strings.Contains(out, "next=1") {
				t.Errorf("output %q: want the next line to run with the give-up's 1 behind it", out)
			}
			if !strings.Contains(out, "end") || st != 0 {
				t.Errorf("output %q status %d: want the shell to carry on to the end", out, st)
			}
		})
	}
}

// And through every shape that can stand between the input line and the
// give-up without being a statement loop of its own. A construct that
// reported the *callee's* line here would leave the caller's line running,
// and a construct that swallowed the give-up would run `inside` as well.
func TestAGiveUpReachesTheInputLineThroughEveryEnclosingShape(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{
			"a function called from an if",
			`f() { unset "a[1+]"; echo inside; }` + "\n" + `if true; then f; echo inif; fi; echo "same=$?"`,
		},
		{
			"a function called from a loop",
			`f() { unset "a[1+]"; echo inside; }` + "\n" + `for i in 1; do f; echo inloop; done; echo "same=$?"`,
		},
		{
			"a function called from a group",
			`f() { unset "a[1+]"; echo inside; }` + "\n" + `{ f; echo ingroup; }; echo "same=$?"`,
		},
		{
			"a function called from another function",
			`g() { unset "a[1+]"; echo ing; }` + "\n" + `f() { g; echo inf; }` + "\n" + `f; echo "same=$?"`,
		},
		{
			"a function called after a command on the same line",
			`f() { unset "a[1+]"; echo inside; }` + "\n" + `echo pre; f; echo "same=$?"`,
		},
		{
			"a function whose body spans several lines",
			"f() {\n  unset \"a[1+]\"\n  echo inside\n}\n" + `f; echo "same=$?"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, giveUpProgram(tc.src), nil, giveUpLineSemantics)
			for _, absent := range []string{"inside", "ing", "inf", "inif", "inloop", "ingroup", "same="} {
				if strings.Contains(out, absent) {
					t.Errorf("output %q ran %q, which the give-up takes with it", out, absent)
				}
			}
			if !strings.Contains(out, "next=1") || !strings.Contains(out, "end") || st != 0 {
				t.Errorf("output %q status %d: want the next line to run at 1 and the shell to finish", out, st)
			}
		})
	}
}

// A subshell is the row that does **not** move, and it is the control that
// says the input line is a property of the shell reading input rather than of
// the statement that failed: the parentheses run in a copy of the runner, so
// the give-up ends the copy and the line the call was on carries on.
func TestAGiveUpInsideASubshellEndsTheSubshellAndNothingElse(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a give-up written in the parentheses", `( unset "a[1+]"; echo insub ); echo "same=$?"`},
		{
			"a function called from the parentheses",
			`f() { unset "a[1+]"; echo inside; }` + "\n" + `( f; echo insub ); echo "same=$?"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, giveUpProgram(tc.src), nil, giveUpLineSemantics)
			if strings.Contains(out, "insub") || strings.Contains(out, "inside") {
				t.Errorf("output %q ran on inside the subshell", out)
			}
			if !strings.Contains(out, "same=1") {
				t.Errorf("output %q: want the rest of the caller's line to run with the subshell's 1", out)
			}
			if !strings.Contains(out, "next=0") || st != 0 {
				t.Errorf("output %q status %d: want the next line to read the echo's 0", out, st)
			}
		})
	}
}

// The line is the input line and not the statement's first line, which is two
// facts: a compound's `;` is on its **last** line, and a backslash-continued
// command's `;` is on a line after its words end. Both of them are still one
// input line, so the `echo` behind the `;` goes with the give-up.
func TestAGiveUpTakesTheRestOfTheInputLineAndNotOfTheFirstLine(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{
			"a compound whose last line carries the next command",
			"if true; then\n  unset \"a[1+]\"\n  echo inside\nfi; echo \"same=$?\"",
		},
		{
			"a command continued with a backslash",
			"unset \"a[1+]\" \\\n  ; echo \"same=$?\"",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, giveUpProgram(tc.src), nil, giveUpLineSemantics)
			if strings.Contains(out, "inside") || strings.Contains(out, "same=") {
				t.Errorf("output %q ran the rest of the input line", out)
			}
			if !strings.Contains(out, "next=1") || !strings.Contains(out, "end") || st != 0 {
				t.Errorf("output %q status %d: want the line after it to run at 1", out, st)
			}
		})
	}
}

// Borrowed text is the other statement loop that reads a shell's input, so it
// is a file as far as giving up a line goes — and it has to put the caller's
// line back when it is done, or a give-up in the function that called the
// `eval` would give up a line of text that has stopped running.
func TestBorrowedTextGivesUpItsOwnLineAndRestoresTheCallers(t *testing.T) {
	// Inside the text: the first of its two lines is given up and the second
	// runs, so the `eval` itself succeeds and the caller's line carries on.
	src := giveUpProgram("f() { eval 'r=2\necho ineval'; echo inf; }\n" + `f; echo "same=$?"`)
	out, st := runGrammar(t, src, nil, giveUpLineSemantics)
	if !strings.Contains(out, "ineval") || !strings.Contains(out, "inf") {
		t.Errorf("output %q: want the give-up caught inside the borrowed text", out)
	}
	if !strings.Contains(out, "same=0") || !strings.Contains(out, "next=0") || st != 0 {
		t.Errorf("output %q status %d: want the caller's line to carry on", out, st)
	}

	// And after the text: a give-up later in the same function body is the
	// **caller's** line again and not the line the `eval` was running. The
	// `eval` is one line of text so that the line it would wrongly leave
	// behind — 1 — is not the caller's line either.
	src = giveUpProgram("f() { eval 'echo ineval'; unset \"a[1+]\"; echo inf; }\n" + `f; echo "same=$?"`)
	out, st = runGrammar(t, src, nil, giveUpLineSemantics)
	if !strings.Contains(out, "ineval") {
		t.Errorf("output %q: want the borrowed text to have run", out)
	}
	if strings.Contains(out, "inf") || strings.Contains(out, "same=") {
		t.Errorf("output %q ran on past the give-up", out)
	}
	if !strings.Contains(out, "next=1") || !strings.Contains(out, "end") || st != 0 {
		t.Errorf("output %q status %d: want the caller's next line to run at 1", out, st)
	}
}

// The complaint's location is **not** the line that is given up, which is why
// the fix could not simply move r.line: a failure two lines inside a function
// body is still reported against its own line while the caller's line is what
// stops running.
func TestAGiveUpReportsTheFailingLineAndGivesUpTheInputLine(t *testing.T) {
	src := giveUpProgram("f() {\n  echo before\n  unset \"a[1+]\"\n}\n" + `f; echo "same=$?"`)
	out, _ := runGrammar(t, src, nil, func(r *Runner) {
		giveUpLineSemantics(r)
		r.Diagnostics = &Diagnostics{Location: LocationLineWord}
	})
	if !strings.Contains(out, "line 5:") {
		t.Errorf("output %q: want the complaint against line 5, where the failure is written", out)
	}
	if strings.Contains(out, "same=") {
		t.Errorf("output %q: want line 7, the caller's, given up", out)
	}
}

// A give-up over an unevaluable **subscript** gives a `-c` string up whole
// rather than resuming at its next command, and the same arithmetic outside
// brackets does not. #3494 measured that rule and wrote it at the two
// builtin-operand sites; the sites that *expand* a subscript never got it, so
// `unset 'a[1+]'` and `a[1+]=v` — the same expression, one builtin apart —
// disagreed here and agree in the panel (#3502).
func TestABadSubscriptGivesACommandStringUpWholeAndABareExpressionDoesNot(t *testing.T) {
	for _, tc := range []struct {
		name, stmt string
		wholeText  bool
	}{
		{"an element assignment", `a[1+]=v`, true},
		{"an element appended to", `a[1+]+=v`, true},
		{"an element read", `echo "[${a[1+]}]"`, true},
		{"the length of an element", `echo "[${#a[1+]}]"`, true},
		{"a subscript inside an arithmetic expansion", `: $(( a[1+] ))`, true},
		{"a subscripted element of an array literal", `b=([1+]=v)`, true},
		{"a subscript handed to unset", `unset "a[1+]"`, true},
		// The bare spellings of the same failure, which is what makes this a
		// rule about brackets rather than about failed expansions.
		{"a division by zero in a word", `echo "$((1/0))"`, false},
		{"an expression the parser refused, in a word", `echo "$((1+))"`, false},
		{"a substring offset", `echo "[${s:1+:2}]"`, false},
		{"a reassignment to a readonly name", `r=2`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "a=(x y z)\nreadonly r=1\ns=abcdef\n" + tc.stmt + "\necho \"next=$?\"\necho end\n"
			// From a script file every row resumes at the next line, which is
			// the half that must not move.
			out, st := runGrammar(t, src, nil, giveUpLineSemantics)
			if !strings.Contains(out, "next=1") || !strings.Contains(out, "end") || st != 0 {
				t.Errorf("script file: output %q status %d, want the next line to run at 1", out, st)
			}

			out, st = runGrammar(t, src, nil, func(r *Runner) {
				giveUpLineSemantics(r)
				r.Route = RouteCommandString
			})
			if tc.wholeText {
				if strings.Contains(out, "next=") || strings.Contains(out, "end") {
					t.Errorf("command string: output %q, want the whole string given up", out)
				}
				if st != 1 {
					t.Errorf("command string: status %d, want the fatal 1", st)
				}
				return
			}
			if !strings.Contains(out, "next=1") || !strings.Contains(out, "end") {
				t.Errorf("command string: output %q, want the next command to run", out)
			}
			if st != 0 {
				t.Errorf("command string: status %d, want the last command's 0", st)
			}
		})
	}
}
