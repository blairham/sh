// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// minimalQuoting is the grammar these rows need: the flag group, arrays, and
// `$'…'` for the values a quoted literal cannot spell.
func minimalQuoting(d *syntax.Dialect) {
	ordering(d)
	d.DollarSingleQuote = true
}

// The `q-` expansion flag: minimal quoting.
//
// Every row is a measurement on zsh 5.9.2, taken 2026-09-08 by handing the
// value in on argv and dumping the answer through `od -c`, because several of
// them differ only in an invisible byte.
//
// What the rows are picked to separate is `q-` from `q`, since a reading that
// quoted unconditionally would pass any row whose value needs quoting, and a
// reading that never quoted would pass any row whose value does not.
func TestTheMinimalQuotingFlag(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The point of the modifier: a value that needs nothing comes back
		// as itself, where `q` and `qq` both spell it out.
		{"a value needing nothing is bare", `v=plain; printf "[%s]" "${(q-)v}"`, "[plain]"},
		{"where q would leave a backslash", `v="a b"; printf "[%s][%s]" "${(q)v}" "${(q-)v}"`, `[a\ b]['a b']`},
		{"and qq would quote regardless", `v=plain; printf "[%s][%s]" "${(qq)v}" "${(q-)v}"`, "['plain'][plain]"},
		// A single quote cannot go inside single quotes, so it is the one
		// character written with a backslash — and the run after it opens a
		// fresh pair only if that run needs one.
		{"a quote is a backslash pair", `v="a'b"; printf "[%s]" "${(q-)v}"`, `[a\'b]`},
		{"and the run after it is quoted only when it must be", `v="a'b c"; printf "[%s]" "${(q-)v}"`, `[a\''b c']`},
		{"two quotes and nothing else", `v="a'b'c"; printf "[%s]" "${(q-)v}"`, `[a\'b\'c]`},
		{"a value that is one quote", `v="'"; printf "[%s]" "${(q-)v}"`, `[\']`},
		{"a quote at each end", `v="'a b'"; printf "[%s]" "${(q-)v}"`, `[\''a b'\']`},
		// The empty value is the one with nothing to quote that still cannot
		// be written bare, since bare it is no word at all.
		{"an empty value is a pair of quotes", `v=""; printf "[%s]" "${(q-)v}"`, "['']"},
		{"and an empty element of an array is too", `a=(x "" y); printf "[%s]" "${(@q-)a}"`, "[x][''][y]"},
		// The join runs first and the quoting wraps what it produced, which
		// is the contrast the `(~)` refusal is about: one pair of quotes
		// round the whole join and not one pair per element.
		{"a join is quoted once, not per element", `d=('a b' c); printf "[%s]" "${(q-j.|.)d}"`, "['a b|c']"},
		// Whitespace: a tab and a newline go inside the quotes as themselves,
		// where `q` spells them `$'\t'` and `$'\n'`.
		{"a tab is quoted, not escaped", "v=$'a\\tb'; printf \"[%s][%s]\" \"${(q)v}\" \"${(q-)v}\"", "[a$'\\t'b]['a\tb']"},
		{"a newline likewise", "v=$'a\\nb'; printf \"[%s]\" \"${(q-)v}\"", "['a\nb']"},
		// And every other control byte is *not* a reason to quote, which is
		// the row a reading of "unprintable means quote" fails. `q+` is the
		// flag that renders those, and it is refused by name.
		{"another control byte is left bare", "v=$'a\\001b'; printf \"[%s]\" \"${(q-)v}\"", "[a\x01b]"},
		{"unless something else asks for quotes", "v=$'a\\001 b'; printf \"[%s]\" \"${(q-)v}\"", "['a\x01 b']"},
		// The characters that do and do not need quoting, one row each way.
		// The second is the longer list and the one an over-eager table gets
		// wrong.
		{"the metacharacters ask for quotes", `v='a|b&c;d<e>f'; printf "[%s]" "${(q-)v}"`, `['a|b&c;d<e>f']`},
		{"so do the glob characters and the brackets", `v='a*b?c[d]e{f}g(h)'; printf "[%s]" "${(q-)v}"`, `['a*b?c[d]e{f}g(h)']`},
		{"a backslash, a dollar, a backquote and a double quote", "v='a\\b$c`d\"e'; printf \"[%s]\" \"${(q-)v}\"", "['a\\b$c`d\"e']"},
		{"a hash anywhere", `v=x#y; printf "[%s]" "${(q-)v}"`, "['x#y']"},
		{"a caret anywhere", `v=x^y; printf "[%s]" "${(q-)v}"`, "['x^y']"},
		{"but not a bang, a percent, a colon, a comma or a slash", `v='a!b%c:d,e/f'; printf "[%s]" "${(q-)v}"`, "[a!b%c:d,e/f]"},
		{"nor a plus, a minus, an at or an underscore", `v='a+b-c@d_e'; printf "[%s]" "${(q-)v}"`, "[a+b-c@d_e]"},
		{"nor a leading minus", `v=-x; printf "[%s]" "${(q-)v}"`, "[-x]"},
		// A multibyte rune passes through, and so does a byte that is not
		// part of one.
		{"a multibyte rune is not a reason to quote", `v=aéb; printf "[%s]" "${(q-)v}"`, "[aéb]"},
		{"though its neighbours may be", `v="aé b"; printf "[%s]" "${(q-)v}"`, "['aé b']"},
		// The two characters that are special only where a word starts.
		// These are the rows a table keyed on the character alone fails.
		{"a tilde needs quotes at the start", `v="~x"; printf "[%s]" "${(q-)v}"`, "['~x']"},
		{"and nowhere else", `v="x~y"; printf "[%s]" "${(q-)v}"`, "[x~y]"},
		{"an equals sign likewise", `v="=x"; printf "[%s][%s]" "${(q-)v}" "${(q-)v#=}"`, "['=x'][x]"},
		{"so an assignment comes back bare", `v="PATH=/x"; printf "[%s]" "${(q-)v}"`, "[PATH=/x]"},
		// The start is the *value's* start and not each run's, which only a
		// value with a quote before the tilde can say.
		{"a tilde a quote into the value needs nothing", `v="'~x"; printf "[%s]" "${(q-)v}"`, `[\'~x]`},
		{"where the same tilde at the front is quoted", `v="~'a"; printf "[%s]" "${(q-)v}"`, `['~'\'a]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, minimalQuoting, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The property the flag exists for, asserted as a property rather than as a
// string: whatever the value was, reading the answer back as shell gives the
// value again.
//
// This is the assertion that cannot be satisfied by agreeing with a
// hand-written expectation that is itself wrong. A table that quoted too
// little fails it on the metacharacters, one that escaped a quote inside
// single quotes fails it on the third value, and one that dropped the empty
// value's quotes fails it there.
func TestMinimalQuotingRoundTrips(t *testing.T) {
	values := []string{
		`plain`, `"with space"`, `"has'quote"`, `"a'b c"`, `"a'b'c"`, `"'"`,
		`""`, `"~"`, `"~x"`, `"x~y"`, `"=x"`, `"PATH=/x"`, `"#c"`, `"x#"`,
		`'a|b&c;d'`, `'a*b?c[d]'`, `'a$b'`, "'a`b\"c'", `'{e}(f)'`,
		`aéb`, `"aé b"`, `$'a\tb'`, `$'a\nb'`, `$'a\001b'`, `$'a\001 b'`,
		`"-x"`, `"a!b%c:d,e/f"`, `"' '"`,
	}
	src := "a=(" + strings.Join(values, " ") + `)
for v in "${a[@]}"; do
  eval "r=${(q-)v}"
  if [ "$r" = "$v" ]; then printf "."; else printf "[%s->%s]" "$v" "$r"; fi
done`
	out, st := runGrammar(t, src, minimalQuoting, nil)
	if want := strings.Repeat(".", len(values)); out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// Which `-` is the modifier and which is the sort flag.
//
// `-` is two flags sharing a spelling: the one a `q` in front of it eats, and
// the signed-numeric sort. This interpreter carries the first and refuses the
// second by name, so these rows are as much about the refusals as about the
// answers — a reading that carried every `-` would answer the refusing rows
// with minimal quoting at status 0, which is the wrong answer wearing a
// success.
//
// The adjacency and the count are both measured on zsh 5.9.2 with
// `b=(-1 -10 -3 2 10)`: `${(oq-)b}` sorts lexically and `${(oq--)b}` sorts
// signed, so the `-` beside the `q` is eaten and a further one is not.
func TestWhichMinusIsTheQuotingModifier(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"beside the q", `v="a b"; printf "[%s]" "${(q-)v}"`, "['a b']"},
		{"with a flag in front of it", `v="a b"; printf "[%s]" "${(@q-)v}"`, "['a b']"},
		{"and with one behind it", `v="a b"; printf "[%s]" "${(q-U)v}"`, "['A B']"},
		// zsh answers `${(-q-)v}` with minimal quoting, the first `-` being
		// a sort flag that does nothing to a scalar. Refused here, because
		// the sort flag is not carried and a group is refused for what is
		// in it rather than for what that would have come to on this value —
		// the same rule `(A)` is refused under.
		{
			"a sort flag in front of it is still refused", `v="a b"; printf "[%s]" "${(-q-)v}"`,
			"sh: ${(-q-)v}: the (-) expansion flag is not implemented\n",
		},
		{
			"before the q it is the other flag", `v="a b"; printf "[%s]" "${(-q)v}"`,
			"sh: ${(-q)v}: the (-) expansion flag is not implemented\n",
		},
		{
			"a flag letter between them separates them", `v="a b"; printf "[%s]" "${(qU-)v}"`,
			"sh: ${(qU-)v}: the (-) expansion flag is not implemented\n",
		},
		{
			"a doubled q does not take it", `v="a b"; printf "[%s]" "${(qq-)v}"`,
			"sh: ${(qq-)v}: the (-) expansion flag is not implemented\n",
		},
		{
			"and only one of two is taken", `v="a b"; printf "[%s]" "${(q--)v}"`,
			"sh: ${(q--)v}: the (-) expansion flag is not implemented\n",
		},
		{
			"a lone one is the sort flag", `a=(1 2); printf "[%s]" "${(o-)a}"`,
			"sh: ${(o-)a}: the (-) expansion flag is not implemented\n",
		},
		{
			"the extended form is refused by its own name", `v="a b"; printf "[%s]" "${(q+)v}"`,
			"sh: ${(q+)v}: the (+) expansion flag is not implemented\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, minimalQuoting, nil)
			if out != tc.want {
				t.Errorf("got %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}

// The two start-only specials reach the other two constructs that quote,
// because all three read one table.
//
// These rows were wrong before `q-` was built: `${(q)…}` and `${x:q}` escaped
// a tilde and an equals sign wherever they stood, so `a~b` came back `a\~b`
// and `PATH=/x` came back `PATH\=/x`. Both still read back as themselves,
// which is why the bug survived — it is a longer spelling and not a broken
// one, and only a comparison against the shell says so.
func TestTheQuotingTableIsShared(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the q flag leaves an interior tilde", `v="a~b"; printf "[%s]" "${(q)v}"`, "[a~b]"},
		{"and escapes a leading one", `v="~x"; printf "[%s]" "${(q)v}"`, `[\~x]`},
		{"it leaves an interior equals sign", `v="PATH=/x"; printf "[%s]" "${(q)v}"`, "[PATH=/x]"},
		{"and escapes a leading one", `v="=x"; printf "[%s]" "${(q)v}"`, `[\=x]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, minimalQuoting, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// And the third construct, which needs the substring range read as a modifier
// list before `:q` is a quoting at all — without that axis answered, `${v:q}`
// is an arithmetic offset and every row below passes for the wrong reason.
func TestTheQuotingModifierReadsTheSameTable(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an interior tilde is left alone", `v="a~b"; printf "[%s]" "${v:q}"`, "[a~b]"},
		{"a leading one is escaped", `v="~x"; printf "[%s]" "${v:q}"`, `[\~x]`},
		{"an interior equals sign is left alone", `v="x=y"; printf "[%s]" "${v:q}"`, "[x=y]"},
		{"a leading one is escaped", `v="=y"; printf "[%s]" "${v:q}"`, `[\=y]`},
		{"and a space is still escaped", `v="a b"; printf "[%s]" "${v:q}"`, `[a\ b]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, nil, func(r *Runner) {
				sem := *r.Semantics
				sem.SubstringRangeReadsModifiers = Yes
				r.Semantics = &sem
			})
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
