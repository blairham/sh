// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// A subscript's own flag group — `${a[(re)value]}` — selects an element by
// *searching* rather than by counting. One grammar in the panel has it
// (syntax.Dialect.ArraySubscriptFlags), so what the letters mean here is that
// shell's answer, measured with the binary and recorded in
// docs/spec/grammar/parameter-expansion.md.
//
// Four letters do the selecting and they are mutually exclusive — the last
// one written wins, measured: `(ri)` is an index and `(ir)` an element:
//
//	(r)  the first element the operand matches, or nothing
//	(R)  the last such element
//	(i)  the index of the first match, or one past the last element
//	(I)  the index of the last match, or one before the first
//
// and three modify the search: `(e)` makes the operand a literal string
// rather than a pattern, `(n:expr:)` asks for the expr'th match rather than
// the first, and `(b:expr:)` moves where the search starts.
//
// That is the reading over an *ordered* array. Over an association the same
// four letters are a different construct — see searchAssoc, where the case of
// the letter is how many matches come back rather than which end the search
// started from, and two of the three modifiers are ignored.
//
// A group with *no* selecting letter changes nothing about the reading:
// `${a[()2]}` and `${a[(e)2]}` are both the second element, measured. That is
// why this reports whether it handled the subscript rather than answering
// every flagged one — the ordinary reading is still the right one for a group
// that only says how a search it did not ask for would have run.

// implementedSubscriptFlags are the letters this carries. The other six the
// grammar accepts are refused *by name* when the subscript is reached, for
// the reason implementedParamFlags gives: the only thing worse than refusing
// a flag is answering it wrong at status 0, and a subscript flag's wrong
// answer is a plausible element rather than a visible failure.
const implementedSubscriptFlags = "rRiIenb"

// searchSubscriptFlags are the four that select. Written in no particular
// order; only which of them came last matters.
const searchSubscriptFlags = "rRiI"

// flaggedSubscript answers a subscript that carries a flag group, reporting
// whether it answered at all.
func (r *Runner) flaggedSubscript(e *syntax.ParamExpr) ([]string, bool) {
	search, ok := r.subscriptSearch(e)
	if !ok {
		return nil, true
	}
	if search == 0 {
		// Nothing to select by, so the operand is an ordinary subscript.
		return nil, false
	}
	if a, isAssoc := r.assocFor(e.Name); isAssoc {
		return r.searchAssoc(e, a, e.IndexFlags, search), true
	}
	elems, scalar, held := r.subscriptTarget(e)
	if !held {
		// A name holding nothing is searched and found to hold nothing:
		// measured, `${nosucharray[(i)x]}` is empty rather than the
		// one-past-the-end an *empty* array answers with.
		return nil, true
	}
	return r.searchSubscript(e, search, subscriptSource{name: e.Name, elems: elems, scalar: scalar})
}

// subscriptSearch reads the flag group and says which letter selects, having
// refused by name any letter this does not carry.
//
// A zero letter with ok is a group that says only how a search it never asked
// for would have run — `${a[(e)2]}` is the second element — and its caller
// falls back to the ordinary reading.
func (r *Runner) subscriptSearch(e *syntax.ParamExpr) (search byte, ok bool) {
	g := e.IndexFlags
	for _, c := range g.Flags {
		if !strings.ContainsRune(implementedSubscriptFlags, c) {
			r.refuseSubscriptFlag(e, string(c), "")
			return 0, false
		}
	}
	return lastOf(g.Flags, searchSubscriptFlags), true
}

