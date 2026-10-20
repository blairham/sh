// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// `;|` — this shell's spelling of bash's `;;&`, run rather than only parsed.
//
// Measured 2026-09-07 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin` with a
// scratch HOME, ZDOTDIR and HISTFILE, over a script file (#1188).
//
// The two spellings are **mutually exclusive**, which is the whole reason
// this is a flag of its own: this shell takes `;|` and refuses `;;&`, bash
// 4-and-later takes `;;&` and refuses `;|`, and dash, bash 3.2 and ksh93
// have neither.

func TestSemiPipeKeepsTestingLaterPatterns(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The arm runs and the *later patterns keep being tested*, so the
		// non-matching middle arm is skipped and the `*` is reached.
		{"case b in\n  b) echo 1 ;|\n  z) echo 2 ;;\n  *) echo star ;;\nesac", "1\nstar"},
		// Which is not fall-through: `;&` runs the middle body without
		// testing its pattern. Both operators exist here, and they differ.
		{"case b in\n  b) echo 1 ;&\n  z) echo 2 ;;\n  *) echo star ;;\nesac", "1\n2"},
		// Nothing after it to test, and the script carries on.
		{"case b in\n  b) echo B ;|\nesac\necho after", "B\nafter"},
		// It composes with fall-through in both directions.
		{"case b in\n  b) echo 1 ;|\n  z) echo 2 ;;\n  b) echo 3 ;&\n  q) echo 4 ;;\n  b) echo 5 ;;\nesac", "1\n3\n4"},
		// A pattern list still alternates; the `|` inside the list and the
		// `|` of the terminator are different slots.
		{"case b in\n  a|b) echo hit ;|\n  *) echo star ;;\nesac", "hit\nstar"},
		// The arm's body is a *list*, and `;|` is one of the tokens a list
		// ends on. #1142's shape is what shows it: an and-or with no
		// right-hand side is allowed here, so the body stops early and only
		// a terminator the list itself recognizes can close it. The `;;`
		// row below is the control — it prints nothing, which is how this
		// row's `star` is known to be the continuing and not the arm merely
		// ending.
		{"case b in\n  b) echo one; true ||\n  ;| *) echo star ;;\nesac", "one\nstar"},
		{"case b in b) true || ;| *) echo star ;; esac", "star"},
		{"case b in b) true || ;; *) echo star ;; esac", ""},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if strings.TrimSpace(out) != tc.want || st != 0 {
			t.Errorf("%q = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}

// The wording half, asked the way pipeboth_test.go asks it: this shell
// quotes the token whole and says nothing after it.
//
//	$ zsh -n s.sh     # case b in\n  b) echo B ;;&\n  *) echo star ;;\nesac
//	s.sh:2: parse error near `&'
//	$ zsh -n s.sh     # echo a ;| echo b
//	s.sh:1: parse error near `;|'
//	$ zsh -n s.sh     # case b in\n  b) echo B ; |\n  *) echo star ;;\nesac
//	s.sh:2: parse error near `|'
//
// The three together are the mutual exclusion and the tokenization. `;;&`
// stays refused, which is CaseContinue staying off — a single flag with two
// values could not have produced it, because the shell that has one spelling
// refuses the other. `;|` outside a `case` names *both bytes*, which a
// dialect lexing `;` and then `|` could not say, and is why the operator is
// not confined to the arm. And with a blank between them the bare `|` is
// named again, so the two bytes have to touch, like `|&`.
func TestTheOtherSpellingAndTheTokenAreNamedAsThisShellNamesThem(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the bash spelling stays refused",
			"case b in\n  b) echo B ;;&\n  *) echo star ;;\nesac\n",
			"parse error near `&'",
		},
		{
			"outside a case, both bytes are named",
			"echo a ;| echo b\n",
			"parse error near `;|'",
		},
		{
			"a blank between them is the bare bar again",
			"case b in\n  b) echo B ; |\n  *) echo star ;;\nesac\n",
			"parse error near `|'",
		},
		{
			"an operator where a pattern belongs is the operator, not a pattern",
			"case a in ;|a) echo hit;; *) echo miss;; esac\n",
			"parse error near `;|'",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := syntax.Parse(tc.src, zsh.Dialect())
			if err == nil {
				t.Fatalf("%q parsed, want a refusal", tc.src)
			}
			if got := zsh.Diagnostics().ParseFailure(err); got != tc.want {
				t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}

// The flag itself, stated by name so a dialect rebuild that dropped it fails
// here rather than only in the wild run.
func TestCaseContinuePipeIsOn(t *testing.T) {
	if !zsh.Dialect().CaseContinuePipe {
		t.Error("zsh reads `;|` as the terminator that keeps testing later patterns")
	}
	if zsh.Dialect().CaseContinue {
		t.Error("zsh refuses `;;&`; the two spellings are mutually exclusive")
	}
}
