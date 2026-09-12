// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// Two constructs this shell refused whole files for, where bash complains
// about the line and reads on. Neither is a construct we cannot read — both
// parse exactly as bash parses them — so what is measured here is **when** the
// complaint is raised and **how much** it ends. #2380.
//
// Measured 2026-09-12 against bash 5.3.15 and bash 3.2.57, from a script file,
// with `echo one` above the construct and `echo two` below it. Whole streams
// and the status, because every one of these once differed only in how much
// ran after the diagnostic, and a Contains assertion cannot see that at all.

// TestAnArrayLiteralSyntaxErrorEndsTheLineAndNotTheFile is the compound
// assignment half.
//
// `a=( … )` is one word: the parentheses belong to the assignment and what
// stands between them is a list of its own, so a `&` or a `>` there is a
// complaint about that list. bash throws the line away unrun — which the
// function row makes visible, since `f` is not defined — and carries on.
func TestAnArrayLiteralSyntaxErrorEndsTheLineAndNotTheFile(t *testing.T) {
	for _, c := range []struct {
		name, src, out string
		status         int
		quiet          bool
	}{
		{
			"an ampersand between the elements",
			"echo one\na=(p & q)\necho two\n", "one\ntwo\n", 0, false,
		},
		{
			// A redirection operator, reached through a subscripted element,
			// so the recovery is not one token's.
			"a redirection operator in an element",
			"echo one\na=( [0]=p [1]=> )\necho two\n", "one\ntwo\n", 0, false,
		},
		{"a pipe", "echo one\na=(p | q)\necho two\n", "one\ntwo\n", 0, false},
		{"a case terminator", "echo one\na=(p ;; q)\necho two\n", "one\ntwo\n", 0, false},
		{
			// The parenthesis an element opens is counted, so the array is
			// not called closed by the inner one and `echo two` is still
			// reached.
			"a parenthesis inside the array",
			"echo one\na=(p & (q) )\necho two\n", "one\ntwo\n", 0, false,
		},
		{
			// The assignment did not happen and `$?` is a failed command's,
			// not a refused file's.
			"the status and the variable after it",
			"echo one\na=(p & q)\necho \"st=$?\"\necho \"a=${a[@]-UNSET}\"\n",
			"one\nst=1\na=UNSET\n", 0, false,
		},
		{
			// The whole line goes, however much of it stands around the
			// assignment.
			"an enclosing if",
			"echo one\nif a=(p & q); then echo T; else echo F; fi\necho two\n",
			"one\ntwo\n", 0, false,
		},
		{
			// And the definition it was written in is not made.
			"a function body",
			"echo one\nf() { a=(p & q); }\necho defined\n", "one\ndefined\n", 0, false,
		},
		{
			"the appending spelling",
			"echo one\na+=(p & q)\necho two\n", "one\ntwo\n", 0, false,
		},
		{
			"as the operand of a declaration utility",
			"echo one\ndeclare -a d=(p & q)\necho two\n", "one\ntwo\n", 0, false,
		},
		{
			// Nothing after the bad line, so what is left is the status the
			// line left: 1, the number a failed command leaves.
			"nothing after it",
			"echo one\na=(p & q)\n", "one\n", 1, false,
		},
		{
			// `set -e` makes it the file's after all, at the parse-failure
			// status rather than at 1.
			"under set -e",
			"set -e\necho one\na=(p & q)\necho two\n", "one\n", 2, false,
		},
		// Controls. An array that reads is untouched, and the two failures
		// that are *not* this one still end the file: the input running out
		// is the ordinary unfinished construct, and a substitution's contents
		// are the file's.
		{"an array that reads", "echo one\na=(x y)\necho \"${a[@]}\"\n", "one\nx y\n", 0, true},
		{"the input running out", "echo one\na=(p q\necho two\n", "one\n", 2, false},
		{"a substitution that does not read", "echo one\necho $(if)\necho two\n", "one\n", 2, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, status := runScript(t, scriptFile(t, c.src))
			if out != c.out || status != c.status {
				t.Errorf("ran %q: out %q status %d, want %q and %d (errs %q)",
					c.src, out, status, c.out, c.status, errs)
			}
			if c.quiet && errs != "" {
				t.Errorf("ran %q: said %q, want silence", c.src, errs)
			}
			if !c.quiet && errs == "" {
				t.Errorf("ran %q: said nothing, want a complaint", c.src)
			}
		})
	}
}

