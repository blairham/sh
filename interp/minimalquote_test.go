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

// extendedQuotingSem is what reading a `q+` answer back needs: the `$'…'`
// escapes it writes. Without them the flag's whole property is unassertable
// — a spelling this shell cannot read is no quoting at all — which is why
// the two landed together.
func extendedQuotingSem() Semantics {
	s := CoreSemantics()
	s.DollarSingleCaretMeta = Yes
	s.DollarSingleNulTruncates = No
	return s
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

// The `q+` flag: extended minimal quoting.
//
// Every row is a measurement on zsh 5.9.2, taken 2026-09-08. What the rows
// are picked to separate is `q+` from `q-`, since the two agree on most
// values and a reading of either that passed the agreements would still be
// the wrong flag.
func TestTheExtendedQuotingFlag(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// Where the two agree, which is most of the table.
		{"a value needing nothing is bare", `v=plain; printf "[%s][%s]" "${(q+)v}" "${(q-)v}"`, "[plain][plain]"},
		{"and one needing quotes gets them", `v="a b"; printf "[%s][%s]" "${(q+)v}" "${(q-)v}"`, "['a b']['a b']"},
		{"an empty value is a pair of quotes", `v=""; printf "[%s]" "${(q+)v}"`, "['']"},
		{"and an empty element of an array is too", `a=(x "" "y z"); printf "[%s]" "${(@q+)a}"`, "[x]['']['y z']"},
		{"a value that is one quote", `v="'"; printf "[%s][%s]" "${(q+)v}" "${(q-)v}"`, `[\'][\']`},
		{"a bang is no reason to quote, in either", `v=a!b; printf "[%s][%s]" "${(q+)v}" "${(q-)v}"`, "[a!b][a!b]"},
		{"nor a leading minus", `v=-x; printf "[%s]" "${(q+)v}"`, "[-x]"},
		// The first difference: the quoting decision is over the whole value
		// and not per run, so a value with a quote in it is quoted
		// throughout. Neither run of `has'quote` needs quotes on its own.
		{"a quote quotes the whole value", `v="has'quote"; printf "[%s][%s]" "${(q+)v}" "${(q-)v}"`, `['has'\''quote'][has\'quote]`},
		{"including the runs that need nothing", `v="a'b c"; printf "[%s][%s]" "${(q+)v}" "${(q-)v}"`, `['a'\''b c'][a\''b c']`},
		{"an empty run is still written as nothing", `v="x'"; printf "[%s]" "${(q+)v}"`, `['x'\']`},
		{"at either end", `v="'x"; printf "[%s]" "${(q+)v}"`, `[\''x']`},
		{"or in the middle", `v="a''b"; printf "[%s]" "${(q+)v}"`, `['a'\'\''b']`},
		// The second: the two start-only specials are special wherever they
		// stand. These are the rows that separate the two tables.
		{"an interior tilde is quoted here", `v="a~b"; printf "[%s][%s]" "${(q+)v}" "${(q-)v}"`, "['a~b'][a~b]"},
		{"and an interior equals sign", `v="PATH=/x"; printf "[%s][%s]" "${(q+)v}" "${(q-)v}"`, "['PATH=/x'][PATH=/x]"},
		{"a leading one either way", `v="~x"; printf "[%s][%s]" "${(q+)v}" "${(q-)v}"`, "['~x']['~x']"},
		// The third: one byte that cannot be written as itself moves the
		// whole value into `$'…'`, and the vocabulary is the caret one
		// rather than `qqqq`'s octal.
		{"a control byte renders the value", "v=$'a\\001b'; printf \"[%s][%s]\" \"${(q+)v}\" \"${(qqqq)v}\"", `[$'a\C-Ab'][$'a\001b']`},
		{"a tab, where q- quotes the byte", "v=$'a\\tb'; printf \"[%s][%s]\" \"${(q+)v}\" \"${(q-)v}\"", "[$'a\\tb']['a\tb']"},
		{"a newline likewise", "v=$'a\\nb'; printf \"[%s]\" \"${(q+)v}\"", "[$'a\\nb']"},
		{"delete", "v=$'a\\177b'; printf \"[%s]\" \"${(q+)v}\"", `[$'a\C-?b']`},
		{"escape, which has no name in this vocabulary", "v=$'a\\033b'; printf \"[%s]\" \"${(q+)v}\"", `[$'a\C-[b']`},
		{"the last control byte", "v=$'a\\037b'; printf \"[%s]\" \"${(q+)v}\"", `[$'a\C-_b']`},
		{"a byte that is no rune takes the meta prefix", "v=$'a\\377b'; printf \"[%s]\" \"${(q+)v}\"", `[$'a\M-\C-?b']`},
		{"the low end of it", "v=$'a\\200b'; printf \"[%s]\" \"${(q+)v}\"", `[$'a\M-\C-@b']`},
		{"where the low seven bits are printable", "v=$'a\\240b'; printf \"[%s]\" \"${(q+)v}\"", `[$'a\M- b']`},
		{"or a named escape", "v=$'a\\211b'; printf \"[%s]\" \"${(q+)v}\"", `[$'a\M-\tb']`},
		// Inside the rendering, the quotes cannot hold a quote or a
		// backslash — and a bang needs no escape, where `qqqq` writes one.
		{"a quote inside the rendering", "v=$'a\\001b'\\''c'; printf \"[%s]\" \"${(q+)v}\"", `[$'a\C-Ab\'c']`},
		{"a backslash inside it", "v=$'a\\001b\\\\c'; printf \"[%s]\" \"${(q+)v}\"", `[$'a\C-Ab\\c']`},
		{"a bang inside it is not escaped", "v=$'a\\001b!c'; printf \"[%s]\" \"${(q+)v}\"", `[$'a\C-Ab!c']`},
		{"nor is a space", "v=$'a\\001b c'; printf \"[%s]\" \"${(q+)v}\"", `[$'a\C-Ab c']`},
		// A rune passes through, and only its neighbours ask for quotes.
		{"a multibyte rune is not a reason to render", `v=aéb; printf "[%s]" "${(q+)v}"`, "[aéb]"},
		{"though its neighbours may quote", `v="aé b"; printf "[%s]" "${(q+)v}"`, "['aé b']"},
		// The join runs first and the quoting wraps what it produced, which
		// is why a marked separator is refused beside this flag.
		{"a join is quoted once, not per element", `d=('a b' c); printf "[%s]" "${(q+j.|.)d}"`, "['a b|c']"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, minimalQuoting, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The property `q+` exists for, asserted as a property rather than as a
// string: whatever the value was, reading the answer back as shell gives the
// value again.
//
// It is a stronger claim than `q-`'s round trip, because the answer may be a
// `$'…'` and reading one back needs the caret and meta escapes — so this
// test fails outright if the writer and the reader disagree by a single
// byte, which no hand-written expectation would catch.
//
// Two byte values are left out, and the reason is a measurement rather than
// a convenience: 0xa7 and 0xdc are spelled `$'\M-”` and `$'\M-\'` by the
// shell being modeled, each ending the quoting early, so the answers do not
// read back *there* either. They are reproduced — see
// quoteExtendedRendered — and pinned by the corpus rather than by a
// property they cannot satisfy.
func TestExtendedQuotingRoundTrips(t *testing.T) {
	values := []string{
		`plain`, `"with space"`, `"has'quote"`, `"a'b c"`, `"a'b'c"`, `"'"`,
		`"''"`, `"x'"`, `"'x"`, `""`, `"~"`, `"~x"`, `"x~y"`, `"=x"`,
		`"PATH=/x"`, `"#c"`, `'a|b&c;d'`, `'a*b?c[d]'`, `'a$b'`,
		"'a`b\"c'", `'{e}(f)'`, `aéb`, `"aé b"`, `"-x"`, `"a!b%c:d,e/f"`,
		`$'a\tb'`, `$'a\nb'`, `$'a\001b'`, `$'a\001 b'`, `$'a\177b'`,
		`$'a\033b'`, `$'a\200b'`, `$'a\240b'`, `$'a\211b'`, `$'a\377b'`,
		`$'a\001b\'c'`, `$'a\001b\\c'`, `$'\001'`, `$'\377'`,
	}
	src := "a=(" + strings.Join(values, " ") + `)
for v in "${a[@]}"; do
  eval "r=${(q+)v}"
  if [ "$r" = "$v" ]; then printf "."; else printf "[%s]" "${(q+)v}"; fi
done`
	out, st := runGrammar(t, src, minimalQuoting, withSem(extendedQuotingSem()))
	if want := strings.Repeat(".", len(values)); out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// Which `-` is the modifier and which is the sort flag, and where a `+` may
// stand at all.
//
// `-` is two flags sharing a spelling: the one a `q` in front of it eats, and
// the signed-numeric sort. `+` is one flag and one error: the same modifier
// slot, and nothing at all anywhere else. The parser settles both readings —
// see syntax.scanParamFlags — so these rows are as much about which
// diagnostic arrives as about which answer does. A reading that took every
// `-` as a modifier answers the sort rows with quoting at status 0, and one
// that took none answers the quoting rows with a lexical sort; both are the
// wrong answer wearing a success.
//
// Measured on zsh 5.9.2, 2026-09-08, under LC_ALL=C.
func TestWhichMinusIsTheQuotingModifier(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"beside the q", `v="a b"; printf "[%s]" "${(q-)v}"`, "['a b']"},
		{"with a flag in front of it", `v="a b"; printf "[%s]" "${(@q-)v}"`, "['a b']"},
		{"and with one behind it", `v="a b"; printf "[%s]" "${(q-U)v}"`, "['A B']"},
		// The adjacency is literal and not associative: a letter between the
		// two leaves a plain `q` and a sort flag, which on a scalar sorts
		// nothing and is visible only in the quoting style.
		{"a flag letter between them separates them", `v="a b"; printf "[%s]" "${(qU-)v}"`, `[A\ B]`},
		{"before the q it is the sort flag", `v="a b"; printf "[%s]" "${(-q)v}"`, `[a\ b]`},
		{"and one of each is both", `v="a b"; printf "[%s]" "${(-q-)v}"`, "['a b']"},
		{"only the first of two is eaten", `v="a b"; printf "[%s]" "${(q--)v}"`, "['a b']"},
		// The rows where the eaten `-` and the sort flag are told apart by
		// what the sort *does*, which needs an array whose two orders differ.
		{"a lone one sorts signed", `b=(-1 -10 -3 2 10); printf "[%s]" "${(@o-)b}"`, "[-10][-3][-1][2][10]"},
		{"a q in front of it takes it, so the sort is lexical", `b=(-1 -10 -3 2 10); printf "[%s]" "${(@oq-)b}"`, `[-1][-10][-3][10][2]`},
		// Both readings at once, on an array where each is visible: the
		// `a b` is quoted, so the `q-` fired, and the negatives are in
		// signed order, so the second `-` reached the sort. A hand-written
		// expectation got this row wrong first — `-10` needs no quotes, and
		// a value set of numbers alone would have shown only half of it.
		{"and the second one is the sort flag again", `c=(-1 "a b" -10); printf "[%s]" "${(@oq--)c}"`, `['a b'][-10][-1]`},
		{"a q+ takes the slot, freeing the next one", `b=(-1 -10 -3 2 10); printf "[%s]" "${(@oq+-)b}"`, `[-10][-3][-1][2][10]`},
		// `+` in the modifier slot, and the four places it is not one. Each
		// of these is an error in the flags on that shell rather than an
		// unimplemented flag, and the position is the measurement.
		{"the extended form beside the q", `v="a b"; printf "[%s]" "${(q+)v}"`, "['a b']"},
		{
			"a plus on its own is no flag", `v="a b"; printf "[%s]" "${(+)v}"`,
			"sh: error in flags near position 4 in '${(+)v}'\n",
		},
		{
			"nor is one behind another letter", `v="a b"; printf "[%s]" "${(U+)v}"`,
			"sh: error in flags near position 5 in '${(U+)v}'\n",
		},
		{
			"nor one behind the modifier", `v="a b"; printf "[%s]" "${(q-+)v}"`,
			"sh: error in flags near position 6 in '${(q-+)v}'\n",
		},
		{
			"nor one behind a q that is not the first", `v="a b"; printf "[%s]" "${(qq+)v}"`,
			"sh: error in flags near position 5 in '${(qq+)v}'\n",
		},
		// A doubled `q` takes neither modifier, and the position reported is
		// the second `q`'s rather than the modifier's — measured, not
		// derived.
		{
			"a doubled q does not take a minus", `v="a b"; printf "[%s]" "${(qq-)v}"`,
			"sh: error in flags near position 5 in '${(qq-)v}'\n",
		},
		{
			"wherever in the group it stands", `v="a b"; printf "[%s]" "${(qoq-)v}"`,
			"sh: error in flags near position 6 in '${(qoq-)v}'\n",
		},
		// And a group that took a `q-` takes no further `q`, adjacent or not.
		{
			"a q- group takes no second q", `v="a b"; printf "[%s]" "${(q-q)v}"`,
			"sh: error in flags near position 6 in '${(q-q)v}'\n",
		},
		{
			"at any distance", `v="a b"; printf "[%s]" "${(q-Uq)v}"`,
			"sh: error in flags near position 7 in '${(q-Uq)v}'\n",
		},
		// Where `q+` is the asymmetry: that shell reads the group and
		// answers it with a spelling that does not read back, so it is
		// refused by name rather than reproduced. See extendedQuoteRefusal.
		{
			"a q+ group is read with a second q, and refused", `v="a b"; printf "[%s]" "${(q+q)v}"`,
			"sh: ${(q+q)v}: the (q+) expansion flag is not implemented beside a further (q)\n",
		},
		{
			"at any distance too", `v="a b"; printf "[%s]" "${(q+Uq)v}"`,
			"sh: ${(q+Uq)v}: the (q+) expansion flag is not implemented beside a further (q)\n",
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
