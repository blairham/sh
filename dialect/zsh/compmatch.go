// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strings"
	"unicode"
)

// `compadd -M`: the match specification a completion is matched under.
//
// A matcher-list style is where nearly every configuration spells this, and
// its first entry is usually case folding — `m:{a-zA-Z}={A-Za-z}` — then
// partial words and substrings. `_main_complete` hands each entry to `compadd`
// through `-M`, which read the option and dropped it, so a matcher-list did
// nothing at all (#6152).
//
// The language is the one `zshcompwid(1)` gives under COMPLETION MATCHING
// CONTROL, and every reading below was measured on zsh 5.9.2, 2026-10-05,
// from a widget calling `compadd -O out -M spec` with `PREFIX` set, so that
// what is read is the builtin's matching alone:
//
//	m:{a-z}={A-Z}               re   README.md repl readme      (not Rx)
//	m:{a-z}={A-Z}               RE   README.md                  (one way only)
//	m:{abc}={xy}                c    c                          (excess ignored)
//	m:[ab]=[xy]                 a    a x y
//	r:|[._-]=* r:|=*            f.b  foo.bar.baz foo.baz f.b
//	r:|.=*                      .u   comp.unix .unix            (* holds no .)
//	r:|[.]=**                   a.c  ab.cd.ef a.b.c             (** holds anything)
//	l:|=* r:|=*                 pl   repl apple plum
//	r:?||[[:upper:]]=*          fB   fooBar fB                  (not fooHooBar)
//	l:.||[[:alpha:]]=by         pass.n  pass.byname pass.name
//	r:a|b=*                     ab   ab axxb aXb
//	l:a|=*                      ab   ab axb                     (not xab)
//	b:-=+                       --x  +-x --x                    (the first only)
//	e:-=+                       x--  x--                        (nothing, see below)
//	x: m:{a-z}={A-Z}            re   repl                       (x: ends it)
//
// Two of those are narrower than the manual's sentences read:
//
//   - **`b:` broadens the first character of the word and no more.** `--x`
//     under `b:-=+` matches `+-x` and not `++x`, and `B:` is the same.
//   - **`e:` broadens nothing while the cursor is at the end of the word.**
//     The end it means is the end of `SUFFIX`, measured with `PREFIX=x
//     SUFFIX=-`, which matches `x+`; with `SUFFIX` empty the default `*`
//     after the cursor stands between the word and the candidate's end, and
//     `x-` under `e:-=+` matches `x-` alone. Matching here reads `PREFIX`
//     only, which is that case.
//
// An upper-case matcher matches exactly as its lower-case partner does and
// then writes the word's own characters into the match in place of what they
// matched, measured through Tab: `M:_=` takes `f_o` to `f_oo`, `L:|-=` takes
// `-f` to `-foo`, `M:{a-z}={A-Z}` takes `rea` to `reaDME`, and `L:|=* r:|=*`
// takes `pl` against `apple` to `ple`. Where a lower-case matcher could match
// the same characters, the manual says it wins; the search tries every
// lower-case matcher before any upper-case one, so the first path found is
// that one.

// matchSpec is one `-M` argument read, or several joined: the matchers up to
// the first `x:`.
type matchSpec []matcher

// matcher is one `letter:…` word.
type matcher struct {
	kind  byte // m, b, e, l or r, lower-cased
	upper bool // M, B, E, L or R: write the word's characters into the match

	// The positions it applies at. For `l:` and `r:`, edge is the `l:|…`
	// and `r:…|` spellings, which apply at the edge of the word; double is
	// the two-anchor spellings, where word is empty and coanchor is set.
	edge, double bool
	anchor       matchPattern
	coanchor     matchPattern
	word         matchPattern
	match        matchPattern
}

// matchPattern is one side of a matcher: a run of one-character elements, or
// `*` or `**` on the match side.
type matchPattern struct {
	elems []matchElem
	stars int // 1 for `*`, 2 for `**`
}

// matchElem matches one character.
type matchElem struct {
	any   bool        // ?
	lit   rune        // a literal, where set is nil and any is false
	set   []matchSlot // a bracket or brace expression
	neg   bool        // a bracket expression's leading ! or ^
	brace bool        // braces: the nth slot pairs with the other side's nth
}

// matchSlot is one entry of a bracket or brace expression: a character, a
// range spelled out, or a class by name.
type matchSlot struct {
	runes []rune // a literal is one rune, a range is all of them
	class string // `[:upper:]` and its kind, where runes is nil
}

