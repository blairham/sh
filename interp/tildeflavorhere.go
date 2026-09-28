// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"regexp"
	"strings"
)

// findTildeFlavorGroup finds the first `~(…)` group in a pattern that names a
// **flavor**, and cuts the pattern in three: what stands in front of it, the
// group's body, and what stands behind.
//
// A group carrying only the option letters is not one of these — that one is
// read by the walk where it stands, and interp/tildemidpattern.go has it. A
// flavor is different in kind: it says what *language* the rest of the
// pattern is written in, so it cannot be a flag the walk carries down a
// branch and has to be settled before any matching begins.
//
// Measured 2026-09-27 against /bin/ksh `Version AJM 93u+ 2012-08-01`, each
// case under `env -i` with a scratch `HOME`:
//
//	                               ksh93u+   why the row is here
//	[[ zab == ~(E)z.b ]] (control) yes       a flavor at the head is read
//	[[ zab == ~(E)a ]]   (control) yes       and matches a *substring*
//	[[ zA == z~(E)A ]]             yes       and one in the middle is read
//	[[ zAB == z~(E)A ]]            yes
//	[[ zXA == z~(E)A ]]            **no**
//	[[ zzA == z~(E)A ]]            yes
//
// The last three are the rows that say **what the group does to the text in
// front of it**, and they rule out every reading but one. It is not a split
// with the expression anchored where the glob stopped — that answers row six
// no. It is not a split with the expression searched in what is left — that
// answers row five yes. What fits all three is that the glob in front is
// **translated into the flavor's language** and the whole is matched the way
// that flavor is matched, which for ksh93's expressions is a substring
// search: `z` and `A` become the expression `zA`, which is in `zzA` and is
// not in `zXA`.
//
// The glob in front keeps its glob meaning through the translation and does
// not become the flavor's text, which is the other half and is measured
// apart: `[[ zXA == z.~(E)A ]]` is **no** and `[[ 'z.A' == z.~(E)A ]]` is
// yes, so the `.` is a literal period; `[[ zaaa == za+~(E)a ]]` is **no** and
// `[[ 'za+a' == za+~(E)a ]]` is yes, so the `+` is a literal plus. Both would
// go the other way if the flavor reached backward over the whole pattern. See
// globToRE2, which is that translation.
//
// The scan steps over an escaped `\~` and reads the first group it finds, so
// a second one behind a flavor is that flavor's own text — measured,
// `[[ zA == z~(E)~(i)a ]]` is no, the `~(i)` having been read by the
// expression rather than by this shell.
func findTildeFlavorGroup(pattern string) (before, body, after string, ok bool) {
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '\\' {
			i++
			continue
		}
		if pattern[i] != '~' {
			continue
		}
		b, rest, split := splitTildeModifier(pattern[i:])
		if !split {
			continue
		}
		m, unhonored := readTildeModifier(b)
		if unhonored != 0 || m.flavor == tildeNever || m.flavor == tildeGlob {
			// A letter this shell does not answer is named and refused by
			// Runner.tildeModifierOpts rather than guessed at here, and a
			// group that leaves the flavor where it was is not this one's:
			// the pattern is still a glob, so the walk reads it.
			continue
		}
		return pattern[:i], b, rest, true
	}
	return "", "", "", false
}

