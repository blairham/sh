// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// This shell writes a `case` with braces as well as with `in` … `esac`, and
// the two halves are independent — either opener composes with either closer.
// Measured 2026-09-12 on zsh 5.9.2 under `env -i PATH=/usr/bin:/bin` with a
// scratch HOME; the other five refuse the `{`.
//
//	$ zsh -c 'case x { x) echo hit;; }'
//	hit
func TestABraceSpelledCaseRunsHere(t *testing.T) {
	if got := zsh.Dialect().CaseBraceBody; got != syntax.CaseBraceBodyMixesWithTheKeyword {
		t.Errorf("spelling = %v, want CaseBraceBodyMixesWithTheKeyword", got)
	}
	for _, tc := range []struct{ src, want string }{
		{"case x { x) echo hit;; }", "hit"},
		{"case x { x) echo hit;; *) echo no;; }", "hit"},
		{"case y { x) echo hit;; *) echo star;; }", "star"},
		// The last arm needs no terminator before the closer.
		{"case x { x) echo hit }", "hit"},
		// No arm at all, and it succeeds.
		{"case x { }; echo st=$?", "st=0"},
		// Brace open, keyword close.
		{"case x { x) echo hit;; esac", "hit"},
		// Keyword open, brace close.
		{"case x in x) echo hit;; }", "hit"},
		// The `{` needs no blank after it, as in command position.
		{"case x {x) echo hit;; }", "hit"},
		// A pattern may carry its own leading paren either way.
		{"case x { (x) echo hit;; }", "hit"},
		// A subject that would brace-expand keeps its braces, as it does
		// after `in`.
		{"case {a,b} {*) echo star;; }", "star"},
		{"case x {\nx) echo hit;;\n}", "hit"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}
}

// The brace opener does not bring ksh93's reading of the word after a `case`
// header with it: `esac` is still reserved after the `{` here, where that
// shell reads the word after *either* opener as a pattern and prints `hit`.
//
//	$ zsh -c 'case esac { esac) echo hit;; }'
//	zsh:1: parse error near `)'
func TestEsacIsStillReservedAfterTheBraceHere(t *testing.T) {
	const src = "case esac { esac) echo hit;; }\n"
	_, err := syntax.Parse(src, zsh.Dialect())
	if err == nil {
		t.Fatalf("%q parsed; the `esac` closes the case here", src)
	}
	if got, want := zsh.Diagnostics().ParseFailure(err), "parse error near `)'"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// A parenthesized pattern is the way to write it, and it needs no flag.
	if out, _ := answersRun(t, "case esac { (esac) echo hit;; }"); strings.TrimSpace(out) != "hit" {
		t.Errorf("a parenthesized `esac` pattern said %q", out)
	}
}

// A long `if … ; then …` may carry an `elif` whose condition ended itself and
// whose body is written with braces. From there the chain is a short one and
// there is no `fi`.
//
//	$ zsh -c 'if (( 0 )); then echo A; elif (( 1 )) { echo B }; echo tail'
//	B
//	tail
func TestALongIfMayCarryABraceBodiedElifHere(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"if (( 0 )); then echo A; elif (( 1 )) { echo B }; echo tail", "B\ntail"},
		{"if [[ -n x ]]; then echo A; elif [[ -n y ]] { echo B }; echo tail", "A\ntail"},
		{"if (( 0 )); then echo A; elif (( 1 )) { echo B } else { echo C }", "B"},
		{"if (( 0 )); then echo A; elif (( 0 )) { echo B } else { echo C }", "C"},
		{"if (( 0 )); then echo A; elif (( 1 )) { echo B } elif (( 1 )) { echo C }", "B"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}
	for _, src := range []string{
		// The condition still has to end itself, which is the short form's
		// own test showing through rather than a rule of the `elif`'s.
		"if :; then echo A; elif : { echo B }\n",
		// And a long `else` takes no brace body: what follows it is an
		// ordinary group and the `fi` is still required.
		"if (( 0 )); then echo A; else { echo C }\n",
	} {
		if _, err := syntax.Parse(src, zsh.Dialect()); err == nil {
			t.Errorf("%q parsed, want a syntax error", src)
		}
	}
}