// searchSubscript answers a search subscript against values already in hand,
// so that a name's elements and an expansion's result are searched by the
// same code. See subscriptSource.
func (r *Runner) searchSubscript(e *syntax.ParamExpr, search byte, src subscriptSource) ([]string, bool) {
	g := e.IndexFlags
	elems, scalar := src.elems, src.scalar
	if scalar {
		return r.searchScalar(g, search, elems[0]), true
	}
	at, found := r.searchElements(g, search, elems)
	// The index is the base plus the element's *position*, which is the same
	// thing only while an array has no gaps. It has none in the grammar that
	// has this construct — measured, `a=(x); a[5]=y` there leaves five
	// elements and `${a[(i)y]}` is 5 — and a grammar that kept its gaps would
	// need the stored subscript rather than the position. Written down here
	// because nothing in this file would notice the difference.
	base := r.arrayBase()
	switch search {
	case 'r', 'R':
		if !found {
			return nil, true
		}
		return []string{elems[at]}, true
	case 'i':
		// One past the last element when nothing matched, which is what
		// makes `a[(i)new]=v` an append in the shell that has the construct.
		if !found {
			return []string{itoa(base + len(elems))}, true
		}
	default: // 'I'
		// One before the first, which is the index no element has.
		if !found {
			return []string{itoa(base - 1)}, true
		}
	}
	return []string{itoa(base + at)}, true
}

// searchElements walks the elements the way the group asks and returns the
// position of the match it wanted, 0-based.
func (r *Runner) searchElements(g *syntax.SubscriptFlags, search byte, elems []string) (at int, found bool) {
	matches := r.subscriptMatcher(g, false)
	back := search == 'R' || search == 'I'
	from, within := r.searchStart(g, len(elems), back)
	if !within {
		// A start outside the array is not clamped to its end: measured,
		// with five elements `${a[(Ib:6:)*a]}` is 0 and `${a[(ib:6:)*a]}` is
		// 6, so neither direction searches at all.
		return 0, false
	}
	want := r.searchNth(g)
	step := 1
	if back {
		step = -1
	}
	for i := from; i >= 0 && i < len(elems); i += step {
		if !matches(elems[i]) {
			continue
		}
		if want--; want == 0 {
			return i, true
		}
	}
	return 0, false
}

// searchScalar answers a search subscript over a plain string, where what the
// four letters count through is the string's *characters*.
//
// Measured on zsh 5.9.2, the one shell with the construct, with
// `s="hello world"`:
//
//	${s[(i)l]}     3    the position of the first match
//	${s[(I)l]}     10   and of the last
//	${s[(r)l]}     l    which position `r` reads the character at
//	${s[(R)[hd]]}  d    so `r` and `R` part where the characters do
//	${s[(i)wor]}   7    the operand matches a *substring*, not one character
//	${s[(r)wor]}   w    and what `r` answers is still one character
//
// So it is not the array's search over the one-element list a scalar is
// otherwise read as — that would make `${s[(r)wor]}` the whole string — and
// not a search over each character on its own either, which would make every
// multi-character operand a miss. It is the array's walk with a *prefix*
// match at each position, and `r` and `R` are the index that walk found read
// as an ordinary subscript: `${s[(r)z]}` and `${s[(R)z]}` are both empty
// because the misses are `${s[12]}` and `${s[0]}`, and neither of those is a
// character either.
//
// The miss indices are therefore the array's, one past the last and one
// before the first, and `(e)`, `(n:expr:)` and `(b:expr:)` are read here as
// they are there: measured, `${s[(ie)lo]}` is 4, `${s[(in:2:)l]}` is 4 and
// `${s[(ib:5:)l]}` is 10.
//
// The character is the locale's rather than a byte, because r.units is:
// measured under a UTF-8 locale `s="héllo"; ${s[(i)l]}` is 3 there and
// under `LC_ALL=C` it is 4.
func (r *Runner) searchScalar(g *syntax.SubscriptFlags, search byte, v string) []string {
	chars := r.units(v)
	at := r.scalarSearchIndex(g, search, chars, v)
	if search == 'i' || search == 'I' {
		return []string{itoa(at)}
	}
	pos := at - r.arrayBase()
	if pos < 0 || pos >= len(chars) {
		return nil
	}
	return []string{chars[pos]}
}

