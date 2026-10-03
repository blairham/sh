// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"
	"sort"
	"strings"

	"github.com/blairham/sh/syntax"
)

// The expansion flags that decide which words survive and in what order:
// `u` keeps the first of each repeat, and `o`, `O`, `n`, `i` and `-` sort.
//
// They are one function because they are one step of the pipeline. Measured
// on zsh 5.9.2 under the C locale, which is what the corpus runs in:
//
//	a=(c a b);      ${(@o)a}   a b c        ascending
//	a=(c a b);      ${(@O)a}   c b a        descending
//	a=(10 9 1);     ${(@o)a}   1 10 9       lexical, so 10 before 9
//	a=(10 9 1);     ${(@n)a}   1 9 10       numeric
//	a=(x1 x10 x9);  ${(@n)a}   x1 x9 x10    and the digits need not be alone
//	a=(B a C b);    ${(@o)a}   B C a b      byte order, so case separates
//	a=(B a C b);    ${(@oi)a}  a B b C      case folded, and stable in a tie
//	a=(b a b c a);  ${(@u)a}   b a c        the first of each, order untouched
//	a=(c a b);      ${(@a)a}   c a b        the index, so the order it was written in
//	a=(c a b);      ${(@aO)a}  b a c        and that reversed
//
// `-` is `n` with a leading minus read as a sign rather than as text, and it
// is the one sort flag whose spelling is shared with something else: the
// character a `q` eats. The parser settles that, keeping the eaten one out of
// Flags, so a `-` reaching this file is always this flag. Measured on zsh
// 5.9.2, 2026-09-08, under LC_ALL=C with `b=(-1 -10 -3 2 10)`:
//
//	${(@o)b}    -1 -10 -3 10 2   lexical
//	${(@n)b}    -1 -3 -10 2 10   numeric, the `-` read as text
//	${(@-)b}    -10 -3 -1 2 10   numeric, the `-` read as a sign
//	${(@O-)b}   10 2 -1 -3 -10   and reversed
//	${(@n-)b}   -10 -3 -1 2 10   `-` implies `n` rather than modifying one
//	${(@a-)b}   -1 -10 -3 2 10   and `a` still beats it, as it beats `n`
//
// It is not a number *parse*, which three more rows say — same shell, same
// day:
//
//	e=(-1 -1.5);      ${(@-)e}   -1 -1.5      a decimal point is not part of
//	                                          the number, so -1.5 is `-1`
//	                                          then `.5` and sorts *after* -1
//	c=(+5 -5 5);      ${(@-)c}   +5 -5 5      a leading `+` is not a sign; it
//	                                          is compared as the byte it is,
//	                                          and 0x2b sorts ahead of 0x2d
//	m=(x-1 x-10 x-3); ${(@-)m}   x-10 x-3 x-1 and the sign need not stand at
//	                                          the word's start
//
// The last is the row that fixes where the reading goes: not a prefix test on
// the word, but the digit-run comparison itself, which is already the place
// `n` reads a run of digits as a number.
//
// Where the step sits is measured too, and it is not where the manual's rule
// numbers would put it by name. The sort runs *after* the operator —
// `a=(zb ya); ${(@o)a#z}` is `b ya`, which is trim-then-sort and not the
// other way — and *after* case conversion: `a=(B a); ${(@oU)a}` is `A B`,
// where sorting first would give `B A`. It runs after splitting, so
// `${(@s.,.o)v}` on `c,a,b` sorts the three fields; and after joining, so
// `${(oj.-.)a}` is `c-a-b` unsorted, one word being already in order.

// orderFlags are the letters this step answers.
// The `-` in here is the signed sort and never the `q` modifier, which
// the parser keeps out of Flags.
const orderFlags = "uoOnia-"

