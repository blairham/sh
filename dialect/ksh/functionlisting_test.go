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
