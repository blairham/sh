// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// ksh93's `~(…)` pattern-modifier prefix: a letter set in front of a pattern
// that says what language the pattern is written in and how it is compared.
//
// It is that shell's **only** spelling for a regular expression — bash and zsh
// have `=~` and ksh93 has this — so a ksh script wanting an ERE has nowhere
// else to go. [syntax.Dialect.TildeGroup] is what lets the group through the
// lexer; this is what reads the letters once it has (#2621).
//
// # The letters, measured
//
// Measured on ksh93u+ 2012-08-01, 2026-09-13, `env -i` with a scratch HOME,
// `-c`, one letter at a time against a probe that can tell the readings apart:
// `[[ abc == ~(X)a.c ]]` separates a regular expression from a glob, since
// `a.c` matches `abc` only where the `.` is a metacharacter, and
// `[[ xabcx == ~(X)a.c ]]` separates a *substring* search from a whole-string
// one.
//
//	letter  what the probes say
//	E       ERE, matching a substring: both probes true, `~(E)a?c` true too
//	G V     a **basic** regular expression — `^a.c$` and `a.c` both match and
//	        `a?c` does not, which is grep's basic syntax. Re-measured a
//	        pattern at a time on 2026-09-20 and the two letters part on none
//	        of eighteen probes. See breToRE2
//	X       the ERE plus one operator, `&`, which is a conjunction over the
//	        same span — `[[ abc == ~(X)a.c&abc ]]` matches and
//	        `[[ abc == ~(X)a&c ]]` does not. Nine further probes separate it
//	        from `E` on nothing. See tildeConjunction
//	P       a Perl regular expression: `\d`, `\w`, `\s`, a lazy `.*?` and an
//	        inline `(?i)` are each read, and each the way Go's `regexp` reads
//	        it
//	A B     each behaves as E does on every probe written here
//	F       a literal string, matching a substring: `~(F)a.c` matches the
//	        three characters `a.c` and does not match `abc`
//	L       a literal string as well, and no probe here separates it from F
//	K       the ksh glob, which is what a pattern with no prefix is
//	M O S U a m x   accepted, and no probe here makes any of them change an
//	        answer
//	p s     the shell glob, which is what a pattern with no prefix is —
//	        re-measured 2026-09-19: `~(p)a?c` and `~(s)a?c` both match `abc`
//	        while `~(p)a.c` and `~(s)a.c` do not, so the dot is an ordinary
//	        character in each and neither is a regular expression
//	g       the match takes as much subject as it can from where it begins:
//	        `v=aXbXc; ${v#~(g)*X}` is `c` where `${v#*X}` is `bXc`. It is
//	        **not** a global replacement — `${v//~(g)X/-}` and `${v/~(g)X/-}`
//	        are what they were without it — and it leaves a suffix trim
//	        alone, measured: `${v%~(g)X*}` is `aXb`, the same shortest suffix
//	        `${v%X*}` takes, because that trim is pinned at the far end and
//	        its greed has nowhere to go. See tildeGreedyTrim
//	N       the word is deleted where the pattern names nothing, which is
//	        the thing zsh spells `(N)` and bash needs an option for:
//	        `printf "[%s]" ~(N)zz*` writes nothing. See Runner.tildeGlobPattern
//	i       case-insensitive, and it folds a bracket and a character class
//	        as well as a literal — `~(i)[abc][abc][abc]` and
//	        `~(i)[[:lower:]][[:lower:]][[:lower:]]` both match `ABC`
//	l r     left and right anchors, which only a substring-matching flavor
//	        can show: `~(El)a.c` matches `abcx` and not `xabc`, `~(Er)a.c`
//	        the other way round, and `~(Elr)a.c` only `abc` itself
//	+ -     turn the letters after them on and off: `~(+i)` is `~(i)` and
//	        `[[ ABC == ~(-i)abc ]]` does not match
//	        (an empty `~()` is a plain glob, and matches)
//
// Every other letter is one ksh93 does **not** have, and what it does with one
// is measured too: `[[ abc == ~(Z)abc ]]` and `[[ abc == ~(Z)* ]]` are both
// status 1 with nothing on standard error, so an unknown letter is a pattern
// that cannot match rather than one that cannot be read.
//
// # What this shell honors
//
// `E`, `F`, `G`, `K`, `L`, `N`, `P`, `V`, `X`, `g`, `i`, `l`, `p`, `r`, `s`,
// the `+`/`-` toggles and the empty group. Every regular-expression flavor
// goes to Go's `regexp`, which is the engine this shell already compiles `=~`
// with: `E`, `X` and `P` are compiled as written and `G` and `V` are
// translated from basic syntax first, which is breToRE2.
//
// The rest are **refused by name rather than accepted and ignored**. `A` and
// `B` agree with `E` on every probe above, which is not evidence that they
// *are* `E`; and no probe here gives `M`, `O`, `S`, `U`, `a`, `m` or `x`
// anything to do. A flag taken and dropped is worse than one refused, because
// a pattern that silently means something else is a wrong answer at status 0.
//
// **What is refused inside the four new letters is a construct rather than
// the letter**, which is the posture #3894 settled for `E` and is the whole
// reason they could land at all. Measured 2026-09-20,
// `[[ abab == ~(G)\(ab\)\1 ]]` matches there and `[[ abcd == ~(G)\(ab\)\1 ]]`
// does not, with `[[ abcd == ~(G)\(ab\)cd ]]` as the control that says the
// group parses: the flavor has **backreferences**, and so does every other
// one that shell has, `E` included once the probe is written unescaped. Go's
// `regexp` is RE2 and has none — `regexp.Compile` refuses `(ab)\1` outright —
// so a pattern using one stops with the construct named, and every pattern
// that does not use one is answered. See #3186.
//
// # What is not modeled
//
//   - A group standing anywhere but the *front* of a pattern. Measured,
//     `[[ abc == a~(E)b.? ]]` matches there, so the prefix is really a flag
//     group that may appear mid-pattern; here only the leading one is read
//     and a later one is the literal characters it was before.
//   - A trim whose flavor searches. Measured, `s=abc; ${s#~(E)b}` is `ac`
//     there — the *matched span* is removed wherever it sits, so `#` and `%`
//     stop being prefix and suffix operators altogether. The trims here go on
//     trying prefixes and suffixes, so such a pattern simply does not match
//     and the value comes back whole, which is what it did before.
//   - `${.sh.match}` after an ERE with capture groups.
//   - A group on a field whose remainder holds a `/`. The group is the whole
//     field's there while the walk matches one component at a time, and the
//     reference shell's own answers do not compose — see tildeGlobPattern for
//     the rows. Such a field behaves exactly as it did before any of these
//     letters was read, which is the one answer here that cannot be a new
//     wrong one. `N` is the exception, because it is about the word rather
//     than about matching a component.
//   - A backreference or a lookaround inside any regular-expression flavor,
//     and a `\<` or `\>` word edge inside a basic one — **refused by name**
//     rather than left to answer a quiet `no`, which is what an `E` pattern
//     did until #3894. See unsupportedTilde.
//   - The rest of the escapes the extended flavors take. ksh93 keeps every
//     backslash in an `E`, `X` or `P` pattern and this shell keeps only the
//     digits, which is a divergence #3894 named and measured and this change
//     inherits unchanged for the two new extended letters. See
//     tildeKeepsBackslash for the rows and for the one exception, `X`'s `\&`.
//   - A `^` or `$` buried inside one operand of a conjunction. The two at the
//     ends of an operand are read against the subject, which is what ksh93
//     does; one in the middle of an alternation inside an operand is read
//     against the span. See tildeOperand.

