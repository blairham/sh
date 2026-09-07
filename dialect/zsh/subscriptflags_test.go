// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A subscript's own flag group in this dialect.
//
// The substrate's tests name the grammar flag and never a shell; this one
// names the shell, because whether these characters are a search or an
// arithmetic expression is exactly what the preset decides. Measured
// 2026-09-06 on zsh 5.9.2, and against the other four, which read every one
// of these as arithmetic and fail there.
func TestASubscriptFlagGroupIsThisDialects(t *testing.T) {
	const a = "a=(alpha beta gamma beta delta)\n"
	for _, tc := range []struct{ src, want string }{
		{`printf "[%s]" "${a[(r)*a]}"`, `[alpha]`},
		{`printf "[%s]" "${a[(R)*a]}"`, `[delta]`},
		{`printf "[%s]" "${a[(i)be*]}"`, `[2]`},
		{`printf "[%s]" "${a[(I)be*]}"`, `[4]`},
		{`printf "[%s]" "${a[(i)zz]}"`, `[6]`},
		{`printf "[%s]" "${a[(I)zz]}"`, `[0]`},
		{`printf "[%s]" "${a[(re)be*]}"`, `[]`},
		{`printf "[%s]" "${a[(re)beta]}"`, `[beta]`},
		{`printf "[%s]" "${a[(rn:2:)*a]}"`, `[beta]`},
		{`printf "[%s]" "${a[(rb:3:)*a]}"`, `[gamma]`},
		// The brace-less spelling, which is a lexer question in this dialect
		// and a syntax error in the four without it.
		{`printf "[%s]" "$a[(r)*a]"`, `[alpha]`},
		// A group that selects nothing leaves the ordinary reading alone.
		{`printf "[%s]" "${a[()2]}"`, `[beta]`},
		// A subscript is never a filename, which only this dialect's
		// `nomatch` can show: with the pathname step still running, the
		// parenthesis in the operand makes the whole subscript a pattern
		// that matches nothing and the word is abandoned.
		{`b=('(e)beta' beta); printf "[%s]" "${b[(re)(e)beta]}"`, `[(e)beta]`},
	} {
		out, st := runZsh(t, t.TempDir(), a+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The shape the plugin manager on this machine writes six times before it
// defines anything, which is why this dialect has the construct at all.
func TestTheFlagGroupShapeAPluginManagerAsksWith(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`p=(/u/bin /pfx/bin); Z=/pfx; [[ -z ${p[(re)${Z}/bin]} ]] && printf add || printf keep`, "keep"},
		{`p=(/u/bin); Z=/pfx; [[ -z ${p[(re)${Z}/bin]} ]] && printf add || printf keep`, "add"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// A group this implementation reads and does not carry is refused by name,
// which is the convention that let the flag set be enumerated exactly rather
// than guessed at.
func TestASubscriptFlagThisDialectReadsAndDoesNotCarry(t *testing.T) {
	for _, tc := range []struct{ src, names string }{
		{`a=(x y); printf "[%s]" "${a[(w)y]}"`, "(w)"},
		{`a=(x y); printf "[%s]" "${a[(k)y]}"`, "(k)"},
		{`s=xy; printf "[%s]" "${s[(r)y]}"`, "(r)"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if !strings.Contains(out, tc.names+" subscript flag is not implemented") || st == 0 {
			t.Errorf("%s = %q (status %d), want a refusal naming %s", tc.src, out, st, tc.names)
		}
	}
}

// A group this dialect cannot read is no group, so the subscript is read as
// arithmetic — which is the same reading the four dialects without the
// construct give it, and the whole of why the flag is additive.
func TestAnUnreadableSubscriptFlagGroupIsArithmetic(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `a=(x y); printf "[%s]" "${a[(z)2]}"`)
	if !strings.Contains(out, "bad math expression") || st == 0 {
		t.Errorf(`${a[(z)2]} = %q (status %d), want an arithmetic failure`, out, st)
	}
}

// A search over an *association* is this dialect's too, and it is a different
// construct from the ordered array's search above: the four letters select
// keys, and their case is how many matches come back rather than which end
// the search started from. Measured 2026-09-07 on zsh 5.9.2.
//
// It is here as well as in the substrate because the preset is what makes
// these characters a search at all — the other four read every row as
// arithmetic and fail there — and because the *shape* below is the one the
// plugin manager on this machine reaches before it has defined anything.
func TestASearchOverAnAssociationIsThisDialects(t *testing.T) {
	const m = "typeset -A m=(gamma one alpha two beta one)\n"
	for _, tc := range []struct{ src, want string }{
		{`printf "[%s]" "${m[(i)alpha]}"`, `[alpha]`},
		{`printf "[%s]" "${m[(i)*a]}"`, `[alpha]`},
		{`printf "[%s]" "${m[(I)*a]}"`, `[alpha beta gamma]`},
		{`printf "[%s]" "${m[(r)one]}"`, `[one]`},
		{`printf "[%s]" "${m[(R)one]}"`, `[one one]`},
		{`printf "[%s]" "${m[(i)zz]}"`, `[]`},
		{`printf "[%s]" "${m[(I)zz]}"`, `[]`},
		{`printf "[%s]" "${#m[(I)*a]}"`, `[3]`},
		{`printf "[%s]" ${(@)m[(I)*a]}`, `[alpha][beta][gamma]`},
		{`printf "[%s]" "${(v)m[(i)alpha]}"`, `[two]`},
	} {
		out, st := runZsh(t, t.TempDir(), m+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The line the plugin manager reaches at the eleventh of its own, and the one
// at the ninetieth: a hook table searched for a literal name and for every
// name under a prefix. Both are `[@]`-shaped answers from a subscript nobody
// wrote `@` in.
func TestTheHookTableSearchAPluginManagerReaches(t *testing.T) {
	const e = "typeset -A e=('z-annex subcommand:wait' w 'z-annex subcommand:load' l other o)\n"
	for _, tc := range []struct{ src, want string }{
		{`printf "[%s]" "${e[(I)z-annex subcommand:wait]}"`, `[z-annex subcommand:wait]`},
		{`printf "[%s]" "${e[(I)z-annex subcommand:nope]}"`, `[]`},
		{`printf "[%s]" ${(on)e[(I)z-annex subcommand:*]}`, `[z-annex subcommand:load][z-annex subcommand:wait]`},
	} {
		out, st := runZsh(t, t.TempDir(), e+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// `(A)` in this dialect: nothing where the expansion does not assign, and a
// refusal naming the flag where it does.
func TestTheArrayFlagIsThisDialects(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`v="a|b"; printf "[%s]" "${(@Akons:|:u)v}"`, `[a][b]`},
		{`v="a b"; printf "[%s]" "${(A)#v}"`, `[3]`},
		{`v=abc; printf "[%s]" "${(AA)v}"`, `[abc]`},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
	out, st := runZsh(t, t.TempDir(), `printf "[%s]" "${(A)u=x y}"`)
	if !strings.Contains(out, "(A) expansion flag is not implemented for an assignment") || st == 0 {
		t.Errorf(`${(A)u=x y} = %q (status %d), want the assignment refused`, out, st)
	}
}
