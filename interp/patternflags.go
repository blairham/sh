// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// The pattern operators one shell keeps behind an option of its own —
// [ExtendedPatternOperators] — and which are ordinary text without it.
//
// docs/spec/grammar/patterns.md records the measurements. Four constructs,
// and they are separate because their precedence differs:
//
//	pat1~pat2   the exclusion: pat1, minus anything pat2 also matches
//	^pat        the negation: anything the rest of this branch does not match
//	item#       the closure: zero or more of the item in front of it
//	(#…)        a flag group, which changes how the rest of the branch reads
//
// The exclusion binds loosest — measured, `(a*~*b*|zz)` matches `zz`, so a
// `|` separates whole exclusions rather than the other way round — and a
// closure binds to exactly the one item in front of it.
//
// Nothing here is reached with the option off, and that is the measurement
// that makes the option necessary rather than a formality: with it off the
// same characters are literal, so `[[ 'a#' == a# ]]` matches and `[[ aaa ==
// a# ]]` does not, which is the opposite of both answers with it on.

// caseFolding is the case comparison a `(#i)`, `(#I)` or `(#l)` flag asks for.
//
// It is separate from [patternOpts.fold], which is the run-time option
// (`nocasematch` and its kin), because the two reach different parts of a
// pattern. Measured on zsh 5.9.2, 2026-09-07, with `extendedglob` on:
//
//	[[ ABC == (#i)abc ]]            matches — a literal folds
//	[[ B == (#i)\b ]]               matches — an escaped literal folds too
//	[[ ABC == (#i)[abc][abc][abc] ]]  does not — a bracket does not fold
//	[[ ABC == (#i)[[:lower:]]## ]]    does not — nor does a character class
//
// so a flag that folded everything would match two patterns real zsh
// refuses. The option folds brackets and classes as well, which is why it
// stays where it was.
type caseFolding int

const (
	// caseExact compares letters as they stand.
	caseExact caseFolding = iota
	// caseEither is `(#i)`: a letter in the pattern matches either case.
	caseEither
	// caseLowerEither is `(#l)`: a *lowercase* letter in the pattern matches
	// either case, and an uppercase one matches only itself. Measured:
	// `[[ ABC == (#l)abc ]]` matches and `[[ abc == (#l)ABC ]]` does not.
	caseLowerEither
)

// eqPatternByte compares one byte of a pattern with one byte of the subject,
// honoring both the option and the flag.
//
// The two sides are not interchangeable — `(#l)` asks about the *pattern's*
// letter — which is why this takes them named rather than as a pair.
func (o patternOpts) eqPatternByte(pat, sub byte) bool {
	if eqByte(pat, sub, o.fold) {
		return true
	}
	switch o.litFold {
	case caseEither:
		return swapCase(pat) == sub
	case caseLowerEither:
		return pat >= 'a' && pat <= 'z' && swapCase(pat) == sub
	}
	return false
}

// splitPatternFlags peels a `(#…)` flag group off the front of p.
//
// The body is what stands between `(#` and the `)` that closes it, and is
// empty for `(#)` — which is a group that says nothing and is measured to
// match: `[[ abc == (#)abc ]]` is true.
func splitPatternFlags(p string) (body, rest string, ok bool) {
	if !strings.HasPrefix(p, "(#") {
		return "", "", false
	}
	end, found := closingParen(p)
	if !found {
		return "", "", false
	}
	return p[2:end], p[end+1:], true
}

// applyPatternFlags folds a flag group's letters into the options.
//
// unknown is the first letter this matcher does not answer, and is 0 when
// every letter was taken. The letters that *are* answered are the three that
// only change how a literal compares, which is the whole of what a flag can
// do without the matcher knowing where it is in the subject — the rest
// (`(#b)`, `(#s)`, `(#a1)` and their kin) need a position and are refused by
// name rather than dropped.
func applyPatternFlags(body string, o patternOpts) (patternOpts, byte) {
	for i := range len(body) {
		switch body[i] {
		case 'i':
			o.litFold = caseEither
		case 'I':
			o.litFold = caseExact
		case 'l':
			o.litFold = caseLowerEither
		default:
			return o, body[i]
		}
	}
	return o, 0
}

