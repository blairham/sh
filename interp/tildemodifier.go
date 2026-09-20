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
// `E`, `F`, `L`, `K`, `N`, `g`, `i`, `l`, `p`, `r`, `s`, the `+`/`-` toggles
// and the empty group. The regular-expression flavors go to Go's `regexp`,
// which is the engine this shell already compiles `=~` with.
//
// The rest are **refused by name rather than accepted and ignored**. `A`, `B`,
// `P`, `V` and `X` each agree with `E` on every probe above, which is not
// evidence that they *are* `E`; and no probe here gives `M`, `O`, `S`, `U`,
// `a`, `m` or `x` anything to do. A flag taken and dropped is worse than one
// refused, because a pattern that silently means something else is a wrong
// answer at status 0.
//
// `G` and `V` are refused for a reason of a different kind, and it is the one
// thing here that is not a matter of effort. Measured 2026-09-18,
// `[[ abab == ~(G)\(ab\)\1 ]]` matches there and `[[ abcd == ~(G)\(ab\)\1 ]]`
// does not, with `[[ abcd == ~(G)\(ab\)cd ]]` as the control that says the
// group parses: the flavor has **backreferences**. Go's `regexp` is RE2 and
// has none — `regexp.Compile` refuses `(ab)\1` outright — so that flavor
// cannot be written on the engine every other one here already uses, and the
// same two probes answer `no` under `~(E)` and `~(X)`, which says it is `G`'s alone.
// See #3186.
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
//   - `G`, `P`, `V` and `X`, for the reason given above: one of them needs
//     backreferences, which the engine the others would use does not have.

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
const honoredTildeLetters = "EFKLNgilprs"

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
	// The same flag the `=~` operator compiles under, and for the same
	// reason: this flavor is a POSIX ERE too, so a newline in the subject is
	// ordinary ground. See regexDotAll.
	b.WriteString(regexDotAll)
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
		// run-time options ask for and reaches one place further than they
		// do. Measured 2026-09-14 on ksh93u+, `~(i)[[:lower:]]` matches `A`
		// and `~(i)[a-z]` matches it too, where bash's `nocasematch` folds
		// only the range — so this is the one caller that sets foldClass,
		// and patternOpts.foldClass has the table (#2716).
		o.fold = o.fold || m.fold
		o.foldClass = o.foldClass || m.fold
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