// globToRE2 renders the glob standing in front of a flavor group as the
// regular expression that matches the same text, and names the first
// construct it cannot carry.
//
// `*` and `?` are the two that mean something, a bracket expression means the
// same in both languages once `!` is written as `^`, and every other
// character is quoted so that a glob's literal `.` or `+` stays literal —
// which is the half `[[ zXA == z.~(E)A ]]` measures.
//
// **A quantified pattern group is translated and the rest are refused.**
// `@(…)`, `?(…)`, `*(…)` and `+(…)` are a group and a bound, which is what
// #4939 settled, and RE2 spells every one of the four — so the group becomes
// a non-capturing group carrying the quantifier the letter stands for, with
// each arm translated by this same function. `!(…)` is a **complement** and
// RE2 has none, a `{n,m}(…)` count is a construct this matcher does not have
// at all, and a bare `(` and a second `~(` are what they were.
//
// **What is left refuses rather than approximating**, which is the rule
// unsupportedERE and #3186's four letters already follow: a construct that
// arrives here and is silently dropped answers a plausible `no` at status 0,
// and that is the shape this repository minds most. The caller does not
// print the reason — it declines to claim the pattern, so the walk answers
// exactly what it answered before — but naming the construct is what keeps
// the two apart for the next reader.
func globToRE2(glob string) (expr, unsupported string) {
	var b strings.Builder
	for i := 0; i < len(glob); i++ {
		c := glob[i]
		if group, quant, rest, ok := splitGlobGroup(glob, i); ok {
			if quant == '!' {
				return "", "a `!(…)` pattern group in front of a `~(…)` flavor group"
			}
			arms := make([]string, 0, 4)
			for _, arm := range globGroupArms(group) {
				translated, bad := globToRE2(arm)
				if bad != "" {
					return "", bad
				}
				arms = append(arms, translated)
			}
			b.WriteString("(?:" + strings.Join(arms, "|") + ")")
			b.WriteString(re2Quantifier(quant))
			i = len(glob) - len(rest) - 1
			continue
		}
		switch c {
		case '\\':
			if i+1 >= len(glob) {
				return "", `a backslash with nothing behind it`
			}
			b.WriteString(regexp.QuoteMeta(glob[i+1 : i+2]))
			i++
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		case '[':
			end := globBracketEnd(glob, i)
			if end < 0 {
				// An unterminated `[` is an ordinary character in every
				// shell's glob, so it is quoted rather than refused.
				b.WriteString(`\[`)
				continue
			}
			set := glob[i : end+1]
			if strings.HasPrefix(set, "[!") {
				set = "[^" + set[2:]
			}
			b.WriteString(set)
			i = end
		case '(':
			// A `(` with no quantifier in front of it, which covers the
			// `{n,m}(…)` count as well: that one is a construct this
			// matcher does not answer anywhere, so translating it here
			// would be inventing a reading — `[[ zza == {2}(z)a ]]`
			// matches in ksh93u+ and does not here, group or no group.
			return "", "an unquantified `(` in front of a `~(…)` flavor group"
		case ')':
			return "", "an unopened `)` in front of a `~(…)` flavor group"
		case '~':
			flags, rest, ok := globTildeFlags(glob[i:])
			if !ok {
				return "", "a `~(…)` group in front of a flavor group that this shell does not read"
			}
			b.WriteString(flags)
			i = len(glob) - len(rest) - 1
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return b.String(), ""
}

// globTildeFlags is what a second `~(…)` group standing in the glob in front
// of a flavor group comes to in the expression: an **inline flag**, written
// where the group stands.
//
// The group is read positionally rather than applied to the whole pattern,
// which is measured. On ksh93u+ 2012-08-01, 2026-09-27, `-c` under `env -i`
// with a scratch `HOME`:
//
//	[[ zqa == z~(i)q~(E)a ]]        yes   (control) the group is read at all
//	[[ zqA == zq~(E)a ]]            no    (control) and without it nothing folds
//	[[ zQA == z~(i)q~(E)a ]]        yes   so the fold reaches the glob behind it
//	[[ zqA == z~(i)q~(E)A ]]        yes   **and the expression as well**
//	[[ zQa == z~(-i)q~(E)a ]]       no    and the sign turns it off again
//	[[ zQa == ~(i)z~(-i)q~(E)a ]]   no    from where the second group stands
//
// The third and fourth rows are the pair that says an inline flag is the
// right shape rather than a flag on the compile: one reaches the text in
// front of the flavor group and the other the expression behind it, which is
// what `(?i)` written at that position does and what a flag on the whole
// pattern would also do — the *last* row is what separates them, since a
// whole-pattern flag cannot be turned off halfway along.
//
// **Only the fold is written.** `l`, `r` and `g` are answered outside the
// matcher — `tildeHereAnchors` and `tildeGreedyTrim` — and `K`, `p`, `s`, `N`
// and an empty group ask nothing of a glob that is about to become an
// expression, so each of those contributes no flag. Every one of them is
// measured as *not* changing this position's answer, and each of those rows
// is non-discriminating on purpose: `[[ Xzqa == z~(l)q~(E)a ]]` and
// `[[ Xzqa == zq~(E)a ]]` are both yes there, so the row says the group was
// stepped over and says nothing about what `l` would ask for. Writing a flag
// for one of them on that evidence would be a guess.
//
// False where the group is one the walk cannot consume, which is a letter
// this shell declines — the caller then refuses the whole translation rather
// than dropping the group, for the reason #3186 gives.
func globTildeFlags(p string) (flags, rest string, ok bool) {
	g, isGroup := splitTildeHereGroup(p)
	if !isGroup {
		return "", "", false
	}
	_, rest, _ = splitTildeModifier(p)
	if !g.foldSet {
		return "", rest, true
	}
	if g.fold {
		return "(?i)", rest, true
	}
	return "(?-i)", rest, true
}

// splitGlobGroup peels a quantified pattern group off the glob at i: its body,
// the quantifier character in front of it, and what is left behind it.
//
// False for anything that is not one, which is every other byte of a glob and
// also a group nothing closes — `@(ab` is four ordinary characters in every
// shell that has the construct.
func splitGlobGroup(glob string, i int) (body string, quant byte, rest string, ok bool) {
	if i+1 >= len(glob) || glob[i+1] != '(' {
		return "", 0, "", false
	}
	switch glob[i] {
	case '@', '?', '*', '+', '!':
	default:
		return "", 0, "", false
	}
	end := globGroupEnd(glob, i+1)
	if end < 0 {
		return "", 0, "", false
	}
	return glob[i+2 : end], glob[i], glob[end+1:], true
}

// globGroupEnd is the index of the `)` closing the group whose `(` is at i, or
// -1 where nothing closes it.
//
// A bracket expression is stepped over whole, for the reason closingParen has:
// a parenthesis inside one is a member rather than nesting, and a bracket
// nothing closes reaches the end of the pattern.
func globGroupEnd(glob string, i int) int {
	depth := 0
	for j := i; j < len(glob); j++ {
		switch glob[j] {
		case '\\':
			j++
		case '[':
			end := globBracketEnd(glob, j)
			if end < 0 {
				return -1
			}
			j = end
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

// globGroupArms splits a group's body on the `|` between its arms, ignoring
// the ones inside a nested group or a bracket expression.
func globGroupArms(body string) []string {
	var arms []string
	depth, start := 0, 0
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '\\':
			i++
		case '[':
			end := globBracketEnd(body, i)
			if end < 0 {
				continue
			}
			i = end
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case '|':
			if depth == 0 {
				arms = append(arms, body[start:i])
				start = i + 1
			}
		}
	}
	return append(arms, body[start:])
}

// re2Quantifier is what each of the four group quantifiers comes to in an
// expression: a bound, exactly as boundOf reads it for the walk.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-27, `-c` under `env -i` with a
// scratch `HOME`, against `@(zq)~(E)a` and its three siblings — the row that
// separates them is the **empty** one, since a substring search finds a
// repetition wherever it sits and cannot tell one from more:
//
//	[[ a == @(zq)~(E)a ]]    no     exactly one
//	[[ a == +(zq)~(E)a ]]    no     one or more
//	[[ a == ?(zq)~(E)a ]]    yes    nought or one
//	[[ a == *(zq)~(E)a ]]    yes    nought or more
//	[[ zqa == +(zq)~(E)a ]]  yes    (control) each of the four takes one
//
// `@` against `+`, and `?` against `*`, are **not** separable at this
// position and that is a limit rather than an omission: the whole expression
// is searched, so a subject holding two repetitions holds one as well and
// `[[ zqzqa == @(zq)~(E)a ]]` matches there. The bounds written here are the
// ones boundOf already reads for the same four letters, which is the reason
// they are not guessed.
func re2Quantifier(quant byte) string {
	switch quant {
	case '?':
		return "?"
	case '*':
		return "*"
	case '+':
		return "+"
	}
	// `@`, which is exactly one and needs no quantifier at all.
	return ""
}

// globBracketEnd finds the `]` closing the bracket expression open at i, or
// -1 where nothing closes it.
//
// The first character may be `!` or `^` and the one after that may be a `]`
// standing for itself, which is the rule every shell's glob has and the one
// place a bracket's extent is not what a naive scan would say.
func globBracketEnd(glob string, i int) int {
	j := i + 1
	if j < len(glob) && (glob[j] == '!' || glob[j] == '^') {
		j++
	}
	if j < len(glob) && glob[j] == ']' {
		j++
	}
	for ; j < len(glob); j++ {
		switch glob[j] {
		case '\\':
			j++
		case '[':
			// A character class, collating symbol or equivalence class —
			// `[:alpha:]` and its two neighbors — whose closing `]` is the
			// class's and not the bracket's.
			if j+1 < len(glob) && strings.ContainsRune(":.=", rune(glob[j+1])) {
				if k := strings.Index(glob[j+2:], string(glob[j+1])+"]"); k >= 0 {
					j += 2 + k + 1
				}
			}
		case ']':
			return j
		}
	}
	return -1
}

// tildeFlavorHere matches a pattern whose flavor group stands somewhere other
// than the front, by translating the glob in front of the group and handing
// the whole to the same machinery a group at the front goes through.
//
// The second result is false where the pattern is not of that shape, so a
// caller falls through to the ordinary walk.
func matchTildeFlavorHere(pattern, piece, subject string, base int, o patternOpts) (bool, bool) {
	before, body, after, ok := findTildeFlavorGroup(pattern)
	if !ok || before == "" {
		// Nothing here, or a group at the very front — which is the shape
		// matchPatternIn already reads, and reading it twice would be two
		// places to keep in step.
		return false, false
	}
	expr, unsupported := globToRE2(before)
	if unsupported != "" {
		// **Not claimed**, so the caller falls through to the walk and the
		// pattern answers exactly what it answered before this existed. A
		// refusal by name would be the house rule if the construct were one
		// this shell had a reading for and declined to apply; what is here
		// instead is a shape nobody has measured — a flavor group inside a
		// pattern group, `@(z~(E)a)`, which ksh93u+ matches and this does
		// not. Leaving it where it was keeps one wrong answer rather than
		// trading it for a second. See #4892.
		return false, false
	}
	m, unhonored := readTildeModifier(body)
	if unhonored != 0 {
		return false, true
	}
	// A fold the options already carry is the group's too: `~(i)` at the head
	// and a flavor further along is one pattern, and the letters compose —
	// `[[ zA == ~(i)z~(E)a ]]` matches in ksh93u+. The expression's own flags
	// are written from the modifier rather than from the options, so the fold
	// has to arrive there.
	m.fold = m.fold || o.fold
	got, _ := matchTilde(m, after, piece, subject, base, withGlobPrefix(o, expr))
	return got, true
}

// withGlobPrefix carries the translated glob down to tildeRegex, which is the
// one place every flavor has already been turned into the same language.
func withGlobPrefix(o patternOpts, expr string) patternOpts {
	o.tildePrefix = expr
	return o
}

// matchTildeFlavorInPiece answers a *piece* of a pattern that carries a
// flavor group — the body of a `@(…)`, one arm of it, or the text behind one —
// against the whole of the subject text that piece was handed.
//
// matchTildeFlavorHere is the same reading for a whole pattern and is where
// the translation is argued. What this adds is the **extent**: a flavor at
// the top of a pattern is a *substring* search, since that is what ksh93's
// expressions are, and a flavor inside a pattern group is anchored to the
// span the group takes. That is not a choice, it is what the matcher's own
// contract already says — every caller of matchHere is asking whether the
// piece it handed over is described whole — and it is measured. On ksh93u+
// 2012-08-01, 2026-09-27, `-c` under `env -i` with a scratch `HOME`:
//
//	[[ zza == z~(E)a ]]      (control) yes    the top level searches
//	[[ zza == @(z~(E)a) ]]             **no**
//	[[ zXa == @(z~(E)a) ]]             no
//	[[ zab == @(z~(E)a)b ]]            yes    so the group took exactly `za`
//	[[ zaXb == @(z~(E)a)b ]]           no
//	[[ zaaq == @(z~(E)a)q ]]           no
//
// The first two are the pair: the same subject and the same flavor answer
// differently with the group around them, which is the whole of what this
// function is for. The last three say the group takes the span it describes
// and no more, which a substring search inside the group would not.
//
// `a*` inside the group is the row that says the expression is really
// compiled rather than the glob being walked: `[[ zab == @(z~(E)a*) ]]` is
// **no** there, because `a*` is zero-or-more `a` in an expression and would
// be `a` then anything in a glob.
//
// The second result is false where the piece is not of this shape — no
// flavor group in it, or a glob in front of one that globToRE2 cannot carry —
// so the caller falls through to the walk and answers exactly what it
// answered before. `@(z)~(E)a` is the shape still left out: the text in front
// of the group is a *pattern group*, which has no translation here, so a
// subject the group and the expression do not cover contiguously keeps the
// answer it had.
//
// **A trim and a substitution are left out at the call site**, and that is a
// refusal to copy rather than a gap. The reference shell's answers for this
// shape on a surface that chooses a span do not compose — with `v=abcd`:
//
//	${v#a~(E)b}        cd     (control) ungrouped, the span is removed
//	${v#@(a~(E)b)}     empty  the **whole value**, where the span is `ab`
//	${v/@(b~(E)c)/X}   X      the whole value again
//	${v%@(c~(E)d)}     abcd   and nothing at all
//
// Three answers from one shape, and no reading produces all three. Those
// surfaces therefore behave exactly as they did before this existed, which
// is the one answer here that cannot be a new wrong one — the same posture
// tildeGlobPattern takes for a field whose remainder holds a `/`.
func matchTildeFlavorInPiece(p, s string, at int, o patternOpts) (bool, bool) {
	before, body, after, ok := findTildeFlavorGroup(p)
	if !ok {
		return false, false
	}
	expr := ""
	if before != "" {
		translated, unsupported := globToRE2(before)
		if unsupported != "" {
			// Not claimed, for the reason matchTildeFlavorHere gives: a
			// shape nobody has measured keeps the answer it already had
			// rather than trading one wrong answer for a second.
			return false, false
		}
		expr = translated
	}
	// findTildeFlavorGroup has already declined a letter this shell does not
	// answer, so the group here is one of the flavors.
	m, _ := readTildeModifier(body)
	// A fold the branch arrived with is the group's too, exactly as it is for
	// a group at the top of the pattern: the expression's flags are written
	// from the modifier rather than from the options.
	m.fold = m.fold || o.fold
	// Anchored rather than searched, which is what `whole` false asks
	// tildeRegexAfter for. Every caller of matchHere wants the piece
	// described whole.
	o.whole = false
	got, _ := matchTilde(m, after, s, o.where.subject, at, withGlobPrefix(o, expr))
	return got, true
}
