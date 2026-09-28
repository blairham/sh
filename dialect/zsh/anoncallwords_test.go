// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// What may stand behind a nameless function's body, and it is **not the same
// for the two headers** (#5079).
//
// The parenthesised header reads the call's words the way a simple command
// reads its own — words and redirections in any order. The keyword header
// takes words alone, and a redirection among them is a refusal. This engine
// refused the first and took the second, so it was wrong in both directions
// at once.
//
// Measured 2026-09-28 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*, so the reference is that shell and not
// another build of this one), script files under `env -i PATH=/usr/bin:/bin`
// with a scratch HOME and standard input on the null device.
//
// The rows are a grid with the **header** as the only thing that moves: the
// body is the same text and the words are the same words. A list of `()`
// spellings alone agrees with "words and redirections always interleave",
// which is wrong in half the grammar.
func TestTheParenthesisedHeaderInterleavesWordsAndRedirections(t *testing.T) {
	const body = `{ printf "[%s]" "$*" }`
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"a redirection between two words", "() " + body + " a 2>e b", "[a b]"},
		{"two of them, between three", "() " + body + " a 2>e b 3>g c", "[a b c]"},
		{"one written with a descriptor", "() " + body + " a 3>f b", "[a b]"},
		{"and a duplication", "() " + body + " a 2>&1 b", "[a b]"},
		// The two ends, which agreed before and still do: the fault was
		// only ever a word *after* a redirection.
		{"all of them behind the words", "() " + body + " a b 2>e", "[a b]"},
		{"and all of them in front", "() " + body + " 2>e a b", "[a b]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The keyword header is the other half of that grid: words, then the trailing
// redirections any compound command carries, and **no word after one**.
func TestTheKeywordHeaderTakesItsWordsBeforeAnyRedirection(t *testing.T) {
	const body = `{ printf "[%s]" "$*" }`
	dir := t.TempDir()
	// The rows that run. Words alone, and words with the redirections
	// behind them.
	for _, tc := range []struct{ name, src, want string }{
		{"words alone", "function " + body + " a b", "[a b]"},
		{"and with a redirection behind them", "function " + body + " a b >f", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
	// And the rows that do not. Each is the parenthesised row above with
	// only the header changed, which is what makes the pair a measurement
	// of the header rather than of the position.
	for _, src := range []string{
		"function " + body + " a >f b\n",
		"function " + body + " >f a b\n",
		// The body on a later line is the same header and answers the same.
		"function\n" + body + " a >f b\n",
		"function\n" + body + " >f a b\n",
	} {
		if _, err := syntax.Parse(src, zsh.Dialect()); err == nil {
			t.Errorf("%q parsed, want a refusal", src)
		}
	}
	// The pair, spelled out: the same text under the other header runs.
	for _, src := range []string{
		"() " + body + " a >f b\n",
		"() " + body + " >f a b\n",
	} {
		if _, err := syntax.Parse(src, zsh.Dialect()); err != nil {
			t.Errorf("%q was refused (%v), and only the header differs from a row above", src, err)
		}
	}
}

// A redirection written **in front of** the body is the body's; one written
// behind it is the call's.
//
// The call's own trace line is what tells them apart, because it is the one
// thing written between the two: it is drawn before the frame is pushed, so a
// redirection that belongs to the call catches it and one that belongs to the
// body does not. `PS4` is set to something without `%N` in it so the rows
// grade where the bytes went and nothing else.
func TestARedirectionInFrontOfTheBodyIsTheBodys(t *testing.T) {
	const pre = "PS4='@ '\nsetopt xtrace\n"
	const tail = "unsetopt xtrace\nprint -r -- \"[$(<e)]\"\n"
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"in front, and the call's line stays out of the file",
			pre + "() 2>e { print A }\n" + tail,
			"@ '(anon)'\nA\n@ unsetopt xtrace\n[@ print A]\n",
		},
		{
			"behind, and it catches the call's line too",
			pre + "() { print A } 2>e\n" + tail,
			"A\n@ unsetopt xtrace\n[@ '(anon)'\n@ print A]\n",
		},
		// The keyword header answers the same both ways, which is what says
		// this is about *where the redirection was written* and not about
		// the header the rows above turn on.
		{
			"the keyword header, in front",
			pre + "function 2>e { print A }\n" + tail,
			"@ '(anon)'\nA\n@ unsetopt xtrace\n[@ print A]\n",
		},
		{
			"and the keyword header, behind",
			pre + "function { print A } 2>e\n" + tail,
			"A\n@ unsetopt xtrace\n[@ '(anon)'\n@ print A]\n",
		},
		// Both at once on one call, with words behind as well.
		{
			"one of each, and the words still read",
			pre + "() 2>e { print A } 3>g a b\n" + tail,
			"@ '(anon)' a b\nA\n@ unsetopt xtrace\n[@ print A]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A redirection after the bare keyword ends the name list, and what stands
// behind it on the same command is a **body**.
//
// This is the row that cannot be guessed from the spelling, and it is the one
// the whole branch turns on: `function foo` makes `foo` a *name*, and
// `function >f foo` **runs** it. Measured with `foo` already defined —
// `${#functions}` is 1 either side, so nothing was declared.
func TestTheKeywordTakesABodyBehindARedirection(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"a brace body",
			`function >f { printf A }; printf "[%s]" "$(<f)"`, "[A]",
		},
		{
			"a simple command as the body",
			`function >f printf A; printf "[%s]" "$(<f)"`, "[A]",
		},
		{
			"a subshell as the body",
			`function >f ( printf A ); printf "[%s]" "$(<f)"`, "[A]",
		},
		{
			"a word that names a function is a body and not a name",
			`foo() { printf "ran" }; function >f foo; printf "[%s]" "$(<f)"; printf " n=%d" ${#functions}`,
			"[ran] n=1",
		},
		{
			"two redirections in front of it",
			`function >f 2>e { printf A; printf B >&2 }; printf "[%s][%s]" "$(<f)" "$(<e)"`,
			"[A][B]",
		},
		// And the bound: a separator between the redirection and the body
		// is a token of its own, so the keyword stands alone and the next
		// line is the script's. Both of these write `A` to the terminal and
		// leave the file empty. The null command is named so the rows need
		// nothing on PATH — its default is `cat`, and a row that could not
		// find it would report an absent null command as a working one.
		{
			"a newline between the two leaves it bare",
			"NULLCMD=:\nfunction >f\n" + `print -r -- A` + "\n" + `printf "[%s]" "$(<f)"`, "A\n[]",
		},
		{
			"and so does a semicolon",
			"NULLCMD=:\nfunction >f; " + `print -r -- A` + "\n" + `printf "[%s]" "$(<f)"`, "A\n[]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
