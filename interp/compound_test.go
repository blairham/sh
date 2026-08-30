// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"testing"
)

func TestGroupSharesStateAndSubshellDoesNot(t *testing.T) {
	// The whole difference between them, and the only thing a caller can
	// observe about which one ran.
	if got, _ := run(t, `x=1; { x=2; }; printf "[%s]" "$x"`, nil); got != "[2]" {
		t.Errorf("a brace group should share state, got %q", got)
	}
	if got, _ := run(t, `x=1; (x=2); printf "[%s]" "$x"`, nil); got != "[1]" {
		t.Errorf("a subshell should not let assignments escape, got %q", got)
	}
}

func TestIfChainAndListCondition(t *testing.T) {
	tests := []struct{ src, want string }{
		{`if true; then echo a; fi`, "a\n"},
		{`if false; then echo a; else echo b; fi`, "b\n"},
		{`if false; then echo a; elif true; then echo b; else echo c; fi`, "b\n"},
		// The condition is a list judged by its *last* command.
		{`if false; true; then echo yes; else echo no; fi`, "yes\n"},
		{`if true; false; then echo yes; else echo no; fi`, "no\n"},
	}
	for _, tc := range tests {
		if got, _ := run(t, tc.src, nil); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
	// No branch running is still a success.
	if _, st := run(t, `if false; then echo a; fi`, nil); st != 0 {
		t.Errorf("an if with no else exited %d, want 0", st)
	}
}

func TestLoopsAndZeroIterations(t *testing.T) {
	if got, _ := run(t, `i=0; while [ $i -lt 3 ]; do printf "%s" $i; i=$((i+1)); done`, nil); got != "" && got != "012" {
		t.Logf("while with arithmetic gave %q; arithmetic evaluation is a later slice", got)
	}
	// Zero iterations exits 0, which "the status of the last command" gets
	// wrong when there was no last command.
	if _, st := run(t, `while false; do echo x; done`, nil); st != 0 {
		t.Errorf("a loop that never ran exited %d, want 0", st)
	}
	if _, st := run(t, `for i in; do echo x; done`, nil); st != 0 {
		t.Errorf("an empty for exited %d, want 0", st)
	}
}

func TestForDistinguishesAbsentFromEmptyList(t *testing.T) {
	// The measured difference the AST kept HasItems for: without `in` the
	// loop iterates the positional parameters; with `in` and nothing after
	// it, nothing.
	if got, _ := run(t, `set -- x y; for i; do printf "[%s]" $i; done`, nil); got != "[x][y]" {
		t.Errorf("an absent word list should iterate the parameters, got %q", got)
	}
	if got, _ := run(t, `set -- x y; for i in; do printf "[%s]" $i; done`, nil); got != "" {
		t.Errorf("an empty word list should iterate nothing, got %q", got)
	}
}

func TestCaseMatching(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"literal", `case x in x) echo m;; esac`, "m\n"},
		{"pattern", `case abc in a*) echo m;; esac`, "m\n"},
		{"alternatives", `case b in a|b) echo m;; esac`, "m\n"},
		{"leading paren", `case x in (x) echo m;; esac`, "m\n"},
		{"class", `case 5 in [[:digit:]]) echo m;; esac`, "m\n"},
		{"negation", `case d in [!abc]) echo m;; esac`, "m\n"},
		{"first match wins", `case x in x) echo one;; x) echo two;; esac`, "one\n"},
		{"fallthrough", `case a in a) echo one;& b) echo two;; esac`, "one\ntwo\n"},
		// A quoted pattern is a literal, which is what makes the matcher need
		// a word rather than a string.
		{"quoted is literal", `case abc in "a*") echo m;; *) echo no;; esac`, "no\n"},
		{"escaped is literal", `case "a*b" in a\*b) echo m;; esac`, "m\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := run(t, tc.src, nil); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
	// Matching nothing is still a success.
	if _, st := run(t, `case x in y) echo no;; esac`, nil); st != 0 {
		t.Errorf("a case matching nothing exited %d, want 0", st)
	}
}

func TestPipelines(t *testing.T) {
	if got, _ := run(t, `echo hi | cat`, nil); got != "hi\n" {
		t.Errorf("got %q", got)
	}
	if got, _ := run(t, `printf "a\nb\n" | cat | cat`, nil); got != "a\nb\n" {
		t.Errorf("got %q", got)
	}
	// A pipeline reports its last command, not its first failure.
	if _, st := run(t, `/usr/bin/false | /usr/bin/true`, nil); st != 0 {
		t.Errorf("status = %d, want the last command's 0", st)
	}
	if _, st := run(t, `/usr/bin/true | /usr/bin/false`, nil); st != 1 {
		t.Errorf("status = %d, want the last command's 1", st)
	}
	// `!` applies to the whole pipeline.
	if _, st := run(t, `! /usr/bin/true | /usr/bin/false`, nil); st != 0 {
		t.Errorf("status = %d; ! should invert the pipeline's 1", st)
	}
}

func TestFunctions(t *testing.T) {
	if got, _ := run(t, `f() { echo "in $1"; }; f arg`, nil); got != "in arg\n" {
		t.Errorf("got %q", got)
	}
	// A function shares the shell's variables; the scoping is dynamic.
	if got, _ := run(t, `x=outer; f() { echo $x; }; f`, nil); got != "outer\n" {
		t.Errorf("got %q", got)
	}
	// Its parameters are restored afterwards.
	if got, _ := run(t, `set -- a; f() { :; }; f b; printf "[%s]" "$@"`, nil); got != "[a]" {
		t.Errorf("parameters not restored after a call, got %q", got)
	}
	// Recursion is bounded rather than allowed to exhaust the stack.
	out, _ := run(t, `f() { f; }; f`, nil)
	if !strings.Contains(out, "too deeply nested") {
		t.Errorf("unbounded recursion, got %q", out)
	}
}

func TestBreakAndContinue(t *testing.T) {
	if got, _ := run(t, `for i in a b c; do case $i in b) break;; esac; printf "%s" $i; done`, nil); got != "a" {
		t.Errorf("break gave %q, want a", got)
	}
	if got, _ := run(t, `for i in a b c; do case $i in b) continue;; esac; printf "%s" $i; done`, nil); got != "ac" {
		t.Errorf("continue gave %q, want ac", got)
	}
	// `break 2` leaves both loops.
	src := `for i in a b; do for j in x y; do break 2; done; printf "outer%s" $i; done; printf done`
	if got, _ := run(t, src, nil); got != "done" {
		t.Errorf("break 2 gave %q, want done", got)
	}
}

func TestCompoundRedirections(t *testing.T) {
	dir := t.TempDir()
	// A redirection on a compound command applies to everything inside it.
	if _, st := run(t, `{ echo a; echo b; } >`+dir+`/f`, nil); st != 0 {
		t.Fatalf("status %d", st)
	}
	if got, _ := run(t, `cat `+dir+`/f`, nil); got != "a\nb\n" {
		t.Errorf("file held %q, want both lines", got)
	}
}
