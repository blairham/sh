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
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if !strings.Contains(out, tc.names+" subscript flag is not implemented") || st == 0 {
			t.Errorf("%s = %q (status %d), want a refusal naming %s", tc.src, out, st, tc.names)
		}
	}
	// The one target still refused: a search on the *left* of `=` over a
	// plain string names a character position, and this dialect writes a
	// character there — `s=hello; s[3]=Q` is `heQlo` in the shell — which is
	// not built, so the index the search found has nowhere to go. The value
	// is asserted with the refusal because an assignment refused this way
	// leaves the status at 0, so nothing else says the write did not land.
	out, _ := runZsh(t, t.TempDir(), `s=hello; s[(r)l]=Q; printf "[%s]" "$s"`)
	if !strings.Contains(out, "(r) subscript flag is not implemented for a scalar") ||
		!strings.Contains(out, "[hello]") {
		t.Errorf(`s[(r)l]=Q = %q, want the flag refused and hello kept`, out)
	}
}

// A search over a plain string is this dialect's too, and it is the ordered
// array's search counting through *characters*: the operand matches a
// substring, the answer is where that substring starts, and `(r)` reads that
// position as an ordinary subscript.
//
// Measured 2026-09-08 on zsh 5.9.2 with `s="hello world"`, and against the
// other four, which read every row as arithmetic and fail there. It is here
// as well as in the substrate because the *preset* is what makes a subscript
// on a string a character at all, and because the shape at the bottom is the
// one the plugin manager on this machine reaches three times before it has
// loaded anything.
func TestASearchOverAScalarIsThisDialects(t *testing.T) {
	const s = `s="hello world"` + "\n"
	for _, tc := range []struct{ src, want string }{
		{`printf "[%s]" "${s[(i)l]}"`, `[3]`},
		{`printf "[%s]" "${s[(I)l]}"`, `[10]`},
		{`printf "[%s]" "${s[(r)[hd]]}"`, `[h]`},
		{`printf "[%s]" "${s[(R)[hd]]}"`, `[d]`},
		// A multi-character operand matches, and `r` still answers with one
		// character rather than with the text that matched.
		{`printf "[%s]" "${s[(i)wor]}" "${s[(r)wor]}"`, `[7][w]`},
		// Both misses read as subscripts, and neither names a character.
		{`printf "[%s]" "${s[(i)zz]}" "${s[(I)zz]}"`, `[12][0]`},
		{`printf "[%s]" "${s[(r)zz]}" "${s[(R)zz]}"`, `[][]`},
		// One position past the last character, where only an empty match
		// lands.
		{`printf "[%s]" "${s[(I)*]}" "${s[(I)?]}"`, `[12][11]`},
		{`printf "[%s]" "${s[(ie)lo]}" "${s[(in:2:)l]}" "${s[(ib:5:)l]}"`, `[4][4][10]`},
		// An empty string answers neither end, which the rule above does not
		// predict.
		{`e=; printf "[%s]" "${e[(i)x]}" "${e[(I)x]}"`, `[0][0]`},
		// The brace-less spelling reaches the same reading.
		{`printf "[%s]" "$s[(r)l]"`, `[l]`},
	} {
		out, st := runZsh(t, t.TempDir(), s+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// A character is the locale's rather than a byte, which only a dialect test
// can show: the substrate answers the encoding question from an axis, and
// this preset is the one that answers it yes.
func TestASearchOverAScalarCountsCharacters(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`LC_ALL=en_US.UTF-8; s="héllo"; printf "[%s]" "${s[(i)l]}" "${s[(i)é]}" "${s[(r)é]}"`, `[3][2][é]`},
		{`LC_ALL=C; s="héllo"; printf "[%s]" "${s[(i)l]}"`, `[4]`},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The shape the plugin manager on this machine reaches three times before it
// has loaded anything: a search used as a present-or-absent test, landing on
// a name that holds a *string* rather than the array it reads as.
func TestTheOptionTestAPluginManagerReachesOverAString(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`opts="-X -w"; [[ -n ${opts[(r)-X]} ]] && printf found || printf missing`, "found"},
		{`opts="-X -w"; [[ -n ${opts[(r)-C]} ]] && printf found || printf missing`, "missing"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
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
