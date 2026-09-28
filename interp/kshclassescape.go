// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/blairham/sh/syntax"
)

// The six escapes ksh93's own glob reads as **character classes** once a
// `~(K)` group has named that glob, and what each of them matches.
//
// `K` names the language a pattern with no prefix is already in, so a group
// holding it is consumed and what follows is an ordinary glob — and it is
// also the one letter that makes a written backslash a class. Measured
// 2026-09-27 against /bin/ksh `Version AJM 93u+ 2012-08-01`, `-c` under
// `env -i` with a scratch `HOME`, **each letter as a pair**, because a class
// reading and a literal reading agree on half of all subjects:
//
//	[[ za1b == z~(K)a\db ]]    yes     [[ zadb == z~(K)a\db ]]    no
//	[[ zadb == z~(K)a\Db ]]    yes     [[ za1b == z~(K)a\Db ]]    no
//	[[ za_b == z~(K)a\wb ]]    yes     [[ 'za-b' == z~(K)a\wb ]]  no
//	[[ 'za.b' == z~(K)a\Wb ]]  yes     [[ za1b == z~(K)a\Wb ]]    no
//	[[ 'za b' == z~(K)a\sb ]]  yes     [[ za1b == z~(K)a\sb ]]    no
//	[[ za1b == z~(K)a\Sb ]]    yes     [[ 'za b' == z~(K)a\Sb ]]  no
//
// **The letter is what decides and not the language.** `~(p)` and `~(s)`
// name the same glob, `~(i)` is the fold and `~()` asks nothing, and none of
// the four changes the answer: `[[ za1b == z~(p)a\db ]]` and its three
// siblings are all no, as is `[[ za1b == za\db ]]` with no group at all. So
// this is `K` alone.
//
// It is also **positional**, which is the row that says the group is read
// where it stands rather than applied to the whole pattern: a `~(K)` written
// *behind* the escape does not reach it — `[[ za1b == za\db~(K) ]]` is no
// there and `[[ zadb == za\db~(K) ]]` is yes.
const kshClassEscapes = "dDwWsS"

// kshClassEscape reports whether the character behind a backslash names one
// of those six classes.
func kshClassEscape(c byte) bool {
	return strings.IndexByte(kshClassEscapes, c) >= 0
}

// matchKshClassEscape reports whether the unit u is in the class the letter
// names.
//
// **One unit and not one byte**, and the two are separated by the locale
// rather than by a choice: measured 2026-09-27 with `LC_ALL` unset and again
// at `en_US.UTF-8`, where a `é` is two bytes and one character.
//
//	                             C locale   en_US.UTF-8
//	[[ 'zaéb' == z~(K)a\Db ]]     no         **yes**
//	[[ 'zaéb' == z~(K)a\Wb ]]     no         no
//
// The pair is what fixes the reading and rules out the other two. Matching
// one *byte* answers the first row no under both locales, since the two bytes
// do not fit the one position. Matching one unit and asking about its first
// byte answers the **second** row yes under UTF-8, since `0xC3` is not a word
// character — where that shell says no, the character being a letter there.
// So the width is the locale's unit and the class is asked of the character.
//
// The classes are Unicode's, which is what makes the second row come out: a
// `é` is a letter, so `\W` does not match it. Under a locale with no
// multibyte characters a unit is a byte and an invalid one decodes to the
// replacement character, which is in none of the six — so the first row
// consumes one byte of the two and the `b` behind it has nothing to match.
func matchKshClassEscape(letter byte, u string) bool {
	r, _ := utf8.DecodeRuneInString(u)
	switch letter {
	case 'd':
		return unicode.IsDigit(r)
	case 'D':
		return !unicode.IsDigit(r)
	case 'w':
		return kshWordRune(r)
	case 'W':
		return !matchKshClassEscape('w', u)
	case 's':
		return unicode.IsSpace(r)
	case 'S':
		return !unicode.IsSpace(r)
	}
	return false
}