// splitExclusion peels the `~` exclusions off a pattern branch.
//
// left is what has to match and rights are the patterns that must not, so
// `a*~*b*~*d` is `a*` with two things taken out of it — measured, all three
// are compared against the whole subject rather than against each other.
//
// A `~` with nothing after it is not an operator. Measured: `[[ 'a~' == a~ ]]`
// matches and `[[ ab == a~ ]]` does not, so a trailing tilde is the character
// itself. The same is true of one with nothing in front of it, which is what
// keeps a `~` that survived tilde expansion from turning the word inside out.
func splitExclusion(p string, o patternOpts) (left string, rights []string, ok bool) {
	if !o.extended {
		return "", nil, false
	}
	var cuts []int
	depth := 0
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '\\':
			i++
		case '(':
			depth++
		case ')':
			depth--
		case '[':
			if end, found := bracketEnd(p, i); found {
				i = end
			}
		case '~':
			if depth == 0 {
				cuts = append(cuts, i)
			}
		}
	}
	if len(cuts) == 0 {
		return "", nil, false
	}
	left = p[:cuts[0]]
	// No side may be empty, and the trailing one is the side a pattern can
	// actually reach. The leading check is kept for symmetry and is dead:
	// a pattern beginning with `~` is taken by tilde expansion before it is
	// a pattern, `${~p}` included — measured, `p='~a'; [[ x == ${~p} ]]` is
	// `no such user or named directory: a` — so no probe reaches it and
	// mutation testing is what said so.
	if left == "" {
		return "", nil, false
	}
	for k, c := range cuts {
		end := len(p)
		if k+1 < len(cuts) {
			end = cuts[k+1]
		}
		part := p[c+1 : end]
		if part == "" {
			return "", nil, false
		}
		rights = append(rights, part)
	}
	return left, rights, true
}

// hasTopLevelExclusion reports whether a whole field carries a `~` that would
// be an exclusion. It is the question pathname expansion has to ask before it
// splits a field into components, because the exclusion is measured to be
// *looser* than `/` — `**/x~*bar*` takes `bar/x` out by matching the whole
// path — where every other operator here is read inside one component.
func hasTopLevelExclusion(field string, o patternOpts) bool {
	_, _, ok := splitExclusion(field, o)
	return ok
}

// bracketEnd is the offset of the `]` that closes the bracket expression
// starting at i, so a scan over a pattern can step over one whole.
func bracketEnd(p string, i int) (int, bool) {
	j := i + 1
	if j < len(p) && (p[j] == '!' || p[j] == '^') {
		j++
	}
	if j < len(p) && p[j] == ']' {
		j++
	}
	for ; j < len(p); j++ {
		if p[j] == ']' {
			return j, true
		}
	}
	return 0, false
}

// splitClosableItem peels the one item a closure could repeat.
//
// A closure binds to exactly one item and never to a run of them — measured,
// `[[ abbb == ab# ]]` matches where `[[ abab == ab# ]]` does not — so what
// counts as an item is the whole of the operator's reach: a group, a bracket
// expression, an escaped character, a `?`, a numeric range, or one ordinary
// character.
//
// A `*` is not one. Real zsh calls `*#` a bad pattern rather than repeating
// the star, so it is left out here and reported by the scan instead — which
// makes the guard below dead in both callers, since each answers a `*` before
// it asks. It is kept because it is what the sentence above says, and
// mutation testing is what established that nothing can kill it.
func splitClosableItem(p string, o patternOpts) (item, rest string, ok bool) {
	if p == "" || p[0] == '*' {
		return "", "", false
	}
	if _, _, after, found := splitGroup(p, o); found {
		return p[:len(p)-len(after)], after, true
	}
	if _, _, after, found := splitNumericRange(p, o); found {
		return p[:len(p)-len(after)], after, true
	}
	switch p[0] {
	case '[':
		if end, found := bracketEnd(p, 0); found {
			return p[:end+1], p[end+1:], true
		}
	case '\\':
		if len(p) > 1 {
			return p[:2], p[2:], true
		}
	}
	w := o.unitWidth(p)
	return p[:w], p[w:], true
}

// unboundedRepeat is the upper bound a closure with no maximum has.
const unboundedRepeat = -1

