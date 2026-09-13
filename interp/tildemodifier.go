// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"regexp"
	"strings"
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
//	G       a regular expression that is not ERE — `^a.c$` and `a.c` both
//	        match and `a?c` does not, which is grep's basic syntax
//	A B P V X  each behaves as E does on every probe written here
//	F       a literal string, matching a substring: `~(F)a.c` matches the
//	        three characters `a.c` and does not match `abc`
//	L       a literal string as well, and no probe here separates it from F
//	K       the ksh glob, which is what a pattern with no prefix is
//	M N O S U a g m p s x   accepted, and no probe here makes any of them
//	        change an answer; `~(g)` does not make `${v/p/r}` global, which
//	        is the one surface a "global" letter could have shown in
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
// `E`, `F`, `L`, `K`, `i`, `l`, `r`, the `+`/`-` toggles and the empty group.
// The regular-expression flavors go to Go's `regexp`, which is the engine this
// shell already compiles `=~` with.
//
// The rest are **refused by name rather than accepted and ignored**. `G` is a
// different regular-expression syntax and translating it is work of its own;
// `A`, `B`, `P`, `V` and `X` each agree with `E` on every probe above, which
// is not evidence that they *are* `E`; and no probe here gives `M`, `N`, `O`,
// `S`, `U`, `a`, `g`, `m`, `p`, `s` or `x` anything to do. A flag taken and
// dropped is worse than one refused, because a pattern that silently means
// something else is a wrong answer at status 0.
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

// tildeFlavor is the pattern language a `~(…)` prefix selects.
type tildeFlavor uint8

const (
	// tildeGlob is `K`, and is what a pattern with no prefix already is.
	tildeGlob tildeFlavor = iota
	// tildeERE is `E`: the pattern is a POSIX extended regular expression.
	tildeERE
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
const honoredTildeLetters = "EFKLilr"

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
		case 'F', 'L':
			m.flavor = tildeLiteral
		case 'K':
			m.flavor = tildeGlob
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

// tildeRegex compiles the pattern for a flavor that is a regular expression.
//
// whole says the caller is asking about a whole subject rather than choosing
// the extent of a match: a substring search is what ksh93 does there, and an
// exact one is what a trim or a substitution needs, since those pick the span
// themselves and hand this one span to compare. Measured both ways —
// `[[ xabcx == ~(E)a.c ]]` matches, and `s=aXbXc; ${s//~(E)X/-}` is `a-b-c`
// rather than `-c`, which it would be if each span were searched.
func (m tildeModifier) tildeRegex(pattern string, whole bool) (*regexp.Regexp, bool) {
	if m.flavor == tildeLiteral {
		pattern = regexp.QuoteMeta(pattern)
	}
	var b strings.Builder
	if m.fold {
		b.WriteString("(?i)")
	}
	if !whole || m.left {
		b.WriteString(`\A`)
	}
	b.WriteString("(?:")
	b.WriteString(pattern)
	b.WriteString(")")
	if !whole || m.right {
		b.WriteString(`\z`)
	}
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, false
	}
	return re, true
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
		// run-time options ask for and folds a bracket and a class with it.
		o.fold = o.fold || m.fold
		return matchPatternIn(pattern, piece, subject, base, o)
	}
	re, ok := m.tildeRegex(pattern, o.whole)
	if !ok {
		return false, matchReport{}
	}
	return re.MatchString(piece), matchReport{}
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
	body, _, ok := splitTildeModifier(pattern)
	if !ok {
		return o
	}
	if _, unhonored := readTildeModifier(body); unhonored != 0 {
		r.diagf("%s: the ~(%c) pattern modifier is not implemented\n", pattern, unhonored)
		r.status = 1
		r.stopTheShell()
	}
	return o
}
