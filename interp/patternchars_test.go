// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// bothLocales runs src twice with the multibyte axis answered yes: once under
// a locale naming UTF-8 and once under one that names a single-byte encoding.
// The pair is the assertion — either half alone reads as a fact about the
// construct rather than about the axis.
func bothLocales(t *testing.T, src, wantUTF8, wantSingle string) {
	t.Helper()
	if out, st := runLocale(t, Yes, []string{"LC_ALL=C.UTF-8"}, src); out != wantUTF8 || st != 0 {
		t.Errorf("in UTF-8: %s gave %q (status %d), want %q at 0", src, out, st, wantUTF8)
	}
	if out, st := runLocale(t, Yes, []string{"LC_ALL=C"}, src); out != wantSingle || st != 0 {
		t.Errorf("in C: %s gave %q (status %d), want %q at 0", src, out, st, wantSingle)
	}
}

// `?` consumes one unit of the subject, and which unit that is moves with the
// locale even though the pattern is five ASCII bytes. Measured: bash 5.3, bash
// 3.2, ksh93 and zsh all answer five under a UTF-8 locale and six under C.
func TestAQuestionMarkConsumesOneUnit(t *testing.T) {
	bothLocales(t, `s=héllo; case $s in ?????) printf five;; ??????) printf six;; esac`,
		"five", "six")
}

// Trimming is the same question read through `${x#pat}`. The suffix probe is
// in the same test on purpose: it is the one #899 cited as evidence that this
// was already right, and it agrees under both locales because `héllo` ends in
// two ASCII characters — so a test that asserted only that one would pass
// against the bug.
func TestTrimmingCountsTheSameUnitAMatchDoes(t *testing.T) {
	bothLocales(t, `s=héllo; printf "[%s][%s]" "${s#???}" "${s%??}"`,
		"[lo][hél]", "[llo][hél]")
	// Two off the front rather than three, which is the one that separates
	// *where a match may start* from what it matches: the shortest split
	// that satisfies `??` is after two characters, and a scan offering every
	// byte offset would satisfy it a byte earlier — at `h` plus the lead
	// byte of `é`, leaving a continuation byte at the head of the result.
	bothLocales(t, `s=héllo; printf "[%s]" "${s#??}"`, "[llo]", "[\xa9llo]")
	// And the doubled operators, which choose the other end of the match.
	bothLocales(t, `s=héllo; printf "[%s][%s]" "${s##?*l}" "${s%%?}"`,
		"[o][héll]", "[o][héll]")
}

// Replacement walks the subject looking for a match at each position, so it
// has the unit question twice: where a match may start, and where the scan
// resumes after one.
func TestReplacementRunsOverUnits(t *testing.T) {
	bothLocales(t, `s=日本語; printf "[%s]" "${s//?/X}"`, "[XXX]", "[XXXXXXXXX]")
	bothLocales(t, `s=héllo; printf "[%s]" "${s/??/X}"`, "[Xllo]", "[X\xa9llo]")
	// The anchored forms take the same stops.
	bothLocales(t, `s=héllo; printf "[%s]" "${s/#??/X}"`, "[Xllo]", "[X\xa9llo]")
	bothLocales(t, `s=héllo; printf "[%s]" "${s/%??/X}"`, "[hélX]", "[hélX]")
}

// A bracket matches one whole unit, which is the half that is wrong in the
// direction nobody notices: a byte matcher takes `é` for the `e` it starts
// near, because `[ae]` holds neither of its bytes but a byte matcher is
// looking at only one of them at a time.
func TestABracketHoldsOneWholeUnit(t *testing.T) {
	bothLocales(t, `case é in [é]) printf lit;; *) printf no;; esac`, "lit", "no")
	bothLocales(t, `case é in [ae]) printf half;; *) printf whole;; esac`, "whole", "whole")
	// Two characters that share a lead byte, which is what a comparison
	// looking at the first byte of a unit gets wrong: è is c3 a8 and é is
	// c3 a9. A miss in every panel member.
	bothLocales(t, `case è in [é]) printf hit;; *) printf miss;; esac`, "miss", "miss")
	// A negated bracket is the same question inverted, and gets it from the
	// same place rather than from a second reading.
	bothLocales(t, `case é in [!ae]) printf other;; *) printf listed;; esac`, "other", "listed")
}

