// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// `functions` and `typeset -f`, this shell's function listing.
//
// The claim that matters is not the spacing: it is that the listing gives
// back the program it read. `typeset` declares a local in a `function f { … }`
// body here and assigns the global in an `f() { … }` one, so the two
// spellings are two programs and a listing that wrote one for the other hands
// over code whose variables leak. Measured 2026-09-12 on ksh93u+ 2012-08-01
// through `od -c`, from a script file (#1494).

// The round trip is the test that matters, and it asserts on what the
// function *does* rather than on what the listing says. A string comparison
// would pass against a word written where this grammar reads something else.
func TestAListedFunctionStillDeclaresTheLocalsItDid(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the keyword form keeps its local",
			"v=g\nfunction f { typeset v=in; }\neval \"$(functions f)\"\nf\necho \"out=$v\"\n",
			"out=g",
		},
		{
			// The control, and the one that says the test can tell the two
			// apart: the same body without the word leaks, before the trip
			// and after it.
			"the bare form still leaks",
			"v=g\nf() { typeset v=in; }\neval \"$(functions f)\"\nf\necho \"out=$v\"\n",
			"out=in",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKshSource(t, tc.src)
			if st != 0 || !strings.Contains(out, tc.want) {
				t.Errorf("%q = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The text itself, byte for byte against the real shell — the header in both
// its spellings, the compact body, and a bare listing running two of them
// together. Each `want` is what ksh93u+ writes for the same script.
func TestTheFunctionListingIsWrittenTheWayThisShellWritesOne(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a one-line body", "f(){ :; }\nfunctions f\n", "f(){ :; }\n"},
		{
			"the keyword spelling", "function g { typeset x=1; }\nfunctions g\n",
			"function g { typeset x=1; }\n",
		},
		{
			"two statements on one line", "f(){ echo a; echo b; }\nfunctions f\n",
			"f(){ echo a; echo b; }\n",
		},
		{
			"every function, in name order", "f(){ :; }\ng(){ :; }\nfunctions\n",
			"f(){ :; }\ng(){ :; }\n",
		},
		{"the other word for it", "f(){ :; }\ntypeset -f f\n", "f(){ :; }\n"},
		{"and its print letter", "f(){ :; }\ntypeset -fp f\n", "f(){ :; }\n"},
		// The names-only shape keeps the same distinction with punctuation
		// instead of a word, which is why the pair is in one listing: a row
		// with one declaration form reads as a fixed suffix.
		{
			"names only, both spellings",
			"f(){ :; }\nfunction g { :; }\ntypeset +f\n", "f()\ng\n",
		},
		{"the sign alone reaches the same listing", "f(){ :; }\nfunctions +\n", "f()\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKshSource(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%q = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// A name nobody defined is silent at 1, under either word.
func TestTheFunctionListingIsSilentForANameItDoesNotHold(t *testing.T) {
	for _, line := range []string{"functions nosuch", "typeset -f nosuch"} {
		out, st := runKshSource(t, "f(){ :; }\n"+line+"\n")
		if out != "" || st != 1 {
			t.Errorf("%s = %q (status %d), want silence at 1", line, out, st)
		}
	}
}

// The listing is the definition's **source text**, which is a stronger claim
// than any of the rows above: it is not that the arrangement resembles this
// shell's, it is that nothing is rearranged at all.
//
// Every `want` is what ksh93u+ 2012-08-01 writes for the same input, measured
// 2026-09-13 through `cat -A` (#2610). Each row is a shape a printer would
// get wrong in its own way, which is the point of having more than one: odd
// spacing, a comment, and a `${x}` are three separate things a tree has
// already forgotten by the time a listing is asked for.
func TestTheFunctionListingIsTheDefinitionsOwnSourceText(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The blanks are the assertion. A layout writes one space here
			// however many were typed.
			"the blanks a definition was written with",
			"f(){    echo     a   ;   }; typeset -f f",
			"f(){    echo     a   ;   };",
		},
		{
			// A comment is in no tree at all, so this row cannot pass by
			// coincidence.
			"a comment inside the body",
			"f() { # note\n :; }; typeset -f f",
			"f() { # note\n :; };",
		},
		{
			// The two spellings of a parameter are one node to everything
			// that expands them, so only the source says which was written.
			"a braced parameter keeps its braces",
			`f() { echo "${x}"; }; typeset -f f`,
			`f() { echo "${x}"; };`,
		},
		{
			// The terminator belongs to the listing, and under `-c` it is a
			// `;` with no newline after it — so the next thing written runs
			// straight on.
			"the terminator is written and nothing is added after it",
			`f() { :; }; typeset -f f; echo AFTER`,
			"f() { :; };AFTER\n",
		},
		{
			// Whatever stood between the body and the terminator is in the
			// span too.
			"the blanks before the terminator as well",
			"f() { :; }   ; typeset -f f",
			"f() { :; }   ;",
		},
		{
			// Nothing terminated it, so nothing terminates the listing.
			"a definition with no terminator after it ends at its body",
			`eval "f() { :; }"; typeset -f f`,
			"f() { :; }",
		},
		{
			// Each one carries its own terminator and the shell writes no
			// separator between them, which is what makes a bare listing of
			// two definitions run together.
			"two functions, each with its own terminator and nothing between",
			"f() { :; }; g() { :; }; typeset -f",
			"f() { :; };g() { :; };",
		},
		{
			// A body written over several lines is where a layout and the
			// source part company: this keeps the newlines and the
			// indentation, and the terminator is the newline after the `}`.
			"a body written over several lines",
			"f() {\n  echo a\n  echo b\n}\ntypeset -f f",
			"f() {\n  echo a\n  echo b\n}\n",
		},
		{
			// The keyword spelling goes through the same span, so the word
			// comes back because it was written and not because a header
			// puts it back.
			"the keyword spelling comes back as written",
			"function  g  {  typeset x=1; }; typeset -f g",
			"function  g  {  typeset x=1; };",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKshSource(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%q = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
