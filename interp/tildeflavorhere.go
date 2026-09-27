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
// **It refuses rather than approximating**, which is the rule
// unsupportedERE and #3186's four letters already follow: a construct that
// arrives here and is silently dropped answers a plausible `no` at status 0,
// and that is the shape this repository minds most. A pattern group, a `(#…)`
// flag group, a `{n,m}(…)` count and a second `~(` are each named and stop
// the script rather than being flattened into text they are not.
func globToRE2(glob string) (expr, unsupported string) {
	var b strings.Builder
	for i := 0; i < len(glob); i++ {
		switch c := glob[i]; c {
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
			return "", "a pattern group in front of a `~(…)` flavor group"
		case ')':
			return "", "an unopened `)` in front of a `~(…)` flavor group"
		case '~':
			return "", "a second `~(…)` group in front of a flavor group"
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return b.String(), ""
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