// A range's endpoints are units too, and the comparison is by code point:
// U+00E7 ç is between a and U+00E9 é as a number, and is not between them
// byte for byte, so a matcher comparing encoded forms answers this wrongly.
func TestABracketRangeIsRankedByCodePoint(t *testing.T) {
	bothLocales(t, `case ç in [a-é]) printf in;; *) printf out;; esac`, "in", "out")
	bothLocales(t, `case é in [a-ÿ]) printf in;; *) printf out;; esac`, "in", "out")
	// Outside the range on the far side, so the answer is not "any character
	// beyond ASCII is in every range".
	bothLocales(t, `case 日 in [a-ÿ]) printf in;; *) printf out;; esac`, "out", "out")
}

// The character classes outside ASCII, which is where the panel is least
// uniform. bash 5.3, bash 3.2 and zsh agree exactly and this follows them;
// ksh93 answers alpha and nothing else, and dash answers none of them.
func TestACharacterClassOutsideAscii(t *testing.T) {
	const src = `for c in alpha alnum upper lower digit space punct print graph; do case $1 in [[:$c:]]) printf "%s " "$c";; esac; done`
	for _, tc := range []struct{ ch, want string }{
		{"é", "alpha alnum lower print graph "},
		{"É", "alpha alnum upper print graph "},
		{"日", "alpha alnum print graph "},
		{"·", "punct print graph "},
	} {
		bothLocales(t, `set -- `+tc.ch+`; `+src, tc.want, "")
	}
	// ASCII is untouched by any of it, in either locale.
	bothLocales(t, `set -- a; `+src, "alpha alnum lower print graph ", "alpha alnum lower print graph ")
}

// Two classes whose answers outside ASCII are not the obvious ones, and both
// are measured rather than taken from Go's tables.
//
// `digit` holds nothing outside ASCII, though `alnum` holds the same
// character: POSIX says the digit class is the digits 0 through 9 in every
// locale, and `case ٣ in [[:digit:]]` is a miss in ksh93, zsh and dash. bash
// 5.3 and 3.2 call it a digit, and are the ones out — so this one assertion
// is what this implementation does rather than what the shell it is being
// does, which is #956 and is filed rather than settled here.
//
// `graph` excludes a space where `print` keeps it, which is the only place
// the two differ: a non-breaking space is print in bash and zsh, graph in
// neither, and space in both. Go counts it Graphic, so the space is taken
// back out by hand.
func TestTwoClassesWhoseAnswersAreNotTheObviousOnes(t *testing.T) {
	bothLocales(t, `case ٣ in [[:digit:]]) printf digit;; *) printf no;; esac`, "no", "no")
	bothLocales(t, `case ٣ in [[:alnum:]]) printf alnum;; *) printf no;; esac`, "alnum", "no")
	// And `alnum` is `Nd` and not every kind of number. `Ⅷ` U+2167 is `Nl`
	// and `½` U+00BD is `No`, and both were alphanumeric here: `No` in no
	// column of the panel at all, `Nl` in BusyBox ash alone and in none of
	// the other six (#2465). This follows the six, which is the same call
	// inWideClass already makes where ksh93 is the odd one.
	//
	// The pair is what makes the assertion discriminating. `Nl` and `No` are
	// separate categories, so a fix that dropped only one of them would pass
	// a single-character test; and neither is `Nd`, so the `٣` row above —
	// which must not move — cannot stand in for them. The panel's own answers
	// are graded by the corpus row
	// `pat/alnum-outside-ascii-is-a-letter-or-a-decimal-digit`, which is where
	// ash's disagreement is recorded; these assertions are about this
	// implementation.
	bothLocales(t, `case Ⅷ in [[:alnum:]]) printf alnum;; *) printf no;; esac`, "no", "no")
	bothLocales(t, `case ½ in [[:alnum:]]) printf alnum;; *) printf no;; esac`, "no", "no")
	// The same three characters through the classes `alnum` is named after,
	// because the bug was as much an inconsistency as a wrong answer: alpha
	// said no, digit said no, and alnum said yes. No shell in the panel
	// disagrees with itself that way.
	bothLocales(t, `case Ⅷ in [[:alpha:]]) printf alpha;; *) printf no;; esac`, "no", "no")
	bothLocales(t, `case Ⅷ in [[:digit:]]) printf digit;; *) printf no;; esac`, "no", "no")
	// A letter is still alphanumeric, so the fix is not "alnum matches
	// nothing outside ASCII".
	bothLocales(t, `case é in [[:alnum:]]) printf alnum;; *) printf no;; esac`, "alnum", "no")
	const nbsp = " "
	bothLocales(t, `case `+nbsp+` in [[:graph:]]) printf graph;; *) printf no;; esac`, "no", "no")
	bothLocales(t, `case `+nbsp+` in [[:print:]]) printf print;; *) printf no;; esac`, "print", "no")
	bothLocales(t, `case `+nbsp+` in [[:space:]]) printf space;; *) printf no;; esac`, "space", "no")
}

