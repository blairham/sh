// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// The short forms this shell has beyond its loops, end to end.
//
// `/usr/libexec/reset-ssh-configuration` — an Apple system script — is
// unparseable without the first of these (#827, #815).
func TestTheShortFormsRun(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"if [[ -n x ]] { echo A; }", "A"},
		{"if (( 1 )) { echo A; }", "A"},
		{"if (( 0 )) { echo A; } else { echo B; }", "B"},
		{"if [[ -z x ]] { echo A } elif [[ -n y ]] { echo B } else { echo C }", "B"},
		{"if [[ -n x ]] echo A; echo after", "A\nafter"},
		// The set of headers that end themselves is wider than the two
		// constructs the rule is usually stated with, and was measured.
		{"if ! [[ -z x ]] { echo A; }", "A"},
		{"if { true; } { echo A; }", "A"},
		{"if ( true ) { echo A; }", "A"},
		{"if case x in x) true;; esac { echo A; }", "A"},
		// A pipeline is judged on its last command, not refused outright.
		{"if true | [[ -n x ]] { echo A; }", "A"},
		// The count loop, in each of its body spellings.
		{"repeat 3 { echo R }", "R\nR\nR"},
		{"repeat 2 echo R; echo after", "R\nR\nafter"},
		{"repeat 2; do echo R; done", "R\nR"},
		{"repeat $((1+1)) { echo R }", "R\nR"},
		// A count that is not one is not an error.
		{"repeat x { echo R }; echo st=$?", "st=0"},
		{"repeat -1 { echo R }; echo st=$?", "st=0"},
		// But a count that is not an *expression* is the arithmetic failure
		// this dialect already words. It reports and carries on here where
		// the shell stops the script, which is the fatality question rather
		// than this construct's.
		{"repeat '1+' { echo R }", "zsh:1: bad math expression: operand expected at end of string"},
		{"repeat 5 { echo R; break }", "R"},
		// `repeat` is a keyword only where a command may begin.
		{"repeat=5; echo $repeat", "5"},
		// The loop that ends with `end`.
		{"foreach f (a b); echo $f; end", "a\nb"},
		{"foreach f in a b\necho $f\nend", "a\nb"},
		// A function with no name, run where it stands.
		{`() { echo "[$0][$1][$#]"; } p q`, "[(anon)][p][2]"},
		{"x=1; () { local x=2; echo in=$x }; echo out=$x", "in=2\nout=1"},
		{"() { return 4; }; echo st=$?", "st=4"},
		{"function { echo fanon; }", "fanon"},
		{"() echo x", "x"},
		// And parentheses with nothing after them are the empty subshell
		// they have always been rather than a body-less function.
		{"( ); echo ok", "ok"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}
}

// The rows the shell itself refuses, which is what makes the rule a rule
// rather than "a brace group may follow anything".
func TestTheShortFormsHaveABoundary(t *testing.T) {
	for _, src := range []string{
		// A word does not end a header.
		"if true { echo A; }\n",
		// A separator ends the whole command rather than the header.
		"if true; { echo A; }\n",
		"if [[ -n x ]]; { echo A }\n",
		// A loop does not end a condition, which is measured rather than
		// derived: it is the one compound command with its own end that is
		// refused here.
		"if for i in a; do true; done { echo A; }\n",
		"if [[ -n x ]] | cat { echo A; }\n",
		// `end` is reserved wherever a command may begin.
		"end\n",
		"end() { :; }\n",
		// And it closes a `foreach` rather than a `for`.
		"for f (a b); echo $f; end\n",
	} {
		if _, err := syntax.Parse(src, zsh.Dialect()); err == nil {
			t.Errorf("%q parsed, want a syntax error", src)
		}
	}
	// While the words are ordinary ones away from a command's start.
	for _, tc := range []struct{ src, want string }{
		{"echo end", "end"},
		{"end=5; echo $end", "5"},
		{"( echo sub )", "sub"},
		{"f() { echo named; }; f", "named"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}
}
