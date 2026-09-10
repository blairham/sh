// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The character-class names this shell has beyond the twelve POSIX ones.
// Measured on zsh 5.9.2, 2026-09-10, `[[ $c = [[:NAME:]] ]]` a character at a
// time (#1721).
//
// The rows are hit/miss rather than a status because that is the failure this
// is about: a name the matcher does not know comes back matching nothing, at
// status 0 and with nothing said, so a guard written with it takes the branch
// for a malformed value. `gitstatus` guards its argument with
// `[[ $name != [[:IDENT:]]## ]]`, and with the name missing that guard fired
// on every well-formed name it was given.
func TestTheCharacterClassesBeyondPosix(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The startup's own line, both ways round.
		{`n=POWERLEVEL9K; [[ $n != [[:IDENT:]]## ]] && echo refused || echo passed`, "passed"},
		{`n=has-a-dash; [[ $n != [[:IDENT:]]## ]] && echo refused || echo passed`, "refused"},
		// What the name holds: letters, digits and the underscore, and
		// nothing else. It follows alnum outside ASCII rather than stopping
		// at the ASCII letters.
		{`[[ a = [[:IDENT:]] ]] && echo hit || echo miss`, "hit"},
		{`[[ 9 = [[:IDENT:]] ]] && echo hit || echo miss`, "hit"},
		{`[[ _ = [[:IDENT:]] ]] && echo hit || echo miss`, "hit"},
		{`[[ - = [[:IDENT:]] ]] && echo hit || echo miss`, "miss"},
		// Outside ASCII it follows alnum, which needs the locale that makes
		// a two-byte sequence one character — the same answer `?` gives.
		{`LC_ALL=en_US.UTF-8; [[ é = [[:IDENT:]] ]] && echo hit || echo miss`, "hit"},
		// One byte below 0x80, which is not a spelling of `print`.
		{`[[ " " = [[:ascii:]] ]] && echo hit || echo miss`, "hit"},
		{`LC_ALL=en_US.UTF-8; [[ é = [[:ascii:]] ]] && echo hit || echo miss`, "miss"},
		// And one raw byte above 0x80, which is a whole unit whatever the
		// locale says — the row a two-byte character cannot ask, because a
		// bracket matches one unit and `é` is two of them either way.
		{`b=$'\xff'; [[ $b = [[:ascii:]] ]] && echo hit || echo miss`, "miss"},
		{`b=$'\xc3'; [[ $b = [[:ascii:]] ]] && echo hit || echo miss`, "miss"},
		// The two that read shell state as it stands rather than a table.
		{`IFS=:x; [[ : = [[:IFS:]] ]] && echo hit || echo miss`, "hit"},
		{`IFS=:x; [[ " " = [[:IFS:]] ]] && echo hit || echo miss`, "miss"},
		{`IFS=:x; [[ : = [[:IFSSPACE:]] ]] && echo hit || echo miss`, "miss"},
		{`[[ " " = [[:IFSSPACE:]] ]] && echo hit || echo miss`, "hit"},
		{`WORDCHARS='@%'; [[ @ = [[:WORD:]] ]] && echo hit || echo miss`, "hit"},
		{`WORDCHARS='@%'; [[ - = [[:WORD:]] ]] && echo hit || echo miss`, "miss"},
		// A whole character of the set is a member and half of one is not:
		// the first byte of a two-byte `WORDCHARS` entry is its own unit
		// here and is not in the class. A membership test written as a
		// substring search answers hit.
		{`WORDCHARS='é'; b=$'\xc3'; [[ $b = [[:WORD:]] ]] && echo hit || echo miss`, "miss"},
		{`WORDCHARS='é'; LC_ALL=en_US.UTF-8; [[ é = [[:WORD:]] ]] && echo hit || echo miss`, "hit"},
		{`IFS=é; b=$'\xa9'; [[ $b = [[:IFS:]] ]] && echo hit || echo miss`, "miss"},
		// A byte that is not a character: one that could have begun one, and
		// one that could not.
		{`b=$'\xc3'; [[ $b = [[:INCOMPLETE:]] ]] && echo hit || echo miss`, "hit"},
		{`b=$'\xc3'; [[ $b = [[:INVALID:]] ]] && echo hit || echo miss`, "miss"},
		{`b=$'\xff'; [[ $b = [[:INVALID:]] ]] && echo hit || echo miss`, "hit"},
		{`b=$'\xff'; [[ $b = [[:INCOMPLETE:]] ]] && echo hit || echo miss`, "miss"},
		{`b=$'\x80'; [[ $b = [[:INVALID:]] ]] && echo hit || echo miss`, "hit"},
		// A byte in neither: 0xC1 is an overlong lead that could not have
		// begun a character, and 0xF5 is past the last legal one.
		{`b=$'\xc1'; [[ $b = [[:INVALID:]] ]] && echo hit || echo miss`, "hit"},
		{`b=$'\xc1'; [[ $b = [[:INCOMPLETE:]] ]] && echo hit || echo miss`, "miss"},
		{`b=$'\xf5'; [[ $b = [[:INVALID:]] ]] && echo hit || echo miss`, "hit"},
		{`b=$'\xf4'; [[ $b = [[:INCOMPLETE:]] ]] && echo hit || echo miss`, "hit"},
		{`b=$'\xc2'; [[ $b = [[:INCOMPLETE:]] ]] && echo hit || echo miss`, "hit"},
		// An ASCII byte is neither, whatever else it is.
		{`[[ a = [[:INVALID:]] ]] && echo hit || echo miss`, "miss"},
		{`[[ a = [[:INCOMPLETE:]] ]] && echo hit || echo miss`, "miss"},
		// The names are case-sensitive, and a name outside the roster is
		// unknown like any other: it matches nothing, silently, at status 0,
		// which is what every shell in the panel does with one.
		{`[[ a = [[:ident:]] ]] && echo hit || echo miss; echo "st=$?"`, "miss\nst=0"},
		{`[[ a = [[:ALPHA:]] ]] && echo hit || echo miss`, "miss"},
		{`[[ a = [[:nosuchclass:]] ]] && echo hit || echo miss`, "miss"},
		// The twelve are still the twelve, which is what says the bracket
		// grammar was never the problem.
		{`[[ a = [[:alpha:]] ]] && echo hit || echo miss`, "hit"},
	} {
		out, st := runZsh(t, t.TempDir(), "setopt extendedglob\n"+tc.src)
		if got := strings.TrimRight(out, "\n"); got != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, got, st, tc.want)
		}
	}
}