// `[[ ]]` reaches the same matcher as `case`, so it answers the same way.
func TestAConditionMatchesTheSameUnits(t *testing.T) {
	bothLocales(t, `[[ héllo == ????? ]] && printf yes || printf no`, "yes", "no")
	bothLocales(t, `[[ é == [a-ÿ] ]] && printf yes || printf no`, "yes", "no")
}

// And so does pathname expansion, where the subject is a filename rather than
// a value — which is why the callers ask about the subject and not only about
// the pattern. An all-ASCII `?` against a two-character name is the whole
// point: the pattern says nothing about the encoding and the answer moves.
func TestGlobbingMatchesTheSameUnits(t *testing.T) {
	// abcd is four bytes and héllo is five characters and six bytes, so
	// exactly one of them is five units — and which one moves with the
	// locale. Under C neither is, so the pattern matches nothing and stands
	// as its own text, which is this dialect's answer to a glob that hits
	// nothing.
	bothLocales(t, `>héllo; >abcd; printf "[%s]" ?????`, "[héllo]", "[?????]")
	bothLocales(t, `>éa; printf "[%s]" ??`, "[éa]", "[??]")
}

// A `*` stops between units and never inside one, so the rest of the pattern
// is never handed a subject beginning with half a character.
//
// **Measured, and the panel splits here**, which is why there is no corpus row
// for it: with a raw continuation byte written into a bracket,
// `case héllo in *[\251]llo)` is a hit in bash 5.3 and a miss in ksh93 and
// zsh under a UTF-8 locale, and a hit in all three under C. bash lets a `*`
// stop inside the two bytes of `é` and the other two do not. This follows the
// two, which is also the reading that agrees with `?` consuming a whole
// character: a matcher cannot count characters in one operator and bytes in
// the operator beside it.
func TestAStarStopsBetweenUnits(t *testing.T) {
	bothLocales(t, "s=héllo; case $s in *[\xa9]llo) printf hit;; *) printf miss;; esac", "miss", "hit")
	// The ordinary case is unaffected, which is what says the alignment is a
	// restriction on where a star may stop and not on what it may match.
	bothLocales(t, `s=héllo; case $s in *llo) printf hit;; *) printf miss;; esac`, "hit", "hit")
	bothLocales(t, `s=héllo; printf "[%s]" "${s##*é}"`, "[llo]", "[llo]")
}

// Asked only where the readings differ. An ASCII pattern against an ASCII
// subject is the same match either way, so a shell whose axis is unanswered
// still matches — and that is most of every script.
func TestAnAsciiMatchAsksNothing(t *testing.T) {
	out, st := runLocale(t, Unspecified, []string{"LC_ALL=C.UTF-8"},
		`case hello in ?????) printf five;; esac; printf "[%s]" "${x-abc}"`)
	if out != "five[abc]" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "five[abc]")
	}
	// A non-ASCII subject under a single-byte locale needs no answer either:
	// every shell matches its bytes there.
	if out, st := runLocale(t, Unspecified, []string{"LC_ALL=C"},
		`case héllo in ??????) printf six;; esac`); out != "six" || st != 0 {
		t.Errorf("in C: got %q (status %d), want %q at 0", out, st, "six")
	}
}

// And is refused by name where they do differ — including when the *pattern*
// is entirely ASCII, which is the case a caller naming only the pattern would
// have let through.
func TestANonAsciiMatchNeedsAnAnswer(t *testing.T) {
	out, st := runLocale(t, Unspecified, []string{"LC_ALL=C.UTF-8"},
		`case héllo in ?????) printf five;; esac`)
	if !strings.Contains(out, "a character being the locale's rather than a byte") {
		t.Errorf("got %q, want the axis named", out)
	}
	if st != 2 {
		t.Errorf("status %d, want 2", st)
	}
}