// kshGlobControlEscape is the character one of ksh93's glob **control**
// escapes names, and whether the letter is one of them.
//
// Not to be read against [kshControlEscape] in interp/printfquote.go, which
// is the other direction and another surface: that one is how `printf %q`
// *writes* a byte, and its note that this shell "has no `\v`" is about that
// spelling. A `~(K)` glob reads `\v` as a vertical tab all the same, which is
// measured below — two surfaces, two answers, no contradiction.
//
// A table of its own beside [kshClassEscapes] rather than eight more entries
// in it, because it is a different shape: a class names a *set* and consumes
// whatever unit is in it, and one of these names exactly one character and
// consumes that character or nothing.
//
// Measured 2026-09-28 against /bin/ksh `Version AJM 93u+ 2012-08-01`, each
// probe from a script file under `env -i PATH=/usr/bin:/bin` with a scratch
// `HOME`, **each letter as a pair** — a control reading and a letter reading
// agree on half of all subjects, so a row saying only that `z~(K)a\nb` fails
// to match `zanb` would pass for a pattern that matched nothing at all:
//
//	letter   `[[ zanb == z~(K)a\nb ]]`   the letter   what it names
//	\n        no                          LF           newline
//	\t        no                          TAB          tab
//	\r        no                          CR           carriage return
//	\f        no                          FF           form feed
//	\v        no                          VT           vertical tab
//	\a        no                          BEL          alert
//	\e        no                          ESC          escape
//	\E        no                          ESC          the same character
//
// The "what it names" column is a row of its own for each letter — the
// subject built with `$'za\nb'` and matched against the same pattern, yes in
// every one.
//
// **Two letters are deliberately not here**, and both were measured rather
// than passed over:
//
//   - `\0` is the **letter** `0` — `[[ za0b == z~(K)a\0b ]]` is yes there —
//     so it is not a numeric escape in this reading.
//   - `\b` matches **neither**. Not the letter `b`, not a backspace, not a
//     zero-width anything: `zabb`, `$'za\bb'`, `zab` and `$'za\b'` are all no
//     against `z~(K)a\bb`, while `$'za\bb'` against `z~(K)a?b` is yes, which
//     is the control that says the subject is one matchable character wide.
//     This column already answers no to every one of those, so it is not a
//     divergence — it is a letter neither shell has a use for, and writing a
//     guess for it is exactly where a table agrees with the reference for the
//     wrong reason.
//
// The escapes are ASCII, so one names one **byte** where a class takes a
// whole unit. A multi-byte character can equal none of them.
func kshGlobControlEscape(c byte) (byte, bool) {
	switch c {
	case 'n':
		return '\n', true
	case 't':
		return '\t', true
	case 'r':
		return '\r', true
	case 'f':
		return '\f', true
	case 'v':
		return '\v', true
	case 'a':
		return '\a', true
	case 'e', 'E':
		// Both spell escape, measured: `$'\e'` and `$'\E'` are both byte 27
		// there, and both patterns match a subject holding one.
		return 0x1b, true
	}
	return 0, false
}

// kshWordRune is what `\w` counts as a word character, named once because
// three readers need the same answer: the class escape above, and the word
// boundary and its complement below. Written out twice, the boundary would
// be the copy a later correction to the class failed to reach.
func kshWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// kshGlobZeroWidth reports whether the character behind a backslash is one of
// ksh93's glob **zero-width** escapes, which consume no subject at all.
//
// A third family beside the classes and the control characters, and a third
// shape: a class names a set and takes a unit, a control escape names one
// character and takes one byte, and one of these takes nothing and asks about
// the *position* instead.
//
// `\b` is a word boundary, `\B` its complement and `\z` the end. Measured
// 2026-09-28 against /bin/ksh `Version AJM 93u+ 2012-08-01` (`go version -m`
// says *not a Go executable*), each probe from a script file under
// `env -i PATH=/usr/bin:/bin` with a scratch `HOME` and `LC_ALL` named:
//
//	[[ zab == z~(K)ab\b ]]      yes   a boundary after the last letter
//	[[ 'za b' == z~(K)a\b?b ]]  yes   and between a letter and a space
//	[[ 'za.b' == z~(K)a\b.b ]]  yes   and before a period
//	[[ zab == z~(K)a\bb ]]      no    two word characters have none between
//	[[ zabb == z~(K)a\bb ]]     no    so it is not the letter `b`
//	[[ $'za\bb' == z~(K)a\bb ]] no    nor a backspace
//
//	[[ zab == z~(K)a\Bb ]]      yes   the complement, where `\b` says no
//	[[ zaBb == z~(K)a\Bb ]]     no    and not the letter `B`
//	[[ zaXb == z~(K)a\Bb ]]     no
//
//	[[ zazb == z~(K)a\zb ]]     no    `\z` is not the letter `z`
//	[[ ab == ~(K)ab\z ]]        yes   it is the end
//	[[ abz == ~(K)ab\z ]]       no
//
// **Each is a pair**, because an assertion that held everywhere and one that
// held nowhere would each agree with this on half the rows. The `\b` and `\B`
// rows are the same subject `zab` answered oppositely, which is what says
// they are complements rather than two spellings of "matches nothing".
//
// `[[ $'za\bb' == z~(K)a?b ]]` matches here and there, and is the control
// that says a backspace subject is one matchable character wide — so the
// sixth row is `\b` declining it rather than the subject being unreachable.
// `[[ zaqb == z~(K)a\qb ]]` matches in both columns, and is the control for
// a letter **neither** family names: the backslash is not being swallowed
// wholesale.
func kshGlobZeroWidth(c byte) (byte, bool) {
	switch c {
	case 'b', 'B', 'z':
		return c, true
	}
	return 0, false
}

