// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// escapingFlags is the grammar these expansions need, named by the constructs
// rather than by a shell.
func escapingFlags(d *syntax.Dialect) {
	d.ParamExpansionFlags = true
	d.ArrayLiteral = true
	d.ArraySubscript = true
}

// withTestEscapes installs an escape set of two letters, plus the rule that
// an escape it does not know loses its backslash.
//
// Two letters and not the measured forty: what this package answers is *when*
// the decoder runs and over what text, and a decoder carrying the whole
// alphabet would let a test pass on the alphabet while the position rule was
// wrong. The set a real shell reads is asserted where it was measured, in
// that shell's own package.
//
// The unknown-escape rule is here because one row needs it: `\$s` has to
// arrive as `$s`, which is what says the name substitution happens *before*
// the decoding rather than after.
func withTestEscapes(r *interp.Runner) {
	r.SetFlagArgumentEscapes(func(s string) string {
		var b strings.Builder
		for i := 0; i < len(s); i++ {
			if s[i] != '\\' || i+1 == len(s) {
				b.WriteByte(s[i])
				continue
			}
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte(s[i])
			}
		}
		return b.String()
	})
}

// The flag reads the argument of the flags written after it, and nothing else.
//
// Every row is a measurement on zsh 5.9.2, the only shell in the panel with
// the construct — see docs/spec/grammar/parameter-expansion.md — with the
// escape set narrowed to the two letters this package installs.
func TestThePrintFlagReadsTheArgumentsBehindIt(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a join separator", `a=(x y); printf "[%s]" "${(pj:\n:)a}"`, "[x\ny]"},
		{"and without the flag it is two characters", `a=(x y); printf "[%s]" "${(j:\n:)a}"`, `[x\ny]`},
		// The order is the rule: a `p` behind the flag it would modify
		// modifies nothing at all.
		{"a flag written before the p is not reached", `a=(x y); printf "[%s]" "${(j:\n:p)a}"`, `[x\ny]`},
		{"doubling it adds nothing", `a=(x y); printf "[%s]" "${(ppj:\n:)a}"`, "[x\ny]"},
		{"a split separator", `v="a	b"; printf "[%s]" ${(ps:\t:)v}`, "[a][b]"},
		{"both, in one group", `a=(x y); printf "[%s]" ${(pj:\t:s:\t:)a}`, "[x][y]"},
		// With no argument-taking flag behind it there is nothing to read,
		// and that is a no-op rather than an error.
		{"alone it is a no-op", `v=ab; printf "[%s]" "${(p)v}"`, "[ab]"},
		{"beside a flag that takes no argument", `a=(y x); printf "[%s]" "${(po@)a}"`, "[x][y]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, escapingFlags, withTestEscapes)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// An argument that is exactly `$name` is substituted instead of being read
// for escapes. The two are alternatives and not steps, which is what the
// `\$s` row pins: it decodes to `$s`, so a reading that escaped first and
// looked for a name second would substitute there and the shell does not.
func TestThePrintFlagSubstitutesASoleParameter(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a name on its own", `s=-; a=(x y); printf "[%s]" "${(pj:$s:)a}"`, "[x-y]"},
		{"and its value is not then read for escapes", `s='\n'; a=(x y); printf "[%s]" "${(pj:$s:)a}"`, `[x\ny]`},
		// Not the whole argument, so not a name.
		{"text in front of it", `s=-; a=(x y); printf "[%s]" "${(pj:A$s:)a}"`, "[xA$sy]"},
		{"text behind it", `s=-; a=(x y); printf "[%s]" "${(pj:${s}A:)a}"`, "[x${s}Ay]"},
		{"an escape behind it", `s=-; a=(x y); printf "[%s]" "${(pj:$s\t:)a}"`, "[x$s\ty]"},
		{"braces", `s=-; a=(x y); printf "[%s]" "${(pj:${s}:)a}"`, "[x${s}y]"},
		{"an escaped dollar", `s=-; a=(x y); printf "[%s]" "${(pj:\$s:)a}"`, "[x$sy]"},
		{"an unset name stays as written", `a=(x y); printf "[%s]" "${(pj:$nosuch:)a}"`, "[x$nosuchy]"},
		{"a set and empty name is empty", `e=; a=(x y); printf "[%s]" "${(pj:$e:)a}"`, "[xy]"},
		{"a bare dollar", `a=(x y); printf "[%s]" "${(pj:$:)a}"`, "[x$y]"},
		{"a parameter that is not a name", `a=(x y); printf "[%s]" "${(pj:$#:)a}"`, "[x$#y]"},
		{"and without the flag no name is substituted", `s=-; a=(x y); printf "[%s]" "${(j:$s:)a}"`, "[x$sy]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, escapingFlags, withTestEscapes)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A positional is substituted whether or not it is there, where a name has to
// be set. Measured with `set -- P Q`.
func TestThePrintFlagSubstitutesAPositionalEvenWhenAbsent(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"one that is there", `set -- P Q; a=(x y); printf "[%s]" "${(pj:$2:)a}"`, "[xQy]"},
		{"one that is not", `set -- P Q; a=(x y); printf "[%s]" "${(pj:$9:)a}"`, "[xy]"},
		{"and none at all", `set --; a=(x y); printf "[%s]" "${(pj:$1:)a}"`, "[xy]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, escapingFlags, withTestEscapes)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A runner nobody handed an escape set refuses the flag by name.
//
// This is the half that would rot into a stub. A `(p)` read as a no-op joins
// on a backslash and an `n` at status 0 — a plausible answer, from an
// expansion that was asked for the other thing — so the letter is carried
// only where the escapes it modifies are.
func TestThePrintFlagIsRefusedWithoutAnEscapeSet(t *testing.T) {
	out, st := runGrammar(t, `a=(x y); printf "[%s]" "${(pj:\n:)a}"`, escapingFlags, nil)
	want := "sh: ${(pj:\\n:)a}: the (p) expansion flag is not implemented\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if st == 0 {
		t.Errorf("status 0, want the refusal to be fatal")
	}
}

// And a letter this slice still does not carry is refused even with an escape
// set installed — the padding pair takes an argument the flag would reach, so
// the refusal has to name the padding rather than answer it.
func TestTheUnbuiltArgumentFlagsAreStillRefused(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"left padding", `v=ab; printf "[%s]" "${(pl:5::-:)v}"`, "the (l) expansion flag is not implemented"},
		{"right padding", `v=ab; printf "[%s]" "${(pr:5::-:)v}"`, "the (r) expansion flag is not implemented"},
		{"the word split", `v=ab; printf "[%s]" "${(pz)v}"`, "the (z) expansion flag is not implemented"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, escapingFlags, withTestEscapes)
			if !strings.Contains(out, tc.want) {
				t.Errorf("output = %q, want %q in it", out, tc.want)
			}
			if st == 0 {
				t.Errorf("status 0, want the refusal to be fatal")
			}
		})
	}
}