// TestTheArrayLiteralRefusalNamesTheTokenItStoppedOn is the wording half, and
// it is separable from the one above: bash names the token where this shell
// named the parenthesis it never found.
func TestTheArrayLiteralRefusalNamesTheTokenItStoppedOn(t *testing.T) {
	src := "echo one\na=(p & q)\necho two\n"
	_, errs, _ := runScript(t, scriptFile(t, src))
	// Both lines, in order, and the second is the one that quotes the source
	// back — which is what makes the first line's blame checkable by eye.
	want := "line 2: syntax error near unexpected token `&'\n"
	if !strings.Contains(errs, want) {
		t.Errorf("said %q, want a line holding %q", errs, want)
	}
	if !strings.Contains(errs, "line 2: `a=(p & q)'\n") {
		t.Errorf("said %q, want the source line quoted back", errs)
	}
}

// TestAWholeFileReadIsStillRefused is the other side of the recovery: a read
// that runs nothing has no next line to go on to, so the refusal is the
// answer. That is what `bash -n` reports and exits non-zero for, and it is
// what keeps the corpus guard honest about a construct the reference shells
// also refuse.
func TestAWholeFileReadIsStillRefused(t *testing.T) {
	if _, err := syntax.Parse("echo one\na=(p & q)\necho two\n", bash.Dialect()); err == nil {
		t.Fatal("parsed the whole file, want the refusal a read that runs nothing hands back")
	}
	if _, err := syntax.Parse("echo one\na=(x y)\necho two\n", bash.Dialect()); err != nil {
		t.Fatalf("refused an array that reads: %v", err)
	}
}

// TestAQuotedExpansionOperandIsReadWhenTheExpansionReachesIt is the second
// construct.
//
// A `${ … }` inside double quotes is read twice — the scan for the closing
// brace honors the quotes inside the braces, and the operand is then read
// again with those quotes standing for themselves. That the second read finds
// an unterminated `$(` is not in dispute and is not new. When it happens is:
// bash reads the operand only where the expansion reaches it, so a branch
// nothing takes is never read at all.
func TestAQuotedExpansionOperandIsReadWhenTheExpansionReachesIt(t *testing.T) {
	for _, c := range []struct {
		name, src, out string
		status         int
		quiet          bool
	}{
		{
			// Never reached, so never read, so nothing is said.
			"a branch the expansion does not take",
			"echo one\nv=SET\necho \"${v-'$('}\"\necho two\n",
			"one\nSET\ntwo\n", 0, true,
		},
		{
			// Reached: a run-time complaint that gives up its line.
			"a branch the expansion takes",
			"echo one\necho \"${v-'$('}\"\necho two\n", "one\ntwo\n", 0, false,
		},
		{
			"the backquote spelling",
			"echo one\necho \"${v-'`'}\"\necho two\n", "one\ntwo\n", 0, false,
		},
		{
			"the brace spelling",
			"echo one\necho \"${v-'${'}\"\necho two\n", "one\ntwo\n", 0, false,
		},
		{
			// A definition holding one is made, in silence.
			"a function body",
			"echo one\nf() { echo \"${v-'$('}\"; }\necho defined\n",
			"one\ndefined\n", 0, true,
		},
		{
			// The rest of the *list* goes with it, because a failed
			// expansion ends the line and a list ends at a newline.
			"the rest of the line",
			"echo one\necho \"${v-'$('}\"; echo same-line\necho two\n",
			"one\ntwo\n", 0, false,
		},
		{
			// Contained by a substitution: the outer command still runs.
			"inside a command substitution",
			"echo one\nx=$(echo \"${v-'$('}\")\necho \"x=[$x] st=$?\"\n",
			"one\nx=[] st=1\n", 0, false,
		},
		// Controls. The replacement operand is read as a word of its own in
		// this dialect, where a quote quotes, so there is no second read and
		// no complaint; and a pattern operand is the same. The last row is
		// the one that says this is about timing and not about the second
		// read: with no quotes to defer behind, the file is refused here
		// exactly as bash refuses it.
		{
			"a replacement operand",
			"echo one\nv=xay\necho \"${v/a/'$('}\"\necho two\n",
			"one\nx$(y\ntwo\n", 0, true,
		},
		{
			"a pattern operand",
			"echo one\nv=xay\necho \"${v#'$('}\"\necho two\n",
			"one\nxay\ntwo\n", 0, true,
		},
		{
			"an operand with no quotes to defer behind",
			"echo one\necho \"${v+'bar}\"\necho two\n", "one\n", 2, false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, status := runScript(t, scriptFile(t, c.src))
			if out != c.out || status != c.status {
				t.Errorf("ran %q: out %q status %d, want %q and %d (errs %q)",
					c.src, out, status, c.out, c.status, errs)
			}
			if c.quiet && errs != "" {
				t.Errorf("ran %q: said %q, want silence", c.src, errs)
			}
			if !c.quiet && errs == "" {
				t.Errorf("ran %q: said nothing, want a complaint", c.src)
			}
		})
	}
}