// kshGlobEscape reports whether the character behind a backslash is one the
// `~(K)` reading gives a meaning to at all — either of the two families.
func kshGlobEscape(c byte) bool {
	if kshClassEscape(c) {
		return true
	}
	if _, ok := kshGlobControlEscape(c); ok {
		return true
	}
	_, ok := kshGlobZeroWidth(c)
	return ok
}

// kshClassEscapeSpan reports whether this span is a backslash the script
// wrote in front of a letter either family names.
//
// The span kind is the whole of the reading, exactly as it is for
// tildeKeepsBackslash: a backslash written in front of a character is its own
// [syntax.Quoting] rather than text, which is what makes the construct
// visible here and invisible one step later.
func kshClassEscapeSpan(s syntax.Span) bool {
	return s.Kind == syntax.Literal && s.Quoting == syntax.BackslashQuoted &&
		len(s.Value) == 1 && kshGlobEscape(s.Value[0])
}

// tildeGlobClasses reports whether any span of the word carries a `~(K)`
// group, which is what makes a written class escape worth keeping the
// backslash on.
//
// **Not positional, deliberately.** Where the group stands decides whether
// the escape behind it is a class, and the *walk* answers that as it consumes
// the group — `[[ za1b == za\db~(K) ]]` does not match, because the reading
// is still off when the escape is reached. So all this has to do is stop a
// word with no such group anywhere from having its escapes rewritten, which
// is what keeps every other pattern reading exactly as it did.
//
// Unquoted literal text only, which is the same rule findTildeFlavorGroup
// reads a flavor group by: a group that arrived quoted is the characters it
// was written with, measured — `[[ za1b == z"~(K)"a\db ]]` does not match
// there. A group that arrived from a *value* needs nothing from this at all:
// an unquoted expansion's text is written into the pattern whole, backslash
// and group together, so `p='z~(K)a\db'; [[ za1b == $p ]]` matches without
// any span here being a written escape.
func tildeGlobClasses(spans []syntax.Span, hasGroup bool) bool {
	if !hasGroup {
		return false
	}
	for _, s := range spans {
		if tildeGlobClassesIn(s) {
			return true
		}
	}
	return false
}

func tildeGlobClassesIn(s syntax.Span) bool {
	if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted {
		return false
	}
	for i := 0; i < len(s.Value); i++ {
		if s.Value[i] == '\\' {
			i++
			continue
		}
		if s.Value[i] != '~' {
			continue
		}
		body, _, ok := splitTildeModifier(s.Value[i:])
		if !ok {
			continue
		}
		if m, unhonored := readTildeModifier(body); unhonored == 0 && m.classes {
			return true
		}
	}
	return false
}