// scalarSearchIndex is the character position the search names, counted from
// the dialect's base, whether or not anything matched.
//
// One position past the last character is walked, which the walk over an
// array's elements has no equivalent of because only an empty match can land
// there: measured, `${s[(I)*]}` on eleven characters is 12 where
// `${s[(I)?]}` is 11.
//
// An *empty* string is the exception to all of it, and is measured rather
// than derived: `e=; ${e[(i)x]}`, `${e[(I)x]}` and `${e[(i)*]}` are every one
// of them 0, where the rule above makes the first and the third 1. A name
// holding nothing at all is a different answer again — empty rather than a
// number — and flaggedSubscript answers that one before this is reached.
func (r *Runner) scalarSearchIndex(g *syntax.SubscriptFlags, search byte, chars []string, v string) int {
	base, n := r.arrayBase(), len(chars)
	if n == 0 {
		return base - 1
	}
	back := search == 'R' || search == 'I'
	miss := base + n
	if back {
		miss = base - 1
	}
	from, within := r.scalarSearchStart(g, n, back)
	if !within {
		return miss
	}
	// Byte offsets rather than a join per position, so the remainder handed to
	// the matcher is a slice of the value itself: one match call per position
	// and nothing copied.
	offs := make([]int, n+1)
	for i, c := range chars {
		offs[i+1] = offs[i] + len(c)
	}
	matches := r.subscriptMatcher(g, true)
	want := r.searchNth(g)
	step := 1
	if back {
		step = -1
	}
	for i := from; i >= 0 && i <= n; i += step {
		if !matches(v[offs[i]:]) {
			continue
		}
		if want--; want == 0 {
			return base + i
		}
	}
	return miss
}

// scalarSearchStart is searchStart with the one difference a walk over
// characters has: a backward search that was not told where to begin begins
// one *past* the last character, where an empty match can still land.
//
// Measured, `${s[(I)*]}` on eleven characters is 12. `(b:expr:)` does not
// reach that position — `${s[(Ib:12:)*]}` is 0 — so a start named explicitly
// is one of the characters or nowhere at all, which is what searchStart
// already says.
func (r *Runner) scalarSearchStart(g *syntax.SubscriptFlags, n int, back bool) (from int, within bool) {
	if g.Begin == "" && back {
		return n, true
	}
	return r.searchStart(g, n, back)
}

// subscriptMatcher is the test one element has to pass, built once for the
// whole walk.
//
// prefix asks the other question the same operand answers: whether it matches
// at the *start* of what it is handed rather than the whole of it, which is
// the question a search over a string's characters asks — see searchScalar.
// One function rather than two because everything below this line is the
// operand's rule and not the subject's, and a second copy of it is how the
// quoting rule the next paragraph describes would come to hold in one search
// and not the other.
//
// The operand is a pattern unless `(e)` says otherwise, and in either case it
// is the subscript's text *as written* with its substitutions performed —
// there is no quoting inside a subscript at all. Measured, three ways:
// `${a[(r)"beta"]}` finds an element whose value is the six characters
// `"beta"` and not the four; `${a[(r)"$h"]}` with `h=beta` finds that same
// six-character element, so the quotes stayed while the value was
// substituted; and `${a[(re)"beta"]}` does the same under exact matching, so
// this is the operand's rule rather than the pattern's.
//
// The consequence for the pattern half is that a substituted value's
// metacharacters are simply live, with nothing to override: `g='be*';
// ${a[(r)$g]}` finds `beta` in the shell that has the construct while
// `${(@)a:#$g}` in the same shell removes nothing, and the difference is that
// only one of the two is a quoting context.
func (r *Runner) subscriptMatcher(g *syntax.SubscriptFlags, prefix bool) func(string) bool {
	operand := r.searchOperand(g.Arg)
	if strings.ContainsRune(g.Flags, 'e') {
		if prefix {
			return func(el string) bool { return strings.HasPrefix(el, operand) }
		}
		return func(el string) bool { return el == operand }
	}
	if prefix {
		// A pattern matching a prefix of `s` is that pattern with a `*` after
		// it matching the whole of `s`, which is one match call per position
		// rather than one per position and length. Measured to be the same
		// reading: `${s[(i)l*]}` is 3 and `${s[(I)l*]}` is 10 on
		// `hello world`, which is where `l` itself is found rather than where
		// a longest match would end.
		operand += "*"
	}
	return func(el string) bool { return r.matchPatternR(operand, el, false) }
}

