// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `type -w` names what a word is, which is how a syntax highlighter
// classifies every word on the line before it picks a colour (#2512).
//
// Measured 2026-09-12 against zsh 5.9.2. Six classes, `name: kind` a line, in
// the order the operands were given.
func TestTypeWordNamesTheKind(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a builtin", "type -w print\n", "print: builtin\n"},
		{"a reserved word", "type -w if\n", "if: reserved\n"},
		{"a function", "ff(){ :; }\ntype -w ff\n", "ff: function\n"},
		{"an alias", "alias aa=ls\ntype -w aa\n", "aa: alias\n"},
		{
			// The one place this differs from the other dialect's `-t` by
			// more than a word: `-t` says nothing at all for a name that is
			// nothing, where this names it. A highlighter reads the line for
			// every word typed, so silence would be indistinguishable from
			// never having asked.
			"a name that is nothing", "type -w nosuchzz\n", "nosuchzz: none\n",
		},
		{
			// An alias beats a function beats a builtin, which is the
			// resolution order and not this letter's own.
			"an alias over a builtin", "alias print=echo\ntype -w print\n", "print: alias\n",
		},
		{"a function over a builtin", "print(){ :; }\ntype -w print\n", "print: function\n"},
		{
			"a line per operand, in order",
			"type -w print if nosuchzz\n",
			"print: builtin\nif: reserved\nnosuchzz: none\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// The status is 0 only when every operand was something.
func TestTypeWordStatusFollowsTheOperands(t *testing.T) {
	for _, c := range []struct {
		name, src string
		want      int
	}{
		{"all found", "type -w print if\n", 0},
		{"one missing", "type -w print nosuchzz\n", 1},
		{"all missing", "type -w nosuchzz alsonone\n", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, st := runZsh(t, t.TempDir(), c.src); st != c.want {
				t.Errorf("status %d, want %d", st, c.want)
			}
		})
	}
}

// `-t` is not a letter this shell has, which is the other half of the pair:
// the two dialects spell the same question differently and each refuses the
// other's spelling.
func TestTypeKindLetterIsNotThisShells(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "type -t ls\n")
	if want := "zsh:type:1: bad option: -t\n"; out != want || st != 1 {
		t.Errorf("got %q status %d, want %q at 1", out, st, want)
	}
}

// And `whence -w` says the same words, because they are two names for one
// builtin and the table behind them is one table.
func TestWhenceAndTypeAgreeOnTheWords(t *testing.T) {
	for _, src := range []string{"print", "if", "nosuchzz"} {
		byType, _ := runZsh(t, t.TempDir(), "type -w "+src+"\n")
		byWhence, _ := runZsh(t, t.TempDir(), "whence -w "+src+"\n")
		if byType != byWhence {
			t.Errorf("%s: type says %q and whence says %q", src, byType, byWhence)
		}
	}
}