// parseMatchSpec reads the matchers in a specification, stopping at `x:`.
// A word it cannot read is skipped rather than refused: a builtin that
// refused a call over one matcher would stop the whole completion.
func parseMatchSpec(spec string) matchSpec {
	var out matchSpec
	for _, word := range strings.Fields(spec) {
		if len(word) < 2 || word[1] != ':' {
			continue
		}
		if word[0] == 'x' {
			break
		}
		m, ok := parseMatcher(word)
		if ok {
			out = append(out, m)
		}
	}
	return out
}

func parseMatcher(word string) (matcher, bool) {
	m := matcher{kind: byte(unicode.ToLower(rune(word[0]))), upper: unicode.IsUpper(rune(word[0]))}
	rest := []rune(word[2:])
	switch m.kind {
	case 'm', 'b', 'e':
		wp, n, ok := parseMatchPattern(rest, '=')
		if !ok {
			return m, false
		}
		m.word, rest = wp, rest[n+1:]
	case 'l':
		first, n, ok := parseMatchPattern(rest, '|')
		if !ok {
			return m, false
		}
		rest = rest[n+1:]
		if len(rest) > 0 && rest[0] == '|' {
			// l:anchor||coanchor=match
			co, k, ok := parseMatchPattern(rest[1:], '=')
			if !ok {
				return m, false
			}
			m.double, m.anchor, m.coanchor, rest = true, first, co, rest[k+2:]
			break
		}
		wp, k, ok := parseMatchPattern(rest, '=')
		if !ok {
			return m, false
		}
		m.anchor, m.word, m.edge, rest = first, wp, len(first.elems) == 0, rest[k+1:]
	case 'r':
		first, n, ok := parseMatchPattern(rest, '|')
		if !ok {
			return m, false
		}
		rest = rest[n+1:]
		if len(rest) > 0 && rest[0] == '|' {
			// r:coanchor||anchor=match
			an, k, ok := parseMatchPattern(rest[1:], '=')
			if !ok {
				return m, false
			}
			m.double, m.coanchor, m.anchor, rest = true, first, an, rest[k+2:]
			break
		}
		an, k, ok := parseMatchPattern(rest, '=')
		if !ok {
			return m, false
		}
		m.word, m.anchor, m.edge, rest = first, an, len(an.elems) == 0, rest[k+1:]
	default:
		return m, false
	}
	switch string(rest) {
	case "*":
		m.match.stars = 1
	case "**":
		m.match.stars = 2
	default:
		mp, n, ok := parseMatchPattern(rest, 0)
		if !ok || n != len(rest) {
			return m, false
		}
		m.match = mp
	}
	if m.match.stars > 0 && m.kind != 'l' && m.kind != 'r' {
		return m, false
	}
	return m, true
}

// parseMatchPattern reads elements until stop, answering how many runes it
// took. A stop of 0 reads to the end.
func parseMatchPattern(src []rune, stop rune) (matchPattern, int, bool) {
	var p matchPattern
	i := 0
	for i < len(src) {
		c := src[i]
		if stop != 0 && c == stop {
			return p, i, true
		}
		switch c {
		case '\\':
			if i+1 >= len(src) {
				return p, i, false
			}
			p.elems = append(p.elems, matchElem{lit: src[i+1]})
			i += 2
		case '?':
			p.elems = append(p.elems, matchElem{any: true})
			i++
		case '[', '{':
			e, n, ok := parseMatchSet(src[i:])
			if !ok {
				return p, i, false
			}
			p.elems = append(p.elems, e)
			i += n
		default:
			p.elems = append(p.elems, matchElem{lit: c})
			i++
		}
	}
	return p, i, stop == 0
}

// parseMatchSet reads one bracket or brace expression from its opening
// character, answering how many runes it took.
func parseMatchSet(src []rune) (matchElem, int, bool) {
	brace := src[0] == '{'
	closer := ']'
	if brace {
		closer = '}'
	}
	e := matchElem{brace: brace}
	i := 1
	if !brace && i < len(src) && (src[i] == '!' || src[i] == '^') {
		// Braces take no negation, so the manual has it: an initial `!` or
		// `^` there is a character.
		e.neg = true
		i++
	}
	first := true
	for i < len(src) {
		c := src[i]
		if c == closer && !(first && !brace) {
			return e, i + 1, true
		}
		first = false
		if c == '[' && i+1 < len(src) && src[i+1] == ':' {
			end := -1
			for k := i + 2; k+1 < len(src); k++ {
				if src[k] == ':' && src[k+1] == ']' {
					end = k
					break
				}
			}
			if end < 0 {
				return e, i, false
			}
			e.set = append(e.set, matchSlot{class: string(src[i+2 : end])})
			i = end + 2
			continue
		}
		if c == '\\' && i+1 < len(src) {
			i++
			c = src[i]
		}
		if i+2 < len(src) && src[i+1] == '-' && src[i+2] != closer {
			lo, hi := c, src[i+2]
			var rs []rune
			for r := lo; r <= hi; r++ {
				rs = append(rs, r)
			}
			e.set = append(e.set, matchSlot{runes: rs})
			i += 3
			continue
		}
		e.set = append(e.set, matchSlot{runes: []rune{c}})
		i++
	}
	return e, i, false
}

