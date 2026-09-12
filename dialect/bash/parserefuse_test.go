// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
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
		{
			// Measured against the 2 this row used to assert, which was our
			// own answer rather than bash's: the input running out ends the
			// *line* as well, so what is left is the 1 a refused line leaves.
			// There is no next line to carry on to — that is what the end of
			// the input means — so the status is the whole of the difference.
			// See TestTheInputRunningOutInsideAnArrayLiteral (#2404).
			"the input running out", "echo one\na=(p q\necho two\n", "one\n", 1, false,
		},
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

// TestEveryRouteThatReadsGivesUpTheSameLine covers the four readers this shell
// has, because the shell answers all four the same way and a rule that reached
// only the top level would look right in most tests.
//
// The three below the top level each have a reader of their own —
// `runSourced`'s by-line loop, its whole-text branch, and the trap body's —
// and each of them would otherwise have run the line's *other* statements in
// silence, which is the plausible wrong answer at status 0 this codebase minds
// most. Measured against bash 5.3.15 on 2026-09-12.
func TestEveryRouteThatReadsGivesUpTheSameLine(t *testing.T) {
	dir := t.TempDir()
	inc := filepath.Join(dir, "inc.sh")
	if err := os.WriteFile(inc, []byte("echo in-one\na=(p & q)\necho in-two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	only := filepath.Join(dir, "only.sh")
	if err := os.WriteFile(only, []byte("a=(p & q)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, src, out string
	}{
		{
			"a sourced file read a line at a time",
			"echo one\n. " + inc + "\necho \"st=$?\"\necho two\n",
			"one\nin-one\nin-two\nst=0\ntwo\n",
		},
		{
			// One line, so there is no later line for the by-line reader to
			// be about and the whole-text branch takes it. `$?` is 1 there —
			// a failed command's status, not the syntax-error status this
			// builtin reports for text it could not read at all.
			"a sourced file of one line",
			"echo one\n. " + only + "\necho \"st=$?\"\n",
			"one\nst=1\n",
		},
		{
			"eval over more than one line",
			"echo one\neval \"a=(p & q)\necho in-two\"\necho \"st=$?\"\n",
			"one\nin-two\nst=0\n",
		},
		{
			"eval over one line",
			"echo one\neval \"a=(p & q)\"\necho \"st=$?\"\n",
			"one\nst=1\n",
		},
		{
			// The body is read as the shell reads any program, so the line
			// after the bad one still runs when the trap fires.
			"a trap body",
			"echo one\ntrap \"a=(p & q)\necho in-trap\" EXIT\necho two\n",
			"one\ntwo\nin-trap\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, status := runScript(t, scriptFile(t, c.src))
			if out != c.out {
				t.Errorf("ran %q: out %q, want %q (errs %q, status %d)",
					c.src, out, c.out, errs, status)
			}
			if !strings.Contains(errs, "syntax error near unexpected token `&'") {
				t.Errorf("ran %q: said %q, want the refusal named — a route that "+
					"swallows it runs the rest of the line in silence", c.src, errs)
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

// TestTheInputRunningOutInsideAnArrayLiteral is the status half of the same
// recovery, and it is the row above's other side.
//
// A syntax error between an array literal's parentheses ends the line rather
// than the file. The input *running out* between them was excluded from that
// on the reasoning that there is no next line to carry on to — which is true,
// and which leaves the status still to be answered. bash answers it the same
// way: 1, the number a refused line leaves, where the identical message
// outside the parentheses is 2.
//
// Measured 2026-09-12 against bash 5.3.15, `env -i` with a scratch HOME, from
// a script file and again with -c and with -n, all three agreeing:
//
//	a=( x                    1    the input ran out inside the parens
//	a=( $(                   1    inside a construct inside them
//	a=( "x                   1    inside a quote
//	echo $(                  2    the same message, outside them
//	echo "x                  2    the same
//	set -e ⏎ a=( x           2    the refused line's own rule
//	a=( $(if; then :; fi) )  2    a token refused deeper in is not this
//
// Row one against row four is the measurement: one wording, two statuses, and
// the parentheses the only difference between them. Row six is what says this
// is the refused *line* rather than a status of its own — `set -e` turns it
// back into 2 exactly as it does for a token refused between the parentheses,
// which a number attached to the failure could not have done.
func TestTheInputRunningOutInsideAnArrayLiteral(t *testing.T) {
	for _, c := range []struct {
		name, src, out string
		status         int
	}{
		{"between the parentheses", "echo one\na=( x", "one\n", 1},
		{
			// Inside a construct inside them, so the refusal is recorded
			// deeper in than this production and is still the line's.
			"inside a substitution inside them", "echo one\na=( $(", "one\n", 1,
		},
		{
			// And inside a quote, where the failure is the lexer's rather
			// than the parser's — a different field holding it, the same
			// answer.
			"inside a quote inside them", "echo one\na=( \"x", "one\n", 1,
		},
		{
			// The elements the parenthesis swallowed do not run: an
			// unterminated literal eats the rest of the file.
			"the lines it swallowed", "echo one\na=( x\necho two\n", "one\n", 1,
		},
		// Controls. The same message outside an array literal is the file's,
		// `set -e` makes this one the file's too, and a *token* the grammar
		// refused deeper in was never this.
		{"a substitution that runs out", "echo one\necho $(", "one\n", 2},
		{"a quote that runs out", "echo one\necho \"x", "one\n", 2},
		{"under set -e", "set -e\necho one\na=( x", "one\n", 2},
		{
			"a token refused deeper in", "echo one\na=( $(if; then :; fi) )\n",
			"one\n", 2,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, status := runScript(t, scriptFile(t, c.src))
			if out != c.out || status != c.status {
				t.Errorf("ran %q: out %q status %d, want %q and %d (errs %q)",
					c.src, out, status, c.out, c.status, errs)
			}
			if errs == "" {
				t.Errorf("ran %q: said nothing, want a complaint", c.src)
			}
		})
	}
}