// searchOperand renders a subscript search's operand: every substitution
// performed, and every other character — quotes and backslashes included —
// left exactly where it was written.
//
// The quote characters are put back because the *lexer* took them off. It
// reads the operand as it reads any word, which is right for the ordinary
// subscript behind a flag group and wrong for the search in front of it, and
// a word is the only shape that carries where a substitution is.
func (r *Runner) searchOperand(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	var b strings.Builder
	open := syntax.Unquoted
	quote := func(q syntax.Quoting) string {
		switch q {
		case syntax.SingleQuoted:
			return "'"
		case syntax.DoubleQuoted:
			return `"`
		}
		return ""
	}
	for _, s := range w.Spans {
		// A run of spans inside one pair of quotes is one pair of quotes:
		// `"ab$h"` is `"ab` plus the value plus `"`, not a pair around each
		// of its three pieces.
		q := s.Quoting
		if q == syntax.BackslashQuoted || q == syntax.DollarSingleQuoted {
			q = syntax.Unquoted
		}
		if q != open {
			b.WriteString(quote(open))
			b.WriteString(quote(q))
			open = q
		}
		switch s.Quoting {
		case syntax.BackslashQuoted:
			// One character each, so this cannot use the run above: `\*\?`
			// is two backslashes and not one.
			b.WriteString(`\` + s.Value)
			continue
		case syntax.DollarSingleQuoted:
			b.WriteString("$'" + s.Value + "'")
			continue
		}
		switch s.Kind {
		case syntax.ParamExp:
			b.WriteString(r.expandParam(s.Param))
		case syntax.CommandSubst:
			b.WriteString(r.commandSubst(r.ctx, s))
		case syntax.ArithSubst:
			v, ok := r.arithSpanValue(s)
			if !ok {
				return ""
			}
			b.WriteString(v)
		default:
			b.WriteString(s.Value)
		}
	}
	b.WriteString(quote(open))
	return b.String()
}

// searchStart is the 0-based element the walk begins at, and whether that is
// an element at all.
//
// `(b:expr:)` names it in the array's own base, a negative one counting back
// from the end — measured, with five elements `b:-1:` is the fifth and
// `b:-5:` the first. Without the flag the walk begins at whichever end the
// direction implies.
func (r *Runner) searchStart(g *syntax.SubscriptFlags, n int, back bool) (from int, within bool) {
	if g.Begin == "" {
		if back {
			return n - 1, n > 0
		}
		return 0, n > 0
	}
	begin, ok := r.subscriptIndex(g.Begin)
	if !ok {
		return 0, false
	}
	base := r.arrayBase()
	if begin < 0 {
		// The same counting an ordinary negative subscript does, so `b:-1:`
		// and `${a[-1]}` name one element.
		from = n + begin
	} else {
		from = begin - base
		if from < 0 {
			// Measured: `b:0:` where the base is 1 starts at the first
			// element rather than nowhere.
			from = 0
		}
	}
	return from, from >= 0 && from < n
}

// searchNth is which match the group asked for, counting from 1.
//
// `(n:expr:)` is an arithmetic expression and not a numeral — measured,
// `(rn:1+1:)` is the second match — and anything below one is one, which is
// also measured: `(rn:0:)` is the first.
func (r *Runner) searchNth(g *syntax.SubscriptFlags) int {
	if g.Nth == "" {
		return 1
	}
	nth, ok := r.subscriptIndex(g.Nth)
	if !ok || nth < 1 {
		return 1
	}
	return nth
}

// searchAssoc answers a flag group's search over an associative array, which
// is a different construct from the ordered array's search wearing the same
// four letters. Measured against zsh 5.9.2 with `m=(a 1 b 2)`:
//
//	${m[(i)a]}   a       the first matching *key*, not an index
//	${m[(I)*]}   a b     *every* matching key, not the last one
//	${m[(r)2]}   2       the first value whose *value* matched
//	${m[(R)*]}   1 2     every such value
//
// So the case of the letter is the count rather than the direction — there is
// no "last match" here to be the mirror of a first — and which half of the
// pair is searched is the letter itself: `i` and `I` read the keys, `r` and
// `R` the values. `${m[(i)zzz]}` and `${m[(I)zzz]}` are both nothing at all
// rather than the out-of-range index an ordered array answers with, and that
// nothing is a *set* empty list: measured, `${m[(I)zz]-none}` is empty where
// `${m[zz]-none}` is `none`, so the search always has an answer even when the
// answer is no keys.
//
// Two of the modifiers the ordered array's search reads are *ignored* here,
// which is measured rather than assumed and is why searchStart and searchNth
// are not called: with three matching keys `${m[(in:3:)a*]}` and
// `${m[(ib:2:)a*]}` are both the first of them, so neither `(n:expr:)` nor
// `(b:expr:)` moves the search. `(e)` is read, through the same matcher the
// ordered search uses: `${m[(Ie)a*]}` finds the key spelled `a*` and not the
// key `aa`.
//
// The order the matches come back in is the order the table lists its keys —
// zsh's is its hash's, ours is sorted, and the two agree on the *invariant*
// that `${m[(I)*]}` is `${(k)m}` filtered rather than on any particular
// sequence. AssocArray.keys() says why a deterministic order is worth having
// where the shells promise none.
func (r *Runner) searchAssoc(e *syntax.ParamExpr, a AssocArray, g *syntax.SubscriptFlags, search byte) []string {
	matches := r.subscriptMatcher(g, false)
	byKey := search == 'i' || search == 'I'
	every := search == 'I' || search == 'R'
	found := make([]string, 0, len(a))
	for _, k := range a.keys() {
		subject := a[k]
		if byKey {
			subject = k
		}
		if !matches(subject) {
			continue
		}
		found = append(found, k)
		if !every {
			break
		}
	}
	return assocSearchWords(e, a, found, byKey)
}

// assocSearchWords is which half of each matched pair the expansion asked
// for, which the *expansion's* own flag group decides where it wrote one.
//
// Measured, and the same answer whichever letter did the searching:
// `${(k)m[(R)*]}` is the keys, `${(v)m[(i)*]}` the value of the one match,
// and `${(kv)m[(I)*]}` key and value as two consecutive words each. Without
// either letter the search's own half is what comes back — keys for `i` and
// `I`, values for `r` and `R` — which is the rule the four letters carry on
// their own.
func assocSearchWords(e *syntax.ParamExpr, a AssocArray, keys []string, byKey bool) []string {
	hasK := e.HasFlags && strings.ContainsRune(e.Flags, 'k')
	hasV := e.HasFlags && strings.ContainsRune(e.Flags, 'v')
	out := make([]string, 0, 2*len(keys))
	for _, k := range keys {
		switch {
		case hasK && hasV:
			out = append(out, k, a[k])
		case hasK:
			out = append(out, k)
		case hasV || !byKey:
			out = append(out, a[k])
		default:
			out = append(out, k)
		}
	}
	return out
}

// assocSearchSubscript reports whether this subscript is a flag group
// searching an associative array, which is the one subscript that names
// *several* elements without being written `[@]` or `[*]` or as a range.
//
// Three readings ask, and they have to agree or the same expansion is a list
// in one and a string in another: `${#m[(I)*]}` is the match count,
// `"${m[(I)*]}"` is one field with the matches joined, and `${(on)m[(I)*]}`
// hands the flag group a list to sort. The third is the shape a real plugin
// manager writes, twenty-six times.
func (r *Runner) assocSearchSubscript(e *syntax.ParamExpr) bool {
	if e.IndexFlags == nil {
		return false
	}
	if lastOf(e.IndexFlags.Flags, searchSubscriptFlags) == 0 {
		return false
	}
	_, isAssoc := r.assocFor(e.Name)
	return isAssoc
}

// refuseSubscriptFlag says which flag was not carried, naming the subscript
// as it was written.
func (r *Runner) refuseSubscriptFlag(e *syntax.ParamExpr, flag, where string) {
	r.diagf("${%s}: the (%s) subscript flag is not implemented%s\n",
		r.paramSubject(e), flag, where)
	r.expandErr = true
}

// lastOf is the last character of s that is in set, or 0 for none.
func lastOf(s, set string) byte {
	last := byte(0)
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(set, s[i]) >= 0 {
			last = s[i]
		}
	}
	return last
}

// flaggedAssignIndex is the element `a[(r)y]=Q` names, as a subscript.
//
// The read side answers a search with the element's *value* for `r` and `R`
// and its *index* for `i` and `I`; an assignment wants the index either way,
// which is the whole of the difference and the reason this is not
// flaggedSubscript with a flag on it.
//
// Measured 2026-09-07 on zsh 5.9.2, which is the one shell with the
// construct, with `b=(x y z)`:
//
//	b[(r)y]=Q          x Q z    the matched element is replaced
//	b[(re)y]=Q         x Q z    and exact matching selects the same one
//	b[(R)x]=Q          on (x y x): x y Q — the reverse search takes the last
//	b[(i)y]=W          x W z
//	b[(I)y]=W          x W z
//	b[(i)nomatch]=W    x y z W  one past the last element, so it appends
//	b[(r)y]+=Q         x yQ z   the element joined rather than replaced
//
// A search that finds nothing under `r` or `R` writes nowhere at all — those
// two answer with an element and there is none — where `i` answers the index
// after the last and `I` the one before the first. `I` missing is the index
// no element has, and writing there is refused rather than guessed.
func (r *Runner) flaggedAssignIndex(a *syntax.Assign) (int, bool) {
	g := a.IndexFlags
	e := &syntax.ParamExpr{Name: a.Name, Index: a.Index, IndexFlags: g}
	for _, c := range g.Flags {
		if !strings.ContainsRune(implementedSubscriptFlags, c) {
			r.refuseSubscriptFlag(e, string(c), "")
			return 0, false
		}
	}
	search := lastOf(g.Flags, searchSubscriptFlags)
	if search == 0 {
		// Nothing to select by, so the operand is an ordinary subscript —
		// which is the read side's rule and measured to be this side's too:
		// `b[(e)2]=Q` on `(x y z)` is `x Q z` in the shell with the
		// construct. The *empty* group is the one shape that is not, and it
		// is `bad pattern` there and a parse error here, so neither writes.
		idx, err := r.subscriptValue(r.joinWord(g.Arg))
		if err != nil {
			text := r.subscriptAsWritten(a.Index)
			r.fatal("%s\n", r.subscriptFailure(text, err))
			return 0, false
		}
		return idx, true
	}
	if _, isAssoc := r.assocFor(a.Name); isAssoc {
		// The same refusal the read side gives, and for the same reason: the
		// letters mean something else over a table, and answering with the
		// ordered array's rule would write to a plausible wrong key. The
		// shell refuses it too, as `attempt to set slice`.
		r.refuseSubscriptFlag(e, string(search), " for an associative array")
		return 0, false
	}
	elems, scalar, held := r.subscriptTarget(e)
	if scalar && held {
		// A search over a plain string names a *character* position, which
		// the read side answers — see searchScalar — and this side still
		// refuses, because the subscript it would hand on is not answered
		// either: `s=hello; s[3]=Q` is `heQlo` in the shell with the
		// construct and two spaces and a `Q` here — the string read as an
		// array of one, with a third element written past it and the whole
		// joined. Returning the index the search
		// found would turn a refusal by name into that value, which is the
		// one outcome worse than the refusal (#1532).
		r.refuseSubscriptFlag(e, string(search), " for a scalar")
		return 0, false
	}
	at, found := r.searchElements(g, search, elems)
	base := r.arrayBase()
	if found {
		return base + at, true
	}
	switch search {
	case 'i', 'r':
		// One past the last element, which is what makes both of these an
		// append: measured, `b[(i)nomatch]=W` and `b[(r)nomatch]=Q` on
		// `(x y z)` each leave four elements with the new one last.
		return base + len(elems), true
	}
	// `R` and `I` missing are not the same answer as each other in the shell
	// — the first is `assignment to invalid subscript range` and the second
	// puts the value at the *front* — and neither is the index one before the
	// first, which is what the read side answers and what an assignment
	// cannot use. Refused by name rather than guessed at; see the issue the
	// spec entry names.
	r.refuseSubscriptFlag(e, string(search), " where nothing matched")
	return 0, false
}
