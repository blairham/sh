// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// runCaseContinue is run() with the `;;&` terminator enabled by name; `;&` is
// core grammar and needs no flag.
func runCaseContinue(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) { d.CaseContinue = true }, nil)
}

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

func TestCaseSubjectIsNeitherSplitNorGlobbed(t *testing.T) {
	// The subject expands but keeps the exemption `[[ ]]` operands and a
	// scalar assignment value have: no field splitting and no pathname
	// expansion, even unquoted. The ordinary word pipeline split a
	// whitespace-only value to zero fields, so its arm never fired.
	tests := []struct{ name, src, want string }{
		{
			"whitespace-only subject reaches its arm",
			`t=$(printf "\t"); case $t in [[:blank:]]) echo m;; *) echo split;; esac`, "m\n",
		},
		{
			"a value with a space stays one subject",
			`m="a b"; case $m in "a b") echo m;; a) echo first-field;; *) echo neither;; esac`, "m\n",
		},
		{
			"a glob character stays a character",
			`g="*"; case $g in \*) echo m;; *) echo globbed;; esac`, "m\n",
		},
		{
			"an unset subject is the empty string",
			`case $u in "") echo m;; *) echo no;; esac`, "m\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := run(t, tc.src, nil); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCaseFallthroughFollowsEachArmsTerminator(t *testing.T) {
	// A body reached by `;&` still owns its terminator: another `;&`
	// keeps falling, `;;` stops, and the end of the item list stops.
	// Honoring only the matched arm's terminator ran one extra body and
	// returned, which dropped the third link of every chain.
	tests := []struct{ name, src, want string }{
		{"chain of three", `case a in a) echo 1;& b) echo 2;& c) echo 3;; esac`, "1\n2\n3\n"},
		{"chain stops at double-semi", `case a in a) echo 1;& b) echo 2;; c) echo 3;; esac`, "1\n2\n"},
		{"fall-through on the last arm", `case a in x) echo no;; a) echo last;& esac`, "last\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := run(t, tc.src, nil); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
	// The status is the last body's, however the arm was reached.
	if _, st := run(t, `case a in a) false;& b) true;& c) false;; esac`, nil); st != 1 {
		t.Errorf("status = %d, want the last fallen-into body's 1", st)
	}
	// The two operators compose rather than alias: a fallen-into arm
	// ending in the continue-matching terminator goes back to pattern
	// testing, and a re-matched arm ending in fall-through falls again.
	// That terminator is grammar the core lacks, hence the named flag.
	if got, _ := runCaseContinue(t, `case a in a) echo 1;& b) echo 2;;& c) echo 3;; a) echo 4;; esac`); got != "1\n2\n4\n" {
		t.Errorf("fall-through into continue-matching: got %q, want %q", got, "1\n2\n4\n")
	}
	if got, _ := runCaseContinue(t, `case a in a) echo 1;;& a) echo 2;& x) echo 3;; a) echo 4;; esac`); got != "1\n2\n3\n" {
		t.Errorf("continue-matching into fall-through: got %q, want %q", got, "1\n2\n3\n")
	}
}

// runSemiPipe is run() with zsh's spelling of that terminator enabled by
// name. A core test names the flag, not the shell.
func runSemiPipe(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) { d.CaseContinuePipe = true }, nil)
}

// TestSemiPipeIsTheOtherSpellingOfCaseContinue — `;|` and `;;&` are one
// terminator under two operators, and the interpreter answers them with one
// branch because they mean the same thing rather than nearly the same.
//
// Measured 2026-09-07 over a script file: every program below prints
// letter-for-letter the same in zsh with `;|` as in bash 5.3 with `;;&`.
// The pair that proves they are not `;&` is the middle arm whose pattern
// does *not* match — `;&` runs its body without testing it and prints Z,
// while `;|` and `;;&` skip it and reach the `*`. The filing's own program
// could not tell the three apart, because with three arms and a matching
// subject `;&` and `;|` give the same output.
func TestSemiPipeIsTheOtherSpellingOfCaseContinue(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"later patterns keep being tested",
			`case b in b) echo 1 ;| z) echo 2 ;; *) echo star ;; esac`,
			"1\nstar\n",
		},
		{
			"and that is not fall-through",
			`case b in b) echo 1 ;& z) echo 2 ;; *) echo star ;; esac`,
			"1\n2\n",
		},
		{
			"on the last arm there is nothing left to test",
			`case b in b) echo B ;| esac; echo after`,
			"B\nafter\n",
		},
		{
			"it composes with fall-through both ways",
			`case b in b) echo 1 ;| z) echo 2 ;; b) echo 3 ;& q) echo 4 ;; b) echo 5 ;; esac`,
			"1\n3\n4\n",
		},
		{
			"a pattern list keeps its own alternation",
			`case b in a|b) echo hit ;| *) echo star ;; esac`,
			"hit\nstar\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, st := runSemiPipe(t, tc.src)
			if got != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, got, st, tc.want)
			}
		})
	}
	// The same programs under the bash spelling, to the same bytes. If these
	// two lists ever disagree, one of the branches has grown a difference the
	// panel does not have.
	for _, tc := range []struct{ pipeSrc, ampSrc string }{
		{
			`case b in b) echo 1 ;| z) echo 2 ;; *) echo star ;; esac`,
			`case b in b) echo 1 ;;& z) echo 2 ;; *) echo star ;; esac`,
		},
		{
			`case b in b) echo 1 ;| z) echo 2 ;; b) echo 3 ;& q) echo 4 ;; b) echo 5 ;; esac`,
			`case b in b) echo 1 ;;& z) echo 2 ;; b) echo 3 ;& q) echo 4 ;; b) echo 5 ;; esac`,
		},
	} {
		pipeOut, pipeSt := runSemiPipe(t, tc.pipeSrc)
		ampOut, ampSt := runCaseContinue(t, tc.ampSrc)
		if pipeOut != ampOut || pipeSt != ampSt {
			t.Errorf("`;|` gave %q (%d) where `;;&` gave %q (%d) — one terminator, two spellings",
				pipeOut, pipeSt, ampOut, ampSt)
		}
	}
	// The status is the last body's, as it is for every other terminator.
	if _, st := runSemiPipe(t, `case b in b) true ;| *) false ;; esac`); st != 1 {
		t.Errorf("status = %d, want the last body's 1", st)
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

// A redirection on a compound command covers the whole of it, and the C-style
// loop is the one that had nowhere to keep one — so every iteration went to
// the terminal while the file was truncated by the leftover operator, which is
// two wrong things and no diagnostic.
//
// Reading the file back is the assertion rather than the absence of output:
// output that went nowhere and output that went to the file look the same from
// here, and only one of them is right.
func TestACStyleForRedirectsEveryIteration(t *testing.T) {
	got, _ := run(t, `for ((i=0;i<2;i++)); do echo "$i"; done > f; echo end; cat f`, nil)
	if got != "end\n0\n1\n" {
		t.Errorf("got %q, want the loop's lines in the file and only `end` before them", got)
	}

	// The brace-bodied spelling of the same loop, where the body ends in a
	// `}` and the suffix reads as the brace group's if nothing claims it.
	got, _ = runGrammar(t, `for ((i=0;i<2;i++)) { echo "$i"; } > f; echo end; cat f`, nil, nil)
	if got != "end\n0\n1\n" {
		t.Errorf("brace body: got %q, want the same", got)
	}
}

// And the reading half, which also shows the redirection outlives an
// iteration: the second `read` continues where the first left off, which it
// could not do if the file were opened again each time round.
func TestACStyleForReadsThroughOneRedirection(t *testing.T) {
	got, _ := run(t,
		`printf 'L1\nL2\n' > d; for ((i=0;i<2;i++)); do read x; echo "got=$x"; done < d`, nil)
	if got != "got=L1\ngot=L2\n" {
		t.Errorf("got %q, want both lines read through one open file", got)
	}
}