// tildeFlavor is the pattern language a `~(…)` prefix selects.
type tildeFlavor uint8

const (
	// tildeGlob is `K`, and is what a pattern with no prefix already is.
	tildeGlob tildeFlavor = iota
	// tildeERE is `E`: the pattern is a POSIX extended regular expression.
	tildeERE
	// tildeAugERE is `X`: the extended regular expression plus one operator,
	// and finding which one took nine probes that separate it from nothing.
	// `&` is a **conjunction** there and an ordinary character in `E`:
	// measured 2026-09-20, `[[ abc == ~(X)a.c&abc ]]` matches and
	// `[[ abc == ~(X)a.c&axc ]]` does not, where `[[ "a&b" == ~(E)a&b ]]`
	// matches the three characters. See tildeConjunction for what the
	// operands have to agree about, which is not what it first looks like.
	tildeAugERE
	// tildePerl is `P`: a Perl regular expression. `\d`, `\w`, `\s`, a lazy
	// quantifier and an inline `(?i)` are all read there and all read the
	// same way by Go's `regexp`, so the flavor is the ERE compile with a
	// wider escape set rather than a second engine — measured 2026-09-20,
	// `[[ a1 == ~(P)a\d ]]` matches with `[[ ab == ~(P)a\d ]]` as the
	// control, and `[[ aXbXc == ~(P)^a.*?Xb ]]` matches where
	// `[[ aXbXc == ~(P)^a.*Xb$ ]]` does not.
	tildePerl
	// tildeBRE is `G` and `V`: a **basic** regular expression, where the
	// backslash turns a grouping, an interval, an alternation and the two
	// one-character repetitions on rather than off. No probe written here
	// separates the two letters — eighteen were tried — so they compile
	// alike. See breToRE2.
	tildeBRE
	// tildeLiteral is `F` and `L`: the pattern is the characters it is
	// written with, and no character is a metacharacter.
	tildeLiteral
	// tildeNever is a letter ksh93 does not have. The pattern is read and
	// cannot match, which is measured rather than chosen.
	tildeNever
)

