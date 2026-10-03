// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestKshGlobTakesTheQuantifiedGroups is #5422. Under `kshglob`, the
// quantified spellings `@(…)`, `+(…)`, `?(…)`, `*(…)` and `!(…)` are groups,
// in `case` and `[[ ]]` alike. The option is read when the pattern is
// matched, so a line that sets it already matches with it. Here `kshglob`
// moved only the bare-group grammar, so none of them matched.
//
// Every row measured 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`).
func TestKshGlobTakesTheQuantifiedGroups(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"setopt kshglob\n[[ b == @(a|b) ]] && print 1\n[[ bb == +(b) ]] && print 2\n[[ '' == ?(x) ]] && print 3\n[[ c == !(a|b) ]] && print 4\n[[ aa == *(a) ]] && print 5", "1\n2\n3\n4\n5\n"},
		{"setopt kshglob\ncase b in @(a|b)) print C;; esac", "C\n"},
		{"setopt kshglob; case b in @(a|b)) print same-line;; esac", "same-line\n"},
		{"setopt kshglob\np='@(a|b)'; [[ b == $~p ]] && print P", "P\n"},
		{"emulate ksh\n[[ b == @(a|b) ]] && print K", "K\n"},
		// Off again, the `@` is a character and the group is zsh's own.
		{"setopt kshglob; unsetopt kshglob\n[[ b == @(a|b) ]] && print no; [[ @a == @(a|b) ]] && print lit", "lit\n"},
		// The control: without the option nothing changes.
		{"[[ b == @(a|b) ]] && print no; [[ @b == @(a|b) ]] && print zsh", "zsh\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), c.src+"\n")
		if out != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
		}
	}
}

// TestShGlobWithKshGlobReadsABareParenthesisAsText is #5467. With `shglob` and
// `kshglob` both on, only a quantified parenthesis opens a group. A bare one
// is an ordinary character, so a `|` between two of them divides the whole
// pattern.
//
// Every row measured 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`).
func TestShGlobWithKshGlobReadsABareParenthesisAsText(t *testing.T) {
	for _, c := range []struct{ pattern, subject, want string }{
		{"a(b|c)", "a(b", "M\n"},
		{"a(b|c)", "c)", "M\n"},
		{"a(b|c)", "ab", ""},
		{"a(b|c)d", "c)d", "M\n"},
		{"x(y)z", "x(y)z", "M\n"},
		{"x(y)z", "xyz", ""},
		{"x(y|z)*", "z)q", "M\n"},
		{"x@(a|b)(c|d)", "xa(c", "M\n"},
		{"@(a|b)", "b", "M\n"},
	} {
		src := "setopt shglob kshglob; [[ '" + c.subject + "' == " + c.pattern + " ]] && print M\n"
		out, _ := runZsh(t, t.TempDir(), src)
		if out != c.want {
			t.Errorf("%s got %q, want %q", src, out, c.want)
		}
	}
	// The `case` arm reads it the same way, and without `shglob` the bare
	// group is zsh's own again.
	for _, c := range []struct{ src, want string }{
		{"setopt shglob kshglob; case 'a(b' in a(b|c)) print C;; esac", "C\n"},
		{"[[ ab == a(b|c) ]] && print Z", "Z\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), c.src+"\n")
		if out != c.want {
			t.Errorf("%s got %q, want %q", c.src, out, c.want)
		}
	}
}