// closureBounds reads a closure operator standing where p begins.
//
// Three spellings, all measured: `#` is zero or more, `##` is one or more,
// and `(#c2,4)` is a count with either side optional — `(#c3)` is exactly
// three, `(#c2,)` is two or more, `(#c,3)` is at most three, and `(#c,)` is
// the same as `#`.
func closureBounds(p string, o patternOpts) (lo, hi int, rest string, ok bool) {
	// Dead, like the star guard above: every caller is already inside a
	// branch the option opened. Kept so the function answers for itself.
	if !o.extended {
		return 0, 0, "", false
	}
	if strings.HasPrefix(p, "##") {
		return 1, unboundedRepeat, p[2:], true
	}
	if strings.HasPrefix(p, "#") {
		return 0, unboundedRepeat, p[1:], true
	}
	body, after, found := splitPatternFlags(p)
	if !found || !strings.HasPrefix(body, "c") {
		return 0, 0, "", false
	}
	spec := body[1:]
	from, to, cok := readCountSpec(spec)
	if !cok {
		return 0, 0, "", false
	}
	return from, to, after, true
}

// readCountSpec reads the `2,4` of a `(#c2,4)`.
func readCountSpec(spec string) (lo, hi int, ok bool) {
	before, after, comma := strings.Cut(spec, ",")
	lo, ok = readCount(before, 0)
	if !ok {
		return 0, 0, false
	}
	if !comma {
		return lo, lo, true
	}
	hi, ok = readCount(after, unboundedRepeat)
	if !ok {
		return 0, 0, false
	}
	return lo, hi, true
}