// orderWords applies them, in the one order that reproduces every
// composition measured: the repeats go first and the sort follows.
//
// `${(@u)a}` alone does not sort and `${(@ou)a}`, `${(@uo)a}`, `${(@uO)a}`
// and `${(@Ou)a}` all agree, so which of the two runs first is not
// observable when both are written — but it is when only `u` is, and that is
// what fixes the order here.
func (r *Runner) orderWords(e *syntax.ParamExpr, words []string) []string {
	signed := signedSortFlag(e)
	if strings.ContainsRune(e.Flags, 'u') {
		seen := make(map[string]bool, len(words))
		out := words[:0:0]
		for _, w := range words {
			if seen[w] {
				continue
			}
			seen[w] = true
			out = append(out, w)
		}
		words = out
	}
	if !strings.ContainsAny(e.Flags, "oOnia") && !signed {
		return words
	}
	descending := strings.ContainsRune(e.Flags, 'O')
	// `a` is the sort *key* rather than a sort: the element's own position,
	// so ascending is the order it was written in and descending is that
	// reversed. Which is why it does not fit beside the comparisons below —
	// it beats every one of them, in either spelling: `${(@oa)a}`,
	// `${(@na)a}` and `${(@ia)a}` on `(c a b)` are all `c a b`, and
	// `${(@aO)a}`, `${(@Oa)a}` and `${(@aOn)a}` are all `b a c`.
	if strings.ContainsRune(e.Flags, 'a') {
		if descending {
			slices.Reverse(words)
		}
		return words
	}
	// `i` sorts on its own: `a=(c a b); ${(@i)a}` is `a b c`, so it is not
	// only a modifier of `o`.
	fold := strings.ContainsRune(e.Flags, 'i')
	// `-` implies `n` rather than modifying one: `${(@-)b}` and `${(@n-)b}`
	// are the same answer, so the signed reading always arrives with the
	// numeric one and never on its own.
	numeric := strings.ContainsRune(e.Flags, 'n') || signed
	// Stable, because a fold makes ties reachable and they keep the order
	// they were written in: `${(@oi)a}` on `(B a C b)` is `a B b C`, with the
	// `B` still ahead of the `b`.
	key := func(w string) string { return w }
	if r.flagEmptiesAreGhosts(e, words) {
		key = nestedSortKey
	}
	sort.SliceStable(words, func(i, j int) bool {
		c := compareWords(key(words[i]), key(words[j]), fold, numeric, signed)
		if descending {
			return c > 0
		}
		return c < 0
	})
	return words
}

// nestedSortKey is what an element a nested expansion produced is sorted on:
// itself, except that an empty one sorts as the single byte 0xa1 rather than
// ahead of everything. Measured on zsh 5.9.2 under LC_ALL=C, 2026-10-01, with
// `b=($'\xff' "" x $'\xa0' $'\xa2')`:
//
//	"${(@o)${b[@]}}"     x \xa0 "" \xa2 \xff   the empty between 0xa0 and 0xa2
//	"${(@O)${b[@]}}"     \xff "" x          (on `($'\xff' "" x)`) and reversed
//	"${(@o)b}"           "" x y             (on `(x "" y)`) where not nested,
//	                                        the empty is first as it should be
//	c=("${(@)b}"); "${(@o)c}"               and first again once it is stored
//
// The same holds under `i` and `n`, and whether the inner is quoted or not:
// `"${(@oi)${b[@]}}"`, `"${(@on)${b[@]}}"` and `"${(@o)"${b[@]}"}"` all put
// the empty last on `("" x)` (#5312). So the key is the nesting, not the
// spelling of the inner.
//
// It is the empty a nested *parameter expansion* hands back, and nothing that
// rewrites the words on the way out keeps it — same shell, same day, on
// `b=(x "" y)`:
//
//	"${(@o)${b[@]}-z}"       x y ""     an operator that leaves the words alone
//	"${(@oU)${b[@]}}"        X Y ""     and a case flag, keep it last
//	"${(@o)${b[@]}#x}"       "" "" y    a pattern operator puts it first,
//	"${(@o)${b[@]}/x/w}"     "" w y     as `%`, `/` and `:#` do too,
//	"${(@os.:.)${:-x::y}}"   "" x y     and so does a split in the outer,
//	"${(@of)"$(print x)"}"              and a command substitution's empty
//	                                    lines were never the inner's elements
func nestedSortKey(w string) string {
	if w == "" {
		return ghostOneUnit
	}
	return w
}