// classHas answers whether r is in the named character class.
func classHas(name string, r rune) bool {
	switch name {
	case "upper":
		return unicode.IsUpper(r)
	case "lower":
		return unicode.IsLower(r)
	case "alpha":
		return unicode.IsLetter(r)
	case "digit":
		return r >= '0' && r <= '9'
	case "alnum":
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	case "space", "blank":
		return unicode.IsSpace(r)
	case "punct":
		return unicode.IsPunct(r) || unicode.IsSymbol(r)
	case "xdigit":
		return strings.ContainsRune("0123456789abcdefABCDEF", r)
	case "cntrl":
		return unicode.IsControl(r)
	case "print":
		return unicode.IsPrint(r)
	case "graph":
		return unicode.IsGraphic(r) && !unicode.IsSpace(r)
	}
	return false
}

// index is where r falls among the slots, counted the way the manual counts
// for pairing two brace expressions: a range is all of its characters and a
// class is one. -1 is nowhere.
func (e matchElem) index(r rune) int {
	n := 0
	for _, s := range e.set {
		if s.runes == nil {
			if classHas(s.class, r) {
				return n
			}
			n++
			continue
		}
		for _, c := range s.runes {
			if c == r {
				return n
			}
			n++
		}
	}
	return -1
}

// slotAt is the slot the nth index falls in, and the offset into it.
func (e matchElem) slotAt(n int) (matchSlot, int, bool) {
	for _, s := range e.set {
		if s.runes == nil {
			if n == 0 {
				return s, 0, true
			}
			n--
			continue
		}
		if n < len(s.runes) {
			return s, n, true
		}
		n -= len(s.runes)
	}
	return matchSlot{}, 0, false
}

// has answers whether the element matches r on its own.
func (e matchElem) has(r rune) bool {
	switch {
	case e.any:
		return true
	case e.set == nil:
		return e.lit == r
	}
	return (e.index(r) >= 0) != e.neg
}

// pairs answers whether a character of the match, c, stands for the word's w
// under one pair of elements: the nth entry of a brace on the word's side
// pairs with the nth on the match's side, and anything else is the match
// side's own set.
func pairs(word, match matchElem, w, c rune) bool {
	if !word.brace || !match.brace {
		return match.has(c)
	}
	n := word.index(w)
	ws, _, ok := word.slotAt(n)
	if !ok {
		return false
	}
	ms, off, ok := match.slotAt(n)
	if !ok {
		// More on the word's side than the match's: the excess pairs with
		// nothing, measured — `m:{abc}={xy}` leaves `c` matching only `c`.
		return false
	}
	if ms.runes == nil {
		if ws.runes == nil {
			// `[:upper:]` against `[:lower:]` is a range each, which is to
			// say a case conversion.
			switch {
			case ws.class == "upper" && ms.class == "lower":
				return c == unicode.ToLower(w)
			case ws.class == "lower" && ms.class == "upper":
				return c == unicode.ToUpper(w)
			}
		}
		return classHas(ms.class, c)
	}
	if ws.runes == nil {
		return false
	}
	return ms.runes[off] == c
}

// matchesAt answers whether p's elements match s from i, all of them.
func (p matchPattern) matchesAt(s []rune, i int) bool {
	if i < 0 || i+len(p.elems) > len(s) {
		return false
	}
	for k, e := range p.elems {
		if !e.has(s[i+k]) {
			return false
		}
	}
	return true
}

// containsMatch answers whether any substring of s matches p, which is what
// a `*` beside an anchor may not hold.
func (p matchPattern) containsMatch(s []rune) bool {
	if len(p.elems) == 0 {
		return false
	}
	for i := range s {
		if p.matchesAt(s, i) {
			return true
		}
	}
	return false
}