// kshZeroWidthHolds reports whether the assertion the letter names is true
// at this position. `at` is absolute into the subject, and base and end are
// where the **piece** the surface handed over begins and ends.
//
// **The piece and not the subject**, which is measured rather than assumed.
// On a whole-subject surface the two are the same and no row can tell them
// apart; `${v%…}` is the one span-choosing surface that reads a `~(K)` group
// at all — see tildeClassesHere — and it is where they part. Measured
// 2026-09-28:
//
//	v=aab; ${v%~(K)\bab}   a     the piece `ab` starts where the *subject*
//	                              has an `a` in front of it, so nothing
//	                              subject-relative would trim
//	v=aab; ${v%~(K)\Bab}   aab   and the complement declines it
//	v=ab;  ${v%~(K)\bb}    a     the same fact one character over
//	v=ab;  ${v%~(K)\Bb}    ab
//	v=ab;  ${v%~(K)a\bb}   ab    while inside the piece both readings agree
//	v=ab;  ${v%~(K)a\Bb}   empty
//
// **The piece's front is a boundary whatever stands at it**, which is the
// asymmetry and is measured twice over. `[[ '.' == ~(K)\b. ]]` matches and
// `[[ '.' == ~(K)\B. ]]` does not, where a period is a non-word character on
// both sides of that position; and `v=..b; ${v%~(K)\b.b}` trims to `.`,
// where the piece starts at a period *and* has a period in front of it in
// the subject, so neither "absent counts as non-word" nor a subject-relative
// reading would hold. The **back** is not special the same way:
// `[[ '.' == ~(K).\b ]]` does not match while `[[ a == ~(K)a\b ]]` does, so
// the end follows the ordinary rule with the absent character counting as a
// non-word one.
//
// `\z` is the end of the piece, and the end of the *subject* is a reading no
// row separates from it: the one surface that chooses a span and reads the
// group is a suffix trim, whose piece always ends where the subject does.
// Written as the piece's for the same reason `\b` is — one rule for the
// family — and recorded here as undecided rather than measured.
func kshZeroWidthHolds(letter byte, o *patternOpts, subject string, at, base, end int) bool {
	switch letter {
	case 'z':
		return at == end
	case 'b', 'B':
		// A boundary is a word character on exactly one side, so the two
		// letters are one test and its negation.
		return kshWordBoundaryAt(o, subject, at, base, end) == (letter == 'b')
	}
	return false
}

// kshWordBoundaryAt reports whether the piece has a word boundary at this
// position.
//
// **The two sides are read differently, and that is the reference's and not
// a shortcut.** The character *at* the position is one unit, exactly as a
// class escape takes one — so it is the locale that decides how wide it is.
// The one *behind* the position is read as a single **byte**, so a trailing
// byte of a multi-byte character is not a word character even in a locale
// where the character is. Measured 2026-09-28 at `LC_ALL=C` and again at
// `en_US.UTF-8`, with `é` two bytes and one character:
//
//	                            C     en_US.UTF-8
//	[[ 'aéb' == ~(K)a\béb ]]    yes   **no**    the unit ahead is `é`, a
//	                                            letter, only where a unit is
//	                                            a character
//	[[ 'aéb' == ~(K)a\Béb ]]    no    **yes**
//	[[ 'aéb' == ~(K)aé\bb ]]    yes   yes       and the byte behind is
//	                                            `0xA9` in both
//	[[ 'éé' == ~(K)é\bé ]]      no    **yes**
//	[[ '.é' == ~(K).\bé ]]      no    **yes**
//	[[ 'é' == ~(K)é\B ]]        yes   yes       a boundary at the end would
//	                                            make this fail
//
// The third and sixth rows are what fix the *behind* side: reading it as a
// character would make `é` a word character under `en_US.UTF-8` and answer
// both of them the other way, and the first two rows would still come out
// right — so a probe that varied only the locale and only looked ahead would
// confirm the wrong rule.
func kshWordBoundaryAt(o *patternOpts, subject string, at, base, end int) bool {
	if at == base {
		return true
	}
	// One byte, decoded on its own: a continuation byte is not valid UTF-8
	// and decodes to the replacement character, which is in no class.
	prev, _ := utf8.DecodeRuneInString(subject[at-1 : at])
	before := kshWordRune(prev)
	after := false
	if at < end {
		u := subject[at:end]
		next, _ := utf8.DecodeRuneInString(u[:o.unitWidth(u)])
		after = kshWordRune(next)
	}
	return before != after
}

// patternHasZeroWidthEscape reports whether a pattern spells one of the three
// zero-width escapes anywhere in it, and is asked only to decide whether the
// match memo has to be dropped when the piece moves — see matchPatternIn.
//
// The spelling and not the reading: a pattern with no `~(K)` group in front
// of the escape reads it as a letter and would not care, but asking that
// here would mean reading the group twice and would go stale the day another
// letter turns the classes on. A backslash is what the question is really
// about, and most patterns have none at all.
func patternHasZeroWidthEscape(pattern string) bool {
	for i := 0; i+1 < len(pattern); i++ {
		if pattern[i] != '\\' {
			continue
		}
		if _, ok := kshGlobZeroWidth(pattern[i+1]); ok {
			return true
		}
		i++
	}
	return false
}