// innerEmptiesSortLast reports whether nothing at e's own level — no split, no
// operator that rewrites the words — has turned its inner's ghosts into plain
// empty words. It is the half of Runner.nestedEmptiesAreGhosts that reads the
// node alone; see nestedSortKey for the measurements.
func innerEmptiesSortLast(e *syntax.ParamExpr) bool {
	// A command substitution's words reach a sort only through a split
	// flag, which the next test turns away, so a nested inner is enough.
	if e.Inner == nil {
		return false
	}
	if strings.ContainsAny(e.Flags, splitFlagLetters) || e.SplitFlags%2 == 1 {
		return false
	}
	switch e.Op {
	case syntax.ParamTrimPrefix, syntax.ParamTrimPrefixLong, syntax.ParamTrimSuffix,
		syntax.ParamTrimSuffixLong, syntax.ParamReplace, syntax.ParamExclude:
		return false
	}
	return true
}

// flagEmptiesAreGhosts reports whether the empty words a flag group's sort and
// pad read are ghosts: a nested list's (see nestedEmptiesAreGhosts), or the
// empty fields an `=` split in the group itself made. Measured 2026-10-02 on
// zsh 5.9.2 (`-f`, `LC_ALL=C`) with `IFS=:` and `u=a::b:`, quoted or not:
//
//	${(@o)=u}          a b "" ""    the empties sort last
//	${(@O)=u}          "" "" b a
//	${(@l:2:)=u}       " a" " " " b" " "   and pad one unit short
//	${(@o)=u#a}        b "" "" ""   an operator runs before the split, so
//	${(@l:2:)=u/a/x}   " x" " " " b" " "   what the split makes is still one
//	${(@os.:.)u}       "" "" a b    where a letter split's are not ghosts
//	e=; ${(@l:2:)=e}   "  "         and an empty value split is a value
//
// The last row is why words are asked: a value with nothing in it splits to
// the one empty word, and that word is the value rather than a field the
// split made — the same distinction `${(@q)=e}` draws by quoting it (#5426).
func (r *Runner) flagEmptiesAreGhosts(e *syntax.ParamExpr, words []string) bool {
	if e.SplitFlags%2 == 1 && !strings.ContainsAny(e.Flags, splitFlagLetters) {
		return len(words) != 1 || words[0] != ""
	}
	return r.nestedEmptiesAreGhosts(e)
}

// nestedEmptiesAreGhosts reports whether the empty words e's inner hands back
// are **ghosts**: empty when printed or matched by an operator, and one unit
// wide to everything that reads them before an operator does — the sort key
// (nestedSortKey), the padding (Runner.padAGhost), a length and a search
// subscript.
//
// Two things must hold. No operator or split at this level rewrote the words,
// which is innerEmptiesSortLast. And the inner is a *list* — an array's
// elements, or what a split made — rather than a value, or a nesting of such a
// list that kept its elements on the way up. Measured on zsh 5.9.2 under
// `LC_ALL=C`, 2026-10-02, with `b=(xyz "" y)` and `e=` (#5412):
//
//	"${#${b[@]}[2]}"          1     an array's empty element
//	"${(c)#${b[@]}}"          7     one character in a count of them too
//	"${(w)#${b[@]}}"          3     and one word
//	IFS=:; u=a::b; "${#${=u}[2]}"  1   an `=` split's empty field
//	"${#${(@)${b[@]}}[2]}"    1     a list kept through a level above
//	"${#${(@)b[2,2]}[1]}"     1     a range is a list of one
//	"${(@l:3:)${(@)b[2]}}"    `   ` a single element is a value — three blanks
//	"${(l:3:)${e}}"           `   ` as is a scalar
//	"${(c)#${${b[@]}}}"       6     and a level that joined its list
//	"${#${${b[@]}[2]}}"       0     or picked one element out of it
//
// Unquoted, a list's empty elements are gone before any of this reads them —
// see unquotedInnerDropsItsEmpties — and an `=` split's stay ghosts either way.
func (r *Runner) nestedEmptiesAreGhosts(e *syntax.ParamExpr) bool {
	if !innerEmptiesSortLast(e) {
		return false
	}
	if r.nestedElementsAreBare(e) != nestedBareNone {
		return true
	}
	span, _ := r.nestedInnerSpan(e)
	p := span.Param
	if span.Kind != syntax.ParamExp || p == nil || p.Inner == nil || p.Length || p.Indirect {
		return false
	}
	if joinFlagWritten(p.Flags) != 0 || (span.Quoting != syntax.Unquoted && !r.flagKeepsFields(p)) {
		// The level above joined its list into one value.
		return false
	}
	if p.Index != nil && !r.subscriptSelectsElements(p) {
		// It picked one element out of it, which is a value.
		return false
	}
	return r.nestedEmptiesAreGhosts(p)
}