// tildeModifier is a read `~(…)` prefix.
type tildeModifier struct {
	flavor tildeFlavor
	fold   bool
	left   bool
	right  bool
	// greedy is `g`: the match takes as much subject as it can from where it
	// begins. See tildeGreedyTrim, which is the one surface that can show it.
	greedy bool
	// null is `N`: a pattern that names nothing deletes the word rather than
	// standing as the text it was written as. See Runner.tildeGlobPattern.
	null bool
}

// kshTildeLetters are the letters ksh93 accepts inside a `~(…)` group.
//
// Measured a letter at a time over both cases of the alphabet: these are the
// ones that leave `[[ abc == ~(X)abc ]]` matching, and every letter left out
// makes it not match. The list is what tells "this shell does not do that
// yet" from "no shell does" — the same distinction knownPatternFlags draws
// for zsh's `(#…)` groups.
const kshTildeLetters = "ABEFGKLMNOPSUVXaglimprsx"

// honoredTildeLetters are the ones this shell answers.
const honoredTildeLetters = "EFGKLNPVXgilprs"

// splitTildeModifier peels a `~(…)` prefix off the front of p.
//
// The body is what stands between `~(` and the `)` that closes it, and is
// empty for `~()` — a group that says nothing, which is measured to match.
func splitTildeModifier(p string) (body, rest string, ok bool) {
	if !strings.HasPrefix(p, "~(") {
		return "", "", false
	}
	end := strings.IndexByte(p, ')')
	if end < 0 {
		return "", "", false
	}
	return p[2:end], p[end+1:], true
}

// readTildeModifier folds a group's letters into a modifier.
//
// unhonored is the first letter ksh93 has and this shell does not, and is 0
// when every letter was either answered or absent from that shell too.
func readTildeModifier(body string) (m tildeModifier, unhonored byte) {
	on := true
	for i := 0; i < len(body); i++ {
		switch c := body[i]; c {
		case '+':
			on = true
		case '-':
			on = false
		case 'E':
			m.flavor = tildeERE
		case 'X':
			m.flavor = tildeAugERE
		case 'P':
			m.flavor = tildePerl
		case 'G', 'V':
			m.flavor = tildeBRE
		case 'F', 'L':
			m.flavor = tildeLiteral
		case 'K', 'p', 's':
			m.flavor = tildeGlob
		case 'g':
			m.greedy = on
		case 'N':
			m.null = on
		case 'i':
			m.fold = on
		case 'l':
			m.left = on
		case 'r':
			m.right = on
		default:
			if strings.IndexByte(kshTildeLetters, c) < 0 {
				// A letter that shell has never heard of. The whole pattern
				// cannot match, and says nothing about it.
				return tildeModifier{flavor: tildeNever}, 0
			}
			return m, c
		}
	}
	return m, 0
}

// tildeExpr is a compiled `~(…)` pattern.
//
// Two shapes, because one flavor has an operator no single expression can
// carry: everything but a conjunction is one [regexp.Regexp], and a
// conjunction is the alternatives of the whole pattern, each holding the
// operands that have to describe the **same** span. See tildeConjunction.
type tildeExpr struct {
	re   *regexp.Regexp
	alts [][]tildeOperand
	// left and right say the span is pinned to an end of the piece, which a
	// conjunction has to enforce for itself: its operands are each compiled
	// anchored, so the anchors cannot ride on the expression the way they do
	// for the single-expression shape.
	left, right bool
}

// tildeOperand is one operand of a conjunction, and the two things about it
// that the span search rather than the engine has to answer.
//
// `^` and `$` inside an operand are about the **subject** and not about the
// span the operator is choosing, which is the one place the two readings come
// apart — measured 2026-09-20, `[[ abcabc == ~(X)^abc&abc$ ]]` does not match
// there even though `abc` sits at each end, with `[[ abab == ~(X)^ab&ab ]]`
// and `[[ abab == ~(X)ab&ab$ ]]` as the controls that say each anchor alone
// is satisfiable. Matching an anchored expression against the span's own text
// would answer yes to all three, because the span's start is the text's.
type tildeOperand struct {
	re *regexp.Regexp
	// atSubjectStart and atSubjectEnd are a leading `^` and a trailing `$`,
	// which is where both anchors are written in practice. One buried in an
	// alternation inside an operand is read against the span and is a limit
	// rather than a reading; see the flavor's notes.
	atSubjectStart, atSubjectEnd bool
}

