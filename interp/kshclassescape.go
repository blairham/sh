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
		return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
	case 'W':
		return !matchKshClassEscape('w', u)
	case 's':
		return unicode.IsSpace(r)
	case 'S':
		return !unicode.IsSpace(r)
	}
	return false
}

// kshClassEscapeSpan reports whether this span is a backslash the script
// wrote in front of one of the six class letters.
//
// The span kind is the whole of the reading, exactly as it is for
// tildeKeepsBackslash: a backslash written in front of a character is its own
// [syntax.Quoting] rather than text, which is what makes the construct
// visible here and invisible one step later.
func kshClassEscapeSpan(s syntax.Span) bool {
	return s.Kind == syntax.Literal && s.Quoting == syntax.BackslashQuoted &&
		len(s.Value) == 1 && kshClassEscape(s.Value[0])
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
