// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `read -E` writes the values it reads to standard output, and `read -e`
// writes them and assigns none.
//
// Both letters were `-e is not implemented yet` at status 2 — they are in
// zsh's `Diagnostics.UnimplementedOptionLetters` for `read` and not in
// `Semantics.ReadOptions`, so the pair was refused as missing. They are not
// bash's letters: there the pair opens a line editor and does nothing off a
// terminal, which is why the meaning is an axis rather than a property of the
// letter — see Semantics.ReadEchoLettersWriteTheValues (#4963).
//
// Measured 2026-09-28 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable*
// for it — under `-f` from a script file with `env -i PATH=/usr/bin:/bin
// TERM=dumb` and a scratch `HOME`, **one shell per row**.
//
// One shell per row is not tidiness. zsh runs the last element of a pipeline
// in the *current* shell, so a grid that put several of these in one script
// read the previous row's values back and reported that `-e` assigns — which
// is what the first pass of this measurement said. `${+x}` rather than the
// value is what settles it, and every row below carries one.
func TestTheReadEchoLetters(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The headline pair. The echo is the **split values** and not
			// the record: two names, two lines, and the last name's line is
			// its remainder.
			"the capital letter writes the values and assigns them",
			`printf '  a   b   c  \n' | { read -E x y; print -r -- "x=[$x] y=[$y] p=${+x}${+y}"; }`,
			"a\nb   c\nx=[a] y=[b   c] p=11\n",
		},
		{
			"and the small letter writes them and assigns none",
			`printf '  a   b   c  \n' | { read -e x y; print -r -- "x=[$x] y=[$y] p=${+x}${+y}"; }`,
			"a\nb   c\nx=[] y=[] p=00\n",
		},
		{
			// A name with no field still gets its line, which is what says
			// the echo is one line per **name** rather than one per field.
			"a name with no field of its own still gets a line",
			`printf 'a\n' | { read -E x y z; print -r -- "x=[$x] y=[$y] z=[$z]"; }`,
			"a\n\n\nx=[a] y=[] z=[]\n",
		},
		{
			// And `-e` leaves a name **as it was** rather than clearing it.
			"the small letter leaves a name holding what it held",
			`printf 'a\n' | { x=keep; read -e x; print -r -- "st=$? x=[$x]"; }`,
			"a\nst=0 x=[keep]\n",
		},
		{
			// The suppression is a write never attempted, not one undone,
			// and a frozen name is how that is visible: silent at 0 here
			// where the capital letter is `read-only variable`.
			"and a frozen name is silent under the small letter",
			`printf 'a b\n' | { typeset -r fz=keep; read -e fz; print -r -- "st=$? fz=[$fz]"; }`,
			"a b\nst=0 fz=[keep]\n",
		},
		{
			// `-e` wins whichever order the two were written in, which is
			// the pair of rows that says the suppression turns on the letter
			// having appeared rather than on it having appeared last.
			"the small letter wins written second",
			`printf 'a\n' | { read -eE x; print -r -- "x=[$x] p=${+x}"; }`,
			"a\nx=[] p=0\n",
		},
		{
			"and written first",
			`printf 'a\n' | { read -Ee x; print -r -- "x=[$x] p=${+x}"; }`,
			"a\nx=[] p=0\n",
		},
		{
			// An array target echoes one line per **element**, and the
			// element count is the assignment's rather than the splitter's:
			// see the blank-line row below.
			"an array target writes one line per element",
			`printf 'a b c\n' | { read -E -A arr; print -r -- "arr=[${(j:|:)arr}] p=${+arr}"; }`,
			"a\nb\nc\narr=[a|b|c] p=1\n",
		},
		{
			"and the small letter leaves the array alone",
			`printf 'a b c\n' | { read -e -A arr; print -r -- "arr=[${(j:|:)arr}] p=${+arr}"; }`,
			"a\nb\nc\narr=[] p=0\n",
		},
		{
			// A line that splits into no fields still leaves one empty
			// element here, so it echoes one empty line. Taking the
			// splitter's list rather than the assignment's would have
			// written nothing.
			"a blank line is one empty element and one empty line",
			`printf '\n' | { read -E -A arr; print -r -- "st=$? n=${#arr}"; }`,
			"\nst=0 n=1\n",
		},
		{
			// End of input echoes too, and the two letters part there: the
			// capital one still assigns the empty value.
			"end of input writes a line and the capital letter still assigns",
			`printf '' | { read -E x; print -r -- "st=$? p=${+x} x=[$x]"; }`,
			"\nst=1 p=1 x=[]\n",
		},
		{
			"where the small letter assigns nothing",
			`printf '' | { read -e x; print -r -- "st=$? p=${+x}"; }`,
			"\nst=1 p=0\n",
		},
		{
			// The shell's own name is reached the same way, which is the
			// control that says this is about the value and not about an
			// operand.
			"the default name is written and, under the small letter, not filled",
			`printf 'a\n' | { read -e; print -r -- "REPLY=[$REPLY] p=${+REPLY}"; }`,
			"a\nREPLY=[] p=0\n",
		},
		{
			"and is filled under the capital one",
			`printf 'a\n' | { read -E; print -r -- "REPLY=[$REPLY]"; }`,
			"a\nREPLY=[a]\n",
		},
		{
			// The echo is the value **after** the escape processing, not the
			// record as it came: `a\tb` without `-r` is `atb` in both the
			// value and the line.
			"the line is the value the name would get, escapes and all",
			`printf 'a\\tb\n' | { read -E x; print -r -- "x=[$x]"; }`,
			"atb\nx=[atb]\n",
		},
		{
			// And the control that says none of this reaches an ordinary
			// `read`: no letter, no line, and the name is filled.
			"a read with neither letter writes nothing",
			`printf 'x\n' | { read x; print -r -- "x=[$x] p=${+x}"; }`,
			"x=[x] p=1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshPrelude(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// And the echo is on **standard output**, which is the half a merged capture
// cannot see.
//
// Two rows rather than one: the line survives `2>/dev/null` and disappears
// under `1>/dev/null`. A test that only asserted the text appeared would pass
// against a shell that wrote it to standard error, which is where a
// diagnostic would naturally have gone.
func TestTheReadEchoGoesToStandardOutput(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"standard error is not where it goes",
			`printf 'a b\n' | { read -E x 2>/dev/null; print -r -- after; }`,
			"a b\nafter\n",
		},
		{
			"and standard output is",
			`printf 'a b\n' | { read -E x 1>/dev/null; print -r -- after; }`,
			"after\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshPrelude(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
