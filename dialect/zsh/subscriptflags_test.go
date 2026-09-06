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
		{`typeset -A h; h[k]=v; printf "[%s]" "${h[(r)v]}"`, "(r)"},
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
