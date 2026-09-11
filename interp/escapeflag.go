// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// The `(g:opts:)` expansion flag: the value's backslash escapes are read the
// way a shell's output builtins read them, with the letters between the
// delimiters saying which parts of the set are live.
//
// The reading itself is not here. It is a measurement about one shell's
// `echo` and `print`, that shell already has it for both of those, and a
// second copy in this package is a second place for the two to stop agreeing —
// so the dialect installs it and the substrate asks. See
// SetExpansionEscapes, and dialect/zsh/print.go for what is being reused.
//
// Where the flag stands in the pipeline is this package's question, and the
// vendor manual answers it: rule 13, "first any replacements from the `(g)`
// flag are performed, then any prompt-style formatting from the `(%)` family".
// Measured on zsh 5.9.2 2026-09-09, from both sides, since a rule list is a
// claim to check rather than a fact:
//
//	v='A\TB';  ${(Lg::)v}    a<TAB>b   the case conversion ran first
//	v='a\tb';  ${(Ug::)v}    A\TB      and it is the same answer written
//	v='a\tb';  ${(g::U)v}    A\TB      on either side of the `g`
//	v='a\tb';  ${(qg::)v}    a$'\t'b   the quoting ran after
//	v='a\x25\x25b'; ${(%g::)v}  a%b   and the prompt escapes after
//
// The first two rows are the discriminating pair. `(L)` on `A\TB` makes the
// `\T` a `\t` that the escape reading can then use, and `(U)` on `a\tb` makes
// a `\T` that it cannot — so a `g` that ran before the case conversion would
// answer the first row `A\TB` and the second `A<TAB>B`, both of them the
// opposite of what the shell says, and neither order of the letters changes it.
//
// The last row is the manual's half-step, and it discriminates too: `\x25` is
// a `%`, so the escape reading is what *makes* the doubled `%` the `(%)` flag
// then reduces. Reading them the other way round leaves `a%%b`, which is what
// `${(g::)v}` alone answers. It is asserted in the dialect's own package,
// because a runner nobody handed a prompt table has no `(%)` step to order
// against — see dialect/zsh/escapeflag_test.go.

// SetExpansionEscapes installs the escape reader the `(g)` expansion flag
// applies to a value, so `${(g::)v}` on `a\tb` is a real tab.
//
// opts is the flag's argument — the option letters between its delimiters,
// empty for the bare `${(g::)v}` — and the decoder answers what the value
// comes to when they are read that way. The letters themselves are the
// grammar's: the parser refuses any letter outside the set as an error in the
// flags, at the letter, so a decoder sees only letters it knows.
//
// The decoder is handed the runner, because the escape set is not a pure
// function of the text: a `\u` escape naming a code point the locale's
// encoding cannot hold is written back, refused, or written regardless,
// depending on the dialect *and* on the runner's own locale variables — see
// Runner.CodePointEscapeText. A decoder that could not reach one wrote the
// character in every locale there is (#2021).
//
// Nil is the runner nobody told, and there `(g)` is refused by name. That is
// deliberate, and it is the sharper case of the same rule `(p)` follows: a
// `(g)` read as a no-op is *right* for every value that has no backslash in
// it, so it would survive the first thing anyone tried it on and answer at
// status 0 wherever it mattered.
func (r *Runner) SetExpansionEscapes(decode func(r *Runner, text, opts string) string) {
	r.expansionEscapes = decode
}

// escapeFlagApplies reports whether this group asks for the escape reading.
//
// The letter alone decides. An empty argument is not "no options" in the sense
// of "no flag": `${(g::)v}` reads escapes with nothing turned on, which is the
// spelling the flag is nearly always written in, and it is the one a group
// with no `g` at all must not be confused with. Flags carries the letter and
// EscapeOpts carries only the letters of its argument, so the two are already
// apart — this function is where that is said once rather than at each use.
func escapeFlagApplies(e *syntax.ParamExpr) bool {
	return strings.ContainsRune(e.Flags, 'g')
}

// escapeFlagged reads the escapes in each of the words the pipeline has so
// far, as rule 13 asks.
//
// Each word, and no other text — which is enough to answer both halves,
// because the joins that could add text have already run by rule 13.
// Measured with `a=('x\ty' 'p\tq')` and `b=(m n)`:
//
//	set -- ${(g::)a}       two fields, a real tab in each
//	"${(g::j:\t:)b}"       m<TAB>n
//	"${(j:\t:)b}"          m\tn
//
// The second row is the one that pins it: a `j` separator carrying an escape
// is read too, because the quoted join at rule 5 has already put it in the
// word. So there is nothing here that says "the value and not the separator" —
// the order of the rules says it, and this loop is only the rule 13 half.
func (r *Runner) escapeFlagged(e *syntax.ParamExpr, words []string) []string {
	for i, w := range words {
		words[i] = r.expansionEscapes(r, w, e.EscapeOpts)
	}
	return words
}