// tildeRegex compiles the pattern for a flavor that is a regular expression.
//
// whole says the caller is asking about a whole subject rather than choosing
// the extent of a match: a substring search is what ksh93 does there, and an
// exact one is what a trim or a substitution needs, since those pick the span
// themselves and hand this one span to compare. Measured both ways —
// `[[ xabcx == ~(E)a.c ]]` matches, and `s=aXbXc; ${s//~(E)X/-}` is `a-b-c`
// rather than `-c`, which it would be if each span were searched.
func (m tildeModifier) tildeRegex(pattern string, whole bool) (tildeExpr, bool) {
	switch m.flavor {
	case tildeLiteral:
		pattern = regexp.QuoteMeta(pattern)
	case tildeBRE:
		expr, unsupported := breToRE2(pattern)
		if unsupported != "" {
			// Named and refused by Runner.tildeModifierOpts before the match
			// is ever asked for. Reaching here means a caller that did not
			// go through it, and a false is the answer it already had.
			return tildeExpr{}, false
		}
		pattern = expr
	}
	x := tildeExpr{left: !whole || m.left, right: !whole || m.right}
	if m.flavor == tildeAugERE {
		if alts, ok := tildeConjunction(pattern); ok {
			for _, alt := range alts {
				group := make([]tildeOperand, 0, len(alt))
				for _, operand := range alt {
					re, err := regexp.Compile(m.wrapRegex(operand, true, true))
					if err != nil {
						return tildeExpr{}, false
					}
					group = append(group, tildeOperand{
						re:             re,
						atSubjectStart: strings.HasPrefix(operand, "^"),
						atSubjectEnd: strings.HasSuffix(operand, "$") &&
							!strings.HasSuffix(operand, `\$`),
					})
				}
				x.alts = append(x.alts, group)
			}
			return x, true
		}
	}
	re, err := regexp.Compile(m.wrapRegex(pattern, x.left, x.right))
	if err != nil {
		return tildeExpr{}, false
	}
	x.re = re
	return x, true
}

// wrapRegex puts the flags and the anchors the letters asked for around one
// expression.
func (m tildeModifier) wrapRegex(pattern string, left, right bool) string {
	var b strings.Builder
	// The same flag the `=~` operator compiles under, and for the same
	// reason: these flavors are regular expressions too, so a newline in the
	// subject is ordinary ground. Measured for all five ksh93 has, 2026-09-20:
	// `[[ $'a\nc' == ~(F)a.c ]]` aside, every one of `E`, `X`, `P`, `V` and
	// `G` matches `a.c` against a subject whose middle character is a
	// newline. See regexDotAll.
	b.WriteString(regexDotAll)
	if m.fold {
		b.WriteString("(?i)")
	}
	if left {
		b.WriteString(`\A`)
	}
	b.WriteString("(?:")
	b.WriteString(pattern)
	b.WriteString(")")
	if right {
		b.WriteString(`\z`)
	}
	return b.String()
}

// match answers a compiled pattern against one piece of the subject.
func (x tildeExpr) match(piece string) bool {
	if x.re != nil {
		return x.re.MatchString(piece)
	}
	// A conjunction. Its operands are compiled anchored at both ends, so the
	// search this does by hand is the one the engine does for the ordinary
	// shape: find a span every operand of some alternative describes.
	//
	// The span is what makes it a search rather than a conjunction of
	// searches, and the two differ — measured 2026-09-20 on ksh93u+,
	// `[[ abc == ~(X)a&c ]]` does **not** match even though `a` and `c` are
	// each in the subject, and `[[ abcabc == ~(X)^abc&abc$ ]]` does not
	// either. The controls that say the operator works at all are
	// `[[ abc == ~(X)(a.c)&(abc) ]]` and `[[ ab == ~(X)a.&.b ]]`, which do.
	for start := 0; start <= len(piece); start++ {
		if x.left && start != 0 {
			break
		}
		for end := start; end <= len(piece); end++ {
			if x.right && end != len(piece) {
				continue
			}
			if x.matchesSpan(piece, start, end) {
				return true
			}
		}
	}
	return false
}