// compareWords orders two words: by this shell's own order — shellOrder, and
// see it for what that is and is not — or by the numbers inside them when `n`
// was written, either with case folded away or not.
func compareWords(a, b string, fold, numeric, signed bool) int {
	if !numeric {
		if fold {
			return shellOrderFolded(a, b)
		}
		return shellOrder(a, b)
	}
	// No tie-break beside it, and that is measured rather than left out.
	// `(001 1 01)` comes back `001 01 1` — this shell's order and not the
	// order they were written in — and compareNatural is what answers that,
	// since two runs of equal value consume nothing and the byte at the
	// position decides. A `shellOrder` behind it therefore could only ever
	// fire on words the comparison above had already called equal, which is
	// where the fold is: measured 2026-09-26 on zsh 5.9.2, `${(oni)}` over
	// `F01z f1a F1A f01Z` is `F01z f01Z f1a F1A`, so a pair that differs only
	// in case keeps the order it was written in. That tie-break was here and
	// answered `F1A f1a` (#4555).
	return compareNatural(a, b, fold, signed)
}

// compareNatural compares two words with each run of digits read as a number.
//
// `x1 x9 x10` is the shape it exists for: the digits need not be the whole
// word, so a word is a sequence of digit and non-digit runs and the two are
// compared differently.
//
// **A run that ties consumes nothing**, and that is the part this had wrong
// until #4555. The walk used to step *past* two runs of equal value and carry
// on from whatever followed them, which reads as the obvious thing to do and
// is not what the reference does: measured on zsh 5.9.2
// (`/opt/homebrew/bin/zsh`, `-f`), 2026-09-26, `${(on)}` over `f01z f1a`
// answers `f01z f1a` where skipping the runs answers `f1a f01z`, because
// `01` and `1` are equal as numbers and `a` is before `z`. Falling straight
// through to the byte at the position the walk is standing on gives the
// reference's answer on that pair and on `h01y h1x`, `f01 f1`, `g001 g01 g1`
// and `0 00 000` alike.
//
// One consequence is worth naming, because it is what makes the rule easy to
// state: the two walks never diverge, so there is one index rather than two.
// Digits decide only where they *differ* as numbers; everything else is
// shellOrder's answer one character at a time.
//
// signed is the `-` flag: a `-` standing in front of a digit run, in *both*
// words at the same point, is that run's sign, and the comparison of the two
// runs is inverted. In one word only it is no sign at all and falls through
// to the character comparison — which is what keeps `-1` ahead of `-y`, and
// what makes the flag invisible outside a pair of negatives, every digit
// sorting above the `-` at 0x2d anyway. Measured in the same run: `${(on-)}`
// over `-10 -9 -01x -1 -1y -0 0 1` is that sequence, which the tie rule above
// is what puts `-01x` ahead of `-1` in.
func compareNatural(a, b string, fold, signed bool) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		switch {
		case signed && a[i] == '-' && b[i] == '-' &&
			i+1 < len(a) && i+1 < len(b) && isDigit(a[i+1]) && isDigit(b[i+1]):
			if c := compareDigitRuns(leadingDigits(a[i+1:]), leadingDigits(b[i+1:])); c != 0 {
				return -c
			}
		case isDigit(a[i]) && isDigit(b[i]):
			if c := compareDigitRuns(leadingDigits(a[i:]), leadingDigits(b[i:])); c != 0 {
				return c
			}
		}
		x, y := a[i], b[i]
		if fold {
			x, y = lowerByte(x), lowerByte(y)
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(a) > len(b):
		return 1
	case len(a) < len(b):
		return -1
	}
	return 0
}

// leadingDigits is the maximal run of digits at the front of s.
func leadingDigits(s string) string {
	i := 0
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	return s[:i]
}

// compareDigitRuns compares two runs of digits as numbers, without converting
// them: a run may be longer than any integer and a shell has no business
// refusing to sort it.
func compareDigitRuns(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

func lowerByte(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c - 'A' + 'a'
	}
	return c
}

// orderApplies reports whether this expansion asked for any of it, so the
// step is skipped rather than copying a slice for nothing.
func orderApplies(e *syntax.ParamExpr) bool {
	return strings.ContainsAny(e.Flags, orderFlags)
}