// And the default the shell starts with, which is `$WORDCHARS` itself: a dash
// and a dot are part of a word here without anything assigning one. Through
// the prelude, because that is where the variable gets its value — the same
// text the line editor's own answer is built from, so the two cannot drift.
func TestTheWordClassFollowsTheWordCharactersDefault(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`echo "[$WORDCHARS]"`, "[*?_-.[]~=/&;!#$%^(){}<>]"},
		{`[[ - = [[:WORD:]] ]] && echo hit || echo miss`, "hit"},
		{`[[ . = [[:WORD:]] ]] && echo hit || echo miss`, "hit"},
		{`[[ a = [[:WORD:]] ]] && echo hit || echo miss`, "hit"},
		{`[[ , = [[:WORD:]] ]] && echo hit || echo miss`, "miss"},
	} {
		out, st := runZshPrelude(t, t.TempDir(), "setopt extendedglob\n"+tc.src)
		if got := strings.TrimRight(out, "\n"); got != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, got, st, tc.want)
		}
	}
}

// And the same names against the filesystem, which is a different route into
// the matcher: pathname expansion builds its own options and would carry the
// roster or not carry it independently of a condition.
func TestTheExtraClassesReachPathnameExpansion(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"abc", "a_b", "9z", "x.y"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ src, want string }{
		{`echo [[:IDENT:]]##`, "9z a_b abc"},
		{`echo [[:WORD:]]##`, "9z a_b abc x.y"},
	} {
		out, st := runZshPrelude(t, dir, "setopt extendedglob\n"+tc.src)
		if got := strings.TrimRight(out, "\n"); got != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, got, st, tc.want)
		}
	}
}