// readCount reads one side of a count, answering empty with the default.
func readCount(s string, empty int) (int, bool) {
	if s == "" {
		return empty, true
	}
	n := 0
	for i := range len(s) {
		if !isDigit(s[i]) {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
		if n > 1<<20 {
			return 0, false
		}
	}
	return n, true
}

// matchRepeat matches a closure — between lo and hi repetitions of item —
// and then whatever follows it.
//
// Every count in range is tried against every split of the subject, because
// only what comes after can say where the run ends: `a#ab` against `aab`
// needs the closure to stop one character early. A repetition always consumes
// at least one unit, which is what makes the recursion terminate for an item
// that can match nothing.
func matchRepeat(item string, lo, hi int, after, s string, o patternOpts) bool {
	return repeatFrom(item, 0, lo, hi, after, s, o)
}

func repeatFrom(item string, k, lo, hi int, after, s string, o patternOpts) bool {
	if k >= lo && matchHere(after, s, o) {
		return true
	}
	if hi != unboundedRepeat && k >= hi {
		return false
	}
	for i := 0; i < len(s); {
		i += o.unitWidth(s[i:])
		if !matchHere(item, s[:i], o) {
			continue
		}
		if repeatFrom(item, k+1, lo, hi, after, s[i:], o) {
			return true
		}
	}
	return false
}

// patternFault is a construct in a pattern this matcher will not answer.
//
// It is found before matching rather than during it, and that is deliberate:
// the matcher recurses, backtracks and tries branches that are never taken,
// so a refusal raised from inside it would fire for a `(#b)` down an arm the
// subject never reached. Reading the pattern once says the same thing at the
// same place every time.
type patternFault struct {
	// flag is the letter of a `(#…)` flag this matcher does not implement,
	// or 0 when the fault is not a flag.
	flag byte
	// bad marks a pattern the shell itself rejects, whatever we implement.
	bad bool
}

// implementedPatternFlags are the flag letters the matcher answers.
//
// The rest of the family needs the position of the match within the subject,
// which this matcher does not carry, so each is refused by name — a flag that
// quietly did nothing would be exactly the failure this refusal exists to
// prevent.
const implementedPatternFlags = "iIl"

// knownPatternFlags are the letters the shell itself has, so that a refusal
// can tell "this shell does not do that yet" apart from "no shell does".
//
// Measured on zsh 5.9.2, 2026-09-07, by asking `[[ abc == (#X)abc ]]` of all
// 52 letters with `extendedglob` on: twelve are taken bare, and `a` and `c`
// are taken only with a number after them — `(#a)` and `(#c)` are both `bad
// pattern` where `(#a1)` and `(#c1)` are not. Everything else is rejected.
const knownPatternFlags = "bBeilmqsuIMU"

// numberedPatternFlags are the two that are a letter *and* a number: `(#a1)`
// is approximate matching within one error and `(#c2,4)` is a count.
const numberedPatternFlags = "ac"

// classifyPatternFlags reads a flag group and says what is wrong with it.
//
// found is false for a group this matcher answers in full.
func classifyPatternFlags(body string) (patternFault, bool) {
	for i := 0; i < len(body); i++ {
		c := body[i]
		if strings.IndexByte(implementedPatternFlags, c) >= 0 {
			continue
		}
		digits := 0
		for i+1+digits < len(body) && (isDigit(body[i+1+digits]) || body[i+1+digits] == ',') {
			digits++
		}
		switch {
		case strings.IndexByte(numberedPatternFlags, c) >= 0 && digits > 0:
			return patternFault{flag: c}, true
		case strings.IndexByte(knownPatternFlags, c) >= 0:
			return patternFault{flag: c}, true
		}
		// A letter no shell in the panel has: the pattern is not a pattern.
		return patternFault{bad: true}, true
	}
	return patternFault{}, false
}

// scanExtendedPattern reads a pattern the way the matcher will and reports
// the first construct that would not be answered.
func scanExtendedPattern(p string, o patternOpts) (patternFault, bool) {
	if left, rights, ok := splitExclusion(p, o); ok {
		for _, part := range append([]string{left}, rights...) {
			if f, found := scanExtendedPattern(part, o); found {
				return f, true
			}
		}
		return patternFault{}, false
	}
	// closable says the text just read is something a closure could repeat.
	// A closure needs one, and everything that reaches here without one is a
	// bad pattern: `*#`, `a###b`, a bare `(#c3)`, and a `#` at the front of
	// a pattern. The last is measured through the only route that gets an
	// unescaped one to the matcher — `p="#foo"; [[ "#foo" == ${~p} ]]` is
	// `bad pattern: #foo` in real zsh, where the same pattern written as
	// `$p` matches, because an expansion's characters are not operators.
	closable := false
	for len(p) > 0 {
		if p[0] == '^' {
			p, closable = p[1:], false
			continue
		}
		if body, rest, ok := splitPatternFlags(p); ok {
			if _, _, isCount := countClosure(body); isCount {
				if !closable {
					return patternFault{bad: true}, true
				}
				p, closable = rest, false
				continue
			}
			if f, bad := classifyPatternFlags(body); bad {
				return f, true
			}
			p, closable = rest, false
			continue
		}
		if p[0] == '#' {
			if !closable {
				return patternFault{bad: true}, true
			}
			n := 1
			if strings.HasPrefix(p, "##") {
				n = 2
			}
			if len(p) > n && p[n] == '#' {
				return patternFault{bad: true}, true
			}
			p, closable = p[n:], false
			continue
		}
		if p[0] == '*' {
			if len(p) > 1 && p[1] == '#' {
				// A star is not an item, so there is nothing to repeat.
				return patternFault{bad: true}, true
			}
			p, closable = p[1:], false
			continue
		}
		item, rest, ok := splitClosableItem(p, o)
		if !ok {
			break
		}
		if body, _, _, isGroup := splitGroup(item, o); isGroup {
			for _, arm := range alternatives(body) {
				if f, found := scanExtendedPattern(arm, o); found {
					return f, true
				}
			}
		}
		p, closable = rest, true
	}
	return patternFault{}, false
}

// countClosure reads a `(#c…)` body, which is a closure wearing a flag
// group's spelling rather than a flag.
func countClosure(body string) (lo, hi int, ok bool) {
	if !strings.HasPrefix(body, "c") {
		return 0, 0, false
	}
	return readCountSpec(body[1:])
}

// extendedPatternOpts fills in the extended-operator answers for one pattern,
// and refuses one holding a construct this matcher does not implement.
//
// badStatus is what the shell exits with where the *shell* rejects the
// pattern, which is a per-surface answer rather than one number: measured on
// zsh 5.9.2, `[[ x == (#Z)a ]]` exits 2, the same pattern in a `case` exits
// 0, and in `${x#…}` or against the filesystem it exits 1. All four abandon
// the script rather than failing the match.
//
// A construct the shell has and we do not is our own refusal and not zsh's,
// so it is worded as one and exits 1 wherever it stands. Reporting it is the
// whole point of this function: a `(#b)` that was quietly dropped would leave
// `$match[1]` empty, which reads as "the group matched nothing" rather than
// as "this shell does not do backreferences".
func (r *Runner) extendedPatternOpts(o patternOpts, pattern string, badStatus int) patternOpts {
	if !r.MatchOption(ExtendedPatternOperators) {
		return o
	}
	o.extended = true
	f, found := scanExtendedPattern(pattern, o)
	switch {
	case !found:
		return o
	case f.bad:
		r.fatalPattern(pattern, badStatus)
	default:
		r.diagf("%s: the (#%c) pattern flag is not implemented\n", pattern, f.flag)
		r.status = 1
		r.ctl = controlExit
	}
	return o
}