// matchCandidate answers whether a candidate matches the word typed so far
// under the specification, and the candidate as it is to be inserted — the
// same string unless an upper-case matcher wrote the word's characters in.
//
// With no specification it is the prefix test it always was.
func (spec matchSpec) matchCandidate(word, candidate string) (string, bool) {
	if len(spec) == 0 {
		return candidate, strings.HasPrefix(candidate, word)
	}
	if strings.HasPrefix(candidate, word) {
		return candidate, true
	}
	w, c := []rune(word), []rune(candidate)
	s := matchSearch{spec: spec, w: w, c: c, failed: map[[2]int]bool{}}
	out, ok := s.from(0, 0)
	if !ok {
		return candidate, false
	}
	return out, true
}

// matchSearch is one candidate's search: positions in the word and in the
// candidate, with the ones known to fail remembered so that a `*` does not
// make the search exponential.
type matchSearch struct {
	spec   matchSpec
	w, c   []rune
	failed map[[2]int]bool
}

// from answers whether the word from i matches the candidate from j, and what
// the candidate is written as from j on.
func (s *matchSearch) from(i, j int) (string, bool) {
	if i == len(s.w) {
		// The default `*` after the cursor takes the rest.
		return string(s.c[j:]), true
	}
	key := [2]int{i, j}
	if s.failed[key] {
		return "", false
	}
	if j < len(s.c) && s.w[i] == s.c[j] {
		if rest, ok := s.from(i+1, j+1); ok {
			return string(s.c[j]) + rest, true
		}
	}
	// Lower-case matchers before upper-case ones: where both could match
	// the same characters, the manual says the lower-case one wins.
	for _, upper := range []bool{false, true} {
		for _, m := range s.spec {
			if m.upper != upper {
				continue
			}
			if out, ok := s.apply(m, i, j); ok {
				return out, true
			}
		}
	}
	s.failed[key] = true
	return "", false
}

// apply tries one matcher at word position i and candidate position j.
func (s *matchSearch) apply(m matcher, i, j int) (string, bool) {
	lw := len(m.word.elems)
	switch m.kind {
	case 'm':
	case 'b':
		if i != 0 {
			return "", false
		}
	case 'e':
		// See the file comment: the end `e:` means is the end of SUFFIX,
		// which this reading of PREFIX alone never reaches.
		return "", false
	case 'l':
		switch {
		case m.double:
			if !m.anchor.matchesAt(s.w, i-len(m.anchor.elems)) || !m.coanchor.matchesAt(s.w, i) {
				return "", false
			}
		case m.edge:
			if i != 0 {
				return "", false
			}
		default:
			if !m.anchor.matchesAt(s.w, i-len(m.anchor.elems)) {
				return "", false
			}
		}
	case 'r':
		switch {
		case m.double:
			if !m.coanchor.matchesAt(s.w, i-len(m.coanchor.elems)) || !m.anchor.matchesAt(s.w, i) {
				return "", false
			}
		case m.edge:
			if i+lw != len(s.w) {
				return "", false
			}
		default:
			if !m.anchor.matchesAt(s.w, i+lw) {
				return "", false
			}
		}
	}
	if m.double {
		lw = 0
	}
	if !m.word.matchesAt(s.w, i) {
		return "", false
	}
	if lw == 0 && len(m.match.elems) == 0 && m.match.stars == 0 {
		// Nothing for nothing: no progress on either side, which every move
		// must make so that the search ends.
		return "", false
	}
	written := func(span []rune) string {
		if m.upper {
			return string(s.w[i : i+lw])
		}
		return string(span)
	}
	if m.match.stars > 0 {
		for k := j; k <= len(s.c); k++ {
			span := s.c[j:k]
			if m.match.stars == 1 && !m.edge && m.anchor.containsMatch(span) {
				break
			}
			if lw == 0 && k == j && !(m.kind == 'r' && m.edge) {
				// An empty `*` for an empty word-pattern is no move at
				// all; the literal and the other matchers cover it.
				continue
			}
			if rest, ok := s.from(i+lw, k); ok {
				return written(span) + rest, true
			}
		}
		return "", false
	}
	lm := len(m.match.elems)
	if j+lm > len(s.c) {
		return "", false
	}
	if lw == lm && !m.double {
		for k := 0; k < lw; k++ {
			if !pairs(m.word.elems[k], m.match.elems[k], s.w[i+k], s.c[j+k]) {
				return "", false
			}
		}
	} else if !m.match.matchesAt(s.c, j) {
		return "", false
	}
	rest, ok := s.from(i+lw, j+lm)
	if !ok {
		return "", false
	}
	return written(s.c[j:j+lm]) + rest, true
}