// matchesSpan reports whether some alternative's every operand describes the
// span of piece between start and end.
func (x tildeExpr) matchesSpan(piece string, start, end int) bool {
	span := piece[start:end]
	for _, alt := range x.alts {
		all := true
		for _, operand := range alt {
			if operand.atSubjectStart && start != 0 {
				all = false
			} else if operand.atSubjectEnd && end != len(piece) {
				all = false
			} else if !operand.re.MatchString(span) {
				all = false
			}
			if !all {
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

// tildeConjunction cuts an augmented ERE into the operands `&` joins, and is
// false for a pattern holding no such operator — which is nearly all of them,
// and is the shape that compiles to one expression.
//
// The cut is at the top level only: a `&` inside a group or a bracket
// expression is the ordinary character, and so is one written `\&`, which is
// why the operands come back with that spelling undone. Measured 2026-09-20:
// `[[ "a&b" == ~(X)[&]b ]]` and `[[ "a&b" == ~(X)a\&b ]]` both match, and
// `[[ abc == ~(X)(a&b) ]]` does not.
//
// **`&` binds tighter than `|`**, which is the one thing here a reader is
// likely to get the other way round, so the alternatives are the outer split:
// `[[ c == ~(X)a&b|c ]]` matches and `[[ c == ~(X)a&(b|c) ]]` does not.
func tildeConjunction(pattern string) (alts [][]string, ok bool) {
	var alt []string
	var b strings.Builder
	depth := 0
	cut := func(alternative bool) {
		alt = append(alt, b.String())
		b.Reset()
		if alternative {
			alts = append(alts, alt)
			alt = nil
		}
	}
	for i := 0; i < len(pattern); {
		switch c := pattern[i]; c {
		case '\\':
			if i+1 < len(pattern) && pattern[i+1] == '&' {
				// An escaped ampersand is the character, and Go's `regexp`
				// has no such escape — so the operator's own spelling is
				// what is dropped rather than passed on.
				b.WriteByte('&')
				i += 2
				continue
			}
			b.WriteString(pattern[i:min(i+2, len(pattern))])
			i += 2
		case '[':
			j := skipBracketExpression(pattern, i)
			b.WriteString(pattern[i:j])
			i = j
		case '(':
			depth++
			b.WriteByte(c)
			i++
		case ')':
			if depth > 0 {
				depth--
			}
			b.WriteByte(c)
			i++
		case '&', '|':
			if depth > 0 {
				b.WriteByte(c)
				i++
				continue
			}
			cut(c == '|')
			ok = ok || c == '&'
			i++
		default:
			b.WriteByte(c)
			i++
		}
	}
	cut(true)
	return alts, ok
}

// breToRE2 rewrites a basic regular expression as one Go's `regexp` can read,
// and names the first construct that has no reading there.
//
// BRE is the mirror of ERE for five operators: a **backslashed** `(`, `)`,
// `{`, `}`, `|`, `+` and `?` is the operator and a bare one is the character,
// where ERE has it the other way round. Measured on ksh93u+ 2012-08-01,
// 2026-09-20, with the pattern supplied through a variable so that the
// shell's own quote removal cannot reach it:
//
//	[[ abc == ~(G)a\(b\)c ]]        matches — `\(…\)` groups
//	[[ 'a(b)c' == ~(G)a(b)c ]]      matches — a bare paren is the character
//	[[ aaa == ~(G)a\{3\} ]]         matches — `\{…\}` is the interval
//	[[ 'a{3}' == ~(G)a{3} ]]        matches — a bare brace is the character
//	[[ ab == ~(G)a\|b ]]            matches — `\|` alternates
//	[[ 'a|b' == ~(G)a|b ]]          matches — a bare bar is the character
//	[[ aab == ~(G)a\+b ]]           matches — `\+` repeats
//	[[ 'a+b' == ~(G)a+b ]]          matches — a bare plus is the character
//	[[ ab == ~(G)ax\?b ]]           matches — `\?` is zero or one
//	[[ 'a?b' == ~(G)a?b ]]          matches — a bare question is the character
//
// `.`, `*`, `[…]` and the anchors read as they do anywhere, and the anchors
// are positional rather than always live — the same rule POSIX states and one
// a translation has to carry, because RE2 would take every one of them:
//
//	[[ 'x^y' == ~(G)x^y ]]          matches — a caret inside is the character
//	[[ 'x$y' == ~(G)x$y ]]          matches — and so is a dollar inside
//	[[ ab == ~(G)\(^a\)b ]]         matches — a caret just past `\(` anchors
//	[[ xab == ~(G)\(^a\)b ]]        does not — the control for the row above
//	[[ ab == ~(G)a\(b$\) ]]         matches — a dollar just before `\)` anchors
//	[[ abx == ~(G)a\(b$\) ]]        does not
//	[[ ab == ~(G)^a\|^b ]]          matches — on the first branch's anchor
//	[[ b == ~(G)^a\|^b ]]           does **not**, so a caret just past a `\|`
//	                                is the character rather than an anchor
//
// **`V` is not distinguished from `G` by any of the eighteen probes written
// here**, the four rows above included, so the two letters compile alike and
// the pair is recorded rather than guessed at: `[[ 'a+b' == ~(V)a+b ]]`,
// `[[ abc == ~(V)a\(b\)c ]]`, `[[ aaa == ~(V)a\{3\} ]]` and
// `[[ ab == ~(V)a\|b ]]` all answer as `G` does.
//
// Two constructs are named and refused rather than translated, which is the
// posture #3894 settled for `E` and this inherits:
//
//	[[ abab == ~(G)\(ab\)\1 ]]      matches there — a **backreference**
//	[[ abcd == ~(G)\(ab\)\1 ]]      does not — the control
//	[[ abcd == ~(G)\(ab\)cd ]]      matches — the control that says it groups
//	[[ 'ab cd' == ~(G)\<cd ]]       matches there — a one-sided **word** edge
//	[[ abcd == ~(G)\<cd ]]          does not — the control
//
// RE2 has neither: no backreferences at all, and `\b` only, which is both
// edges at once and cannot be narrowed without a lookaround it also lacks.
//
// A `*` with nothing in front of it is left as the operator rather than
// escaped into a character, and that is measured rather than lazy: ksh93
// matches **neither** subject for `~(G)*ab` — not `*ab` and not `ab` — and a
// `*` RE2 refuses to compile answers the same way, where escaping it would
// make the first match and invent a reading.
func breToRE2(pattern string) (expr, unsupported string) {
	var b strings.Builder
	// atStart is where there is nothing yet for a repetition to take and a
	// `^` is the anchor rather than the character: the front of the
	// expression, and just past a `\(` or a `\|`.
	atStart := true
	// re2Meta is what has to be escaped to reach RE2 as a character.
	const re2Meta = `\.+*?()|[]{}^$`
	for i := 0; i < len(pattern); {
		switch c := pattern[i]; c {
		case '\\':
			if i+1 >= len(pattern) {
				b.WriteString(`\\`)
				i++
				continue
			}
			d := pattern[i+1]
			switch {
			case d >= '1' && d <= '9':
				return "", `\` + string(rune(d)) + " backreference"
			case d == '<' || d == '>':
				return "", `\` + string(rune(d)) + " word edge"
			case strings.IndexByte(`(){}|+?`, d) >= 0:
				b.WriteByte(d)
				atStart = d == '('
			default:
				// The character it is written as, which RE2 spells with a
				// backslash for its own operators and without one for
				// everything else — `\y` there is an error rather than the
				// letter. A rune rather than a byte, so that a backslash in
				// front of a multi-byte character does not split it.
				_, size := utf8.DecodeRuneInString(pattern[i+1:])
				if size == 1 && strings.IndexByte(re2Meta, d) >= 0 {
					b.WriteByte('\\')
				}
				b.WriteString(pattern[i+1 : i+1+size])
				atStart = false
				i += 1 + size
				continue
			}
			i += 2
		case '[':
			j := skipBracketExpression(pattern, i)
			b.WriteString(pattern[i:j])
			atStart = false
			i = j
		case '^':
			if atStart {
				b.WriteByte('^')
			} else {
				b.WriteString(`\^`)
			}
			i++
		case '$':
			if breAnchorsHere(pattern, i+1) {
				b.WriteByte('$')
			} else {
				b.WriteString(`\$`)
			}
			atStart = false
			i++
		case '(', ')', '{', '}', '|', '+', '?':
			b.WriteByte('\\')
			b.WriteByte(c)
			atStart = false
			i++
		case '.', '*':
			b.WriteByte(c)
			atStart = false
			i++
		default:
			if strings.IndexByte(re2Meta, c) >= 0 {
				b.WriteByte('\\')
			}
			b.WriteByte(c)
			atStart = false
			i++
		}
	}
	return b.String(), ""
}

// breAnchorsHere reports whether a `$` ending at i is the end-of-subject
// anchor rather than the character — true at the end of the expression, and
// before the `\)` or `\|` that ends the branch it is in.
func breAnchorsHere(pattern string, i int) bool {
	if i >= len(pattern) {
		return true
	}
	return pattern[i] == '\\' && i+1 < len(pattern) && pattern[i+1] == ')'
}

// unsupportedTilde names the first construct in this pattern that the engine
// underneath the flavor cannot express, and is empty for one it can take
// whole — a glob or a literal always, since neither reaches an engine.
//
// One function per *flavor family* rather than one per letter: the three
// extended flavors share a scan because they share RE2's two absences, and
// the basic one is answered by the translation that has to read the pattern
// anyway. A second scan beside either is how the two would come to disagree.
func (m tildeModifier) unsupportedTilde(pattern string) string {
	switch m.flavor {
	case tildeERE, tildeAugERE, tildePerl:
		return unsupportedERE(pattern)
	case tildeBRE:
		_, unsupported := breToRE2(pattern)
		return unsupported
	}
	return ""
}

// unsupportedERE names the first construct in an ERE that the engine
// underneath cannot express, and is empty for a pattern it can take whole.
//
// `E` is the one regular-expression flavor this shell answers and Go's
// `regexp` is RE2, which has **no backreferences and no lookaround** —
// `regexp.Compile` refuses `(ab)\1`, `(?=`, `(?!` and `(?<` outright. What
// that refusal bought before this was a `false` out of tildeRegex, so
// measured against ksh93u+ 2012-08-01 on 2026-09-20:
//
//	[[ abab == ~(E)(ab)\1 ]]    ksh93 yes, this shell a silent no at 0
//	[[ abc == ~(E)a(?=b)bc ]]   ksh93 yes, this shell a silent no at 0
//
// A wrong answer in silence is the shape this repository minds most, and the
// four letters #3186 refuses by name are the precedent for what to do
// instead: say which construct, and stop. A script that halts honestly is a
// script whose author can see the gap; one that reads `no` cannot.
//
// The scan is over what the **engine** would read rather than over the
// characters, because the two differ in exactly the places that decide this:
// a `\` behind another `\` is a literal backslash and the digit after it is
// an ordinary digit, and inside a bracket expression `[(?=]` is three
// ordinary characters. Refusing either would be refusing a pattern RE2
// compiles and answers correctly, which trades a silent wrong answer for a
// loud one (#3894).
//
// `\1` is refused wherever the digit run goes on, and that is deliberate
// rather than a rounding: Go reads `\12` as the **octal** escape for a
// newline where ksh93 reads a backreference followed by a `2`, so the one
// spelling this scan would otherwise let through is the one whose silence is
// hardest to see.
//
// Not covered, and left as it was: a pattern the engine refuses for some
// other reason — `[\1]`, an unclosed group — still answers a quiet no. That
// is the general "the compile failed" silence rather than a construct this
// shell declines to have, and naming a construct is what this is for.
func unsupportedERE(pattern string) string {
	for i := 0; i < len(pattern); {
		switch pattern[i] {
		case '\\':
			if i+1 >= len(pattern) {
				return ""
			}
			if d := pattern[i+1]; d >= '1' && d <= '9' {
				return `\` + string(d) + " backreference"
			}
			i += 2
		case '[':
			i = skipBracketExpression(pattern, i)
		case '(':
			for _, look := range []string{"(?=", "(?!", "(?<=", "(?<!"} {
				if strings.HasPrefix(pattern[i:], look) {
					return look + " lookaround"
				}
			}
			i++
		default:
			i++
		}
	}
	return ""
}

// skipBracketExpression returns the index just past the bracket expression
// opening at i, and i+1 where the `[` opens none the engine would close.
//
// A `]` first in the set is a member and not the close, `^` may precede it,
// and a `[:class:]`, `[.collating.]` or `[=equivalence=]` carries a `]` of its
// own that does not end the set. An unterminated `[` is treated as the one
// character it is, which leaves the rest of the pattern scanned rather than
// skipped — the engine will refuse such a pattern anyway, and stopping the
// scan there would be the one way this could miss a construct that follows.
func skipBracketExpression(pattern string, i int) int {
	j := i + 1
	if j < len(pattern) && pattern[j] == '^' {
		j++
	}
	if j < len(pattern) && pattern[j] == ']' {
		j++
	}
	for j < len(pattern) {
		switch {
		case pattern[j] == '\\' && j+1 < len(pattern):
			j += 2
		case pattern[j] == '[' && j+1 < len(pattern) &&
			(pattern[j+1] == ':' || pattern[j+1] == '.' || pattern[j+1] == '='):
			k := strings.Index(pattern[j+2:], string(pattern[j+1])+"]")
			if k < 0 {
				return i + 1
			}
			j += 2 + k + 2
		case pattern[j] == ']':
			return j + 1
		default:
			j++
		}
	}
	return i + 1
}

// matchTilde answers a pattern carrying a `~(…)` prefix.
//
// The anchors are asked about the *subject* and not about the piece, which is
// the same rule the zsh `(#s)` and `(#e)` assertions follow and for the same
// reason: a trim hands over a prefix of the subject, and "the match is at the
// start" is a question about where that prefix sits.
func matchTilde(m tildeModifier, pattern, piece, subject string, base int, o patternOpts) (bool, matchReport) {
	if m.flavor == tildeNever {
		return false, matchReport{}
	}
	if m.left && base != 0 {
		return false, matchReport{}
	}
	if m.right && base+len(piece) != len(subject) {
		return false, matchReport{}
	}
	if m.flavor == tildeGlob {
		// The default, and the one flavor that is this shell's own matcher:
		// the letters left to honor here are `i`, which is the same fold the
		// run-time options ask for and reaches one place further than they
		// do. Measured 2026-09-14 on ksh93u+, `~(i)[[:lower:]]` matches `A`
		// and `~(i)[a-z]` matches it too, where bash's `nocasematch` folds
		// only the range — so this is the one caller that sets foldClass,
		// and patternOpts.foldClass has the table (#2716).
		o.fold = o.fold || m.fold
		o.foldClass = o.foldClass || m.fold
		return matchPatternIn(pattern, piece, subject, base, o)
	}
	x, ok := m.tildeRegex(pattern, o.whole)
	if !ok {
		return false, matchReport{}
	}
	return x.match(piece), matchReport{}
}

// tildeModifierOpts turns the reading on for a dialect that has the construct,
// and refuses a letter that shell has and this one does not.
//
// The refusal is the shape extendedPatternOpts already uses for a `(#…)` flag
// this matcher cannot answer: report it by name, and stop rather than return
// an answer that would be a guess.
func (r *Runner) tildeModifierOpts(o patternOpts, pattern string) patternOpts {
	if !r.dialect().TildeGroup {
		return o
	}
	o.tilde = true
	body, rest, ok := splitTildeModifier(pattern)
	if !ok {
		return o
	}
	m, unhonored := readTildeModifier(body)
	if unhonored != 0 {
		r.diagf("%s: the ~(%c) pattern modifier is not implemented\n", pattern, unhonored)
		r.status = 1
		r.stopTheShell()
		return o
	}
	// The same refusal for a construct rather than a letter, and in the same
	// place on purpose: this is the one route a `~(…)` pattern takes to the
	// matcher, so a second scan somewhere nearer the compile would be a
	// second thing to keep in step. See unsupportedTilde for what is refused
	// and why a silent `no` was the wrong answer (#3894).
	if bad := m.unsupportedTilde(rest); bad != "" {
		r.diagf("%s: the %s is not implemented\n", pattern, bad)
		r.status = 1
		r.stopTheShell()
	}
	return o
}

// tildePrefixModifier reads a `~(…)` prefix off the front of a pattern, for a
// caller outside the matcher that needs to know what the letters asked for.
//
// False where the dialect has no such group, where the pattern carries none,
// and where the group holds a letter this shell does not answer — that last
// one because the refusal belongs to Runner.tildeModifierOpts, which says
// which letter it was, and a caller here guessing at the group's meaning
// first is how the two would come to disagree.
func tildePrefixModifier(pattern string, hasGroup bool) (tildeModifier, bool) {
	if !hasGroup {
		return tildeModifier{}, false
	}
	body, _, ok := splitTildeModifier(pattern)
	if !ok {
		return tildeModifier{}, false
	}
	m, unhonored := readTildeModifier(body)
	if unhonored != 0 {
		return tildeModifier{}, false
	}
	return m, true
}

// tildeGreedyTrim reports whether `~(g)` makes this trim take the longest
// piece its pattern will match.
//
// `g` is the match taking as much subject as it can from where it begins, and
// a trim is the only surface where that is visible: a whole-subject match has
// no length to choose and a replacement here already takes the longest span.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-19, with `v=aXbXc` and
// `w=abcabc`:
//
//	${v#~(g)*X}   c       where ${v#*X} is bXc and ${v##*X} is c
//	${w#~(g)a*b}  c       where ${w#a*b} is cabc and ${w##a*b} is c
//	${v#~(g)*}    empty   where ${v#*} is the whole value
//	${v%~(g)X*}   aXb     where ${v%X*} is aXb and ${v%%X*} is a
//	${w%~(g)b*c}  abca    where ${w%b*c} is abca and ${w%%b*c} is a
//
// So it reaches a **prefix** trim and not a suffix one, and that is the same
// rule rather than two: a prefix trim is pinned at the start of the value, so
// the far end is free and greed lengthens the piece; a suffix trim is pinned
// at the end, so what greed would lengthen is already fixed and the shell
// still takes the suffix that begins latest.
func tildeGreedyTrim(pattern string, prefix bool, o patternOpts) bool {
	if !prefix {
		return false
	}
	m, ok := tildePrefixModifier(pattern, o.tilde)
	return ok && m.greedy
}

// tildeGlobPattern reports whether a field carries a `~(…)` group that makes
// it a **pattern** for pathname expansion, and what that group asked for.
//
// A name with no metacharacter in it is not a pattern and never reaches the
// filesystem — which is why `~(N)zzz` was passed through as the six
// characters it was written as, and why `~(i)A.TXT` found nothing in a
// directory holding `a.txt`. The group is what makes it one, measured on
// ksh93u+ 2012-08-01, 2026-09-19, in a directory holding `a.txt` and `b.txt`:
//
//	~(i)a.txt   a.txt        the group sent a name to the filesystem
//	~(E)a.txt   a.txt        and so does every other flavor
//	~(F)a.txt   a.txt
//	~(N)a       deleted      nothing is named `a`
//	~(N)zz      deleted
//	~()a.txt    ~()a.txt     an **empty** group does not
//	~(E)zzz     ~(E)zzz      a miss without `N` stands as it was written
//
// The empty-group row is why this asks for a body and not just for a group.
//
// True for a letter this shell does not answer as well, deliberately: the
// field then reaches the matcher and Runner.tildeModifierOpts refuses it by
// name, which is what `~(G)a*` already did. Declining it here instead would
// leave `~(G)a.txt` as a silent literal, and a pattern that quietly means
// something else is the shape this repository minds most.
//
// **A remainder holding a `/` is left out**, and that is a limit rather than
// a reading. The group is the whole field's there and the walk matches one
// component at a time, so honoring it would give the letters to the first
// component and to no other — and the reference shell does not answer that
// shape consistently enough to copy. Measured the same day from
// `/tmp/k3186`, holding `Sub/C.txt`: `~(N)Sub/C.txt` and
// `~(N)/tmp/k3186/Sub/C.txt` both name the file, while `~(E)Su./C..xt` and
// `~(i)/tmp/k3186/sub/c.txt` are each the characters they were written with
// even though the flavor and the fold would match every component. So a
// field with a `/` in it behaves exactly as it did before any of these
// letters was read, which is the one answer here that cannot be a new wrong
// one. See #3186.
func tildeGlobPattern(field string, hasGroup bool) (tildeModifier, bool) {
	if !hasGroup {
		return tildeModifier{}, false
	}
	body, rest, ok := splitTildeModifier(field)
	if !ok || body == "" || strings.Contains(rest, "/") {
		return tildeModifier{}, false
	}
	m, _ := readTildeModifier(body)
	return m, true
}
