// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `~(g)`: the match takes as much subject as it can from where it begins.
//
// The issue that asked for this letter called it a global replacement, on the
// strength of a probe that used `//` — where the two readings agree. They do
// not agree on a trim, and re-measuring there is what says what the letter is.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-19, from a script file with
// standard input on /dev/null. Each row is written beside the two spellings
// that bracket it, which is the whole of the evidence: the letter's answer is
// the doubled operator's for a prefix trim and the single operator's for a
// suffix one (#3186).
func TestTheGreedyLetterLengthensAPrefixTrim(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// The prefix trim, where the far end is free and the greed reaches.
		{`v=aXbXc; printf '[%s]' "${v#*X}"`, "[bXc]"},
		{`v=aXbXc; printf '[%s]' "${v##*X}"`, "[c]"},
		{`v=aXbXc; printf '[%s]' "${v#~(g)*X}"`, "[c]"},
		{`w=abcabc; printf '[%s]' "${w#a*b}"`, "[cabc]"},
		{`w=abcabc; printf '[%s]' "${w##a*b}"`, "[c]"},
		{`w=abcabc; printf '[%s]' "${w#~(g)a*b}"`, "[c]"},
		{`v=aXbXc; printf '[%s]' "${v#~(g)*}"`, "[]"},
		{`v=aXbXc; printf '[%s]' "${v#*}"`, "[aXbXc]"},
		// The suffix trim, where the match is pinned at the end and the greed
		// has nowhere to go: the letter changes nothing.
		{`v=aXbXc; printf '[%s]' "${v%X*}"`, "[aXb]"},
		{`v=aXbXc; printf '[%s]' "${v%%X*}"`, "[a]"},
		{`v=aXbXc; printf '[%s]' "${v%~(g)X*}"`, "[aXb]"},
		{`w=abcabc; printf '[%s]' "${w%b*c}"`, "[abca]"},
		{`w=abcabc; printf '[%s]' "${w%%b*c}"`, "[a]"},
		{`w=abcabc; printf '[%s]' "${w%~(g)b*c}"`, "[abca]"},
		// And it is **not** a global replacement, which is the reading the
		// issue's table had: both spellings answer what they answered
		// without the letter.
		{`v=aXbXc; printf '[%s]' "${v//~(g)X/-}"`, "[a-b-c]"},
		{`v=aXbXc; printf '[%s]' "${v/~(g)X/-}"`, "[a-bXc]"},
		{`v=aXbXc; printf '[%s]' "${v/X/-}"`, "[a-bXc]"},
		// The flavor it leaves behind is the shell glob, so `?` is one
		// character and `.` is itself.
		{`[[ abc == ~(g)a?c ]] && echo yes || echo no`, "yes\n"},
		{`[[ abc == ~(g)a.c ]] && echo yes || echo no`, "no\n"},
		// And it composes with a letter that was already answered.
		{`[[ abc == ~(gi)ABC ]] && echo yes || echo no`, "yes\n"},
	} {
		out, st := kshOut(t, c.src)
		if out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// `~(p)` and `~(s)` are the shell glob, which is what a pattern with no
// prefix already is.
//
// Both are corrections to the issue's own table, which read `~(s)` as a
// regular expression whose `.` had been narrowed so that it stopped spanning
// a newline. It is not one: the dot never spanned anything there because the
// pattern is a glob, and `?` — which a regular expression would read as a
// quantifier on nothing — is the glob's one character.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-19 (#3186).
func TestThePatternLettersAreTheShellGlob(t *testing.T) {
	for _, c := range []struct {
		src    string
		status int
	}{
		{`[[ abc == ~(p)a*c ]]`, 0},
		{`[[ abc == ~(p)a?c ]]`, 0},
		{`[[ abc == ~(p)a.c ]]`, 1},
		{`[[ abc == ~(s)a?c ]]`, 0},
		{`[[ abc == ~(s)abc ]]`, 0},
		{`[[ abc == ~(s)a.c ]]`, 1},
		// The row the issue read as a narrowed dot. A `?` matches the
		// newline here, so nothing about this flavor is line-oriented.
		{`[[ $'a\nc' == ~(s)a?c ]]`, 0},
		// The controls: the same patterns under the flavor that really is a
		// regular expression answer the other way.
		{`[[ abc == ~(E)a.c ]]`, 0},
		{`[[ abc == ~(E)a?c ]]`, 0},
	} {
		out, st := kshOut(t, c.src)
		if out != "" || st != c.status {
			t.Errorf("%s\n got %q at %d\nwant %q at %d", c.src, out, st, "", c.status)
		}
	}
}

// `~(N)` is nullglob for one pattern, and reading it fixes two things at
// once: the word that names nothing is deleted, and the word that names
// something stops carrying the prefix as literal text.
//
// The second was the quiet half of #3186 — `printf '[%s]' ~(N)a.txt` in a
// directory holding `a.txt` answered `[~(N)a.txt]` at status 0, where the
// same letter on a pattern that missed was refused at 1. Two answers for one
// letter, and the wrong one said nothing.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-19, in a directory holding `a.txt`
// and `b.txt`, each probe from a script file with standard input on
// /dev/null.
func TestTheNullLetterDeletesAWordThatNamesNothing(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct{ src, want string }{
		// A pattern that misses: the word goes, and nothing is said. The
		// `[]` is `printf` reusing its format over no operand at all, which
		// is what the reference shell writes for the same line.
		{`printf '[%s]' ~(N)zz*; echo`, "[]\n"},
		// A name that is not there goes too, which is what says the letter
		// is about the filesystem and not about metacharacters.
		{`printf '[%s]' ~(N)zzz; echo`, "[]\n"},
		// A pattern that hits, which is the silently-literal row.
		{`printf '[%s]' ~(N)a.txt; echo`, "[a.txt]\n"},
		{`printf '[%s]' ~(N)*.txt; echo`, "[a.txt][b.txt]\n"},
		{`printf '[%s]' ~(Ni)A.TXT; echo`, "[a.txt]\n"},
		// It reaches its own word and no other.
		{`printf '[%s]' ~(N)*.txt other; echo`, "[a.txt][b.txt][other]\n"},
		{`for f in ~(N)zz*; do printf '<%s>' "$f"; done; echo END`, "END\n"},
		// The two controls. Without the letter the missing name stands as
		// its own text, and `set -f` turns the whole walk off so there is no
		// pattern to miss.
		{`printf '[%s]' zzz; echo`, "[zzz]\n"},
		{`set -f; printf '[%s]' ~(N)zz*; echo`, "[~(N)zz*]\n"},
		// Nothing globs in these two positions, so the word is its text
		// there whatever the letter says.
		{`x=~(N)zz*; printf '[%s]' "$x"; echo`, "[~(N)zz*]\n"},
		{`printf '[%s]' '~(N)a.txt'; echo`, "[~(N)a.txt]\n"},
		// And in a match context the letter is inert: it is a glob and the
		// subject is a string rather than a directory.
		{`[[ abc == ~(N)abc ]] && echo yes || echo no`, "yes\n"},
		{`case abc in ~(N)a*) echo yes;; *) echo no;; esac`, "yes\n"},
		{`s=abc; printf '[%s]' "${s#~(N)a}"; echo`, "[bc]\n"},
	} {
		out, st, err := preset.Combined(t, dialecttest.Base{Dir: dir}, c.src)
		if err != nil {
			t.Fatalf("run %q: %v", c.src, err)
		}
		if out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// A field whose `~(…)` remainder holds a `/` is left exactly where it was,
// and that is a limit written down rather than a reading.
//
// The group is the whole field's there while the walk matches one component
// at a time, and the reference shell does not answer the shape consistently
// enough to copy: measured 2026-09-19 in a directory holding `Sub/C.txt`,
// ksh93u+ names the file for `~(N)Sub/C.txt` and for
// `~(N)/tmp/…/Sub/C.txt`, and answers the characters as written for
// `~(E)Su./C..xt` and `~(i)/tmp/…/sub/c.txt` — where the flavor and the fold
// would each have matched every component.
//
// `N` is the exception and is read whatever the remainder holds, because it
// is about the **word** rather than about matching a component: a field that
// is a pattern for some other reason still loses the word when it names
// nothing (#3186).
func TestATildeGroupOverSeveralComponentsIsLeftAsWritten(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "Sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Sub", "C.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ src, want string }{
		{`printf '[%s]' ~(N)Sub/C.txt; echo`, "[~(N)Sub/C.txt]\n"},
		{`printf '[%s]' ~(i)sub/c.txt; echo`, "[~(i)sub/c.txt]\n"},
		// The word still goes where the field is a pattern for a reason of
		// its own and names nothing.
		{`printf '[%s]' ~(N)Sub/zz*; echo`, "[]\n"},
	} {
		out, st, err := preset.Combined(t, dialecttest.Base{Dir: dir}, c.src)
		if err != nil {
			t.Fatalf("run %q: %v", c.src, err)
		}
		if out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// The four regular-expression letters each read a language of their own, and
// a row per letter is what says so: reading any of them as `E` would pass a
// test that only asked whether the letter is accepted.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-20, every pattern supplied through
// a variable so that the shell's own quote removal and its `&` operator
// cannot reach it first. The rows answering 1 are the controls, and they are
// what separates each letter from the one beside it — `~(G)a?c` is a literal
// `?` where `~(X)a?c` is an optional character, `~(X)a.c&abc` is a
// conjunction where `~(E)a&b` is three characters, and `~(P)a\d` is a class
// where `~(E)a\d` is not reached at all (#3186).
func TestEachRegularExpressionLetterReadsItsOwnLanguage(t *testing.T) {
	for _, c := range []struct {
		src    string
		status int
	}{
		// `G` and `V` are basic: a backslashed paren groups and a bare one
		// is the character, a bare `+` and `?` are characters too.
		{`p='~(G)a\(b\)c'; [[ abc == $p ]]`, 0},
		{`p='~(G)a(b)c'; [[ 'a(b)c' == $p ]]`, 0},
		{`p='~(G)a?c'; [[ 'a?c' == $p ]]`, 0},
		{`p='~(G)a?c'; [[ abc == $p ]]`, 1},
		{`p='~(G)a+b'; [[ 'a+b' == $p ]]`, 0},
		{`p='~(G)a\+b'; [[ aab == $p ]]`, 0},
		{`p='~(V)a\(b\)c'; [[ abc == $p ]]`, 0},
		{`p='~(V)a\{3\}'; [[ aaa == $p ]]`, 0},
		{`p='~(V)a\{3\}'; [[ aa == $p ]]`, 1},
		// `X` is the extended one plus the conjunction, whose operands take
		// the same span rather than the same subject.
		{`p='~(X)a.c&abc'; [[ abc == $p ]]`, 0},
		{`p='~(X)a.c&axc'; [[ abc == $p ]]`, 1},
		{`p='~(X)a&c'; [[ abc == $p ]]`, 1},
		{`p='~(E)a&b'; [[ 'a&b' == $p ]]`, 0},
		{`p='~(X)a?c'; [[ abc == $p ]]`, 0},
		// `P` is Perl's, which is the escape set rather than a new engine.
		{`p='~(P)a\d'; [[ a1 == $p ]]`, 0},
		{`p='~(P)a\d'; [[ ab == $p ]]`, 1},
		{`p='~(P)^a.*?Xb'; [[ aXbXc == $p ]]`, 0},
		{`p='~(P)^a.*Xb$'; [[ aXbXc == $p ]]`, 1},
		// And the letters that are still refused by name, which is what
		// keeps the reading honest: `A` and `B` agree with `E` on every
		// probe written, and that is not evidence that they are `E`.
		{`[[ abc == ~(A)abc ]]`, 1},
		{`[[ abc == ~(B)abc ]]`, 1},
	} {
		out, st := kshOut(t, c.src)
		wantOut := ""
		if st != c.status {
			t.Errorf("%s\n got %q at %d\nwant %q at %d", c.src, out, st, wantOut, c.status)
		} else if c.status != 1 && out != wantOut {
			t.Errorf("%s\n got %q, want %q", c.src, out, wantOut)
		}
	}
}

// A construct the engine underneath cannot express is refused by name too,
// and `E` is the letter it reaches: it is **shipped** rather than refused, so
// a pattern using a backreference or a lookaround used to be compiled, fail
// to compile, and answer a confident `no` at status 0.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-20, `env -i` from a script file:
//
//	[[ abab == ~(E)(ab)\1 ]]    yes there; a silent no here
//	[[ abcd == ~(E)(ab)\1 ]]    no there; a silent no here
//	[[ abc == ~(E)a(?=b)bc ]]   yes there; a silent no here
//	[[ abc == ~(E)a(?!b)bc ]]   no there; a silent no here
//
// The two rows that agreed agreed by accident: the pattern is the same
// unwritable one and the subject is what differs, so our `no` was the compile
// failing rather than the backreference being read. Both refuse now, which is
// the honest form of the same answer — a script that stops can see the gap,
// and one that reads `no` cannot (#3894).
func TestARegularExpressionConstructTheEngineLacksIsRefusedByName(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{
			`[[ abab == ~(E)(ab)\1 ]] && echo yes || echo no`,
			"ksh: ~(E)(ab)\\1: the \\1 backreference is not implemented\n",
		},
		{
			`[[ abcd == ~(E)(ab)\1 ]] && echo yes || echo no`,
			"ksh: ~(E)(ab)\\1: the \\1 backreference is not implemented\n",
		},
		{
			`[[ abab == ~(E)(ab)\9x ]] && echo yes || echo no`,
			"ksh: ~(E)(ab)\\9x: the \\9 backreference is not implemented\n",
		},
		{
			`[[ abc == ~(E)a(?=b)bc ]] && echo yes || echo no`,
			"ksh: ~(E)a(?=b)bc: the (?= lookaround is not implemented\n",
		},
		{
			`[[ abc == ~(E)a(?!b)bc ]] && echo yes || echo no`,
			"ksh: ~(E)a(?!b)bc: the (?! lookaround is not implemented\n",
		},
		// A value is the other way a pattern arrives, and the one that can
		// hold a lookbehind: `<` after `(?` is the shell's own redirection
		// operator where it is written.
		{
			`p='a(?<=b)c'; [[ abc == ~(E)$p ]] && echo yes || echo no`,
			"ksh: ~(E)a(?<=b)c: the (?<= lookaround is not implemented\n",
		},
		{
			`p='a(?<!b)c'; [[ abc == ~(E)$p ]] && echo yes || echo no`,
			"ksh: ~(E)a(?<!b)c: the (?<! lookaround is not implemented\n",
		},
		// A trim is the same reading through another surface, and it takes
		// the same refusal rather than coming back with the value whole.
		{
			`v=abab; printf "[%s]" "${v#~(E)(ab)\1}"`,
			"ksh: ~(E)(ab)\\1: the \\1 backreference is not implemented\n",
		},
		// The same two constructs in the letters #3186 added, because the
		// refusal is the reason they could land: the engine is one engine
		// and its two absences are the same two whatever the flavor.
		{
			`p='~(X)(ab)\1'; [[ abab == $p ]]`,
			"ksh: ~(X)(ab)\\1: the \\1 backreference is not implemented\n",
		},
		{
			`p='~(P)(ab)\1'; [[ abab == $p ]]`,
			"ksh: ~(P)(ab)\\1: the \\1 backreference is not implemented\n",
		},
		{
			`p='~(G)\(ab\)\1'; [[ abab == $p ]]`,
			"ksh: ~(G)\\(ab\\)\\1: the \\1 backreference is not implemented\n",
		},
		{
			`p='~(V)\(ab\)\1'; [[ abab == $p ]]`,
			"ksh: ~(V)\\(ab\\)\\1: the \\1 backreference is not implemented\n",
		},
		{
			`p='~(X)a(?=b)bc'; [[ abc == $p ]]`,
			"ksh: ~(X)a(?=b)bc: the (?= lookaround is not implemented\n",
		},
		{
			`p='~(P)(?<=a)bc'; [[ abc == $p ]]`,
			"ksh: ~(P)(?<=a)bc: the (?<= lookaround is not implemented\n",
		},
		// The word edge is the basic flavors' own, and RE2 has only the
		// two-sided `\b`: measured, `[[ "ab cd" == ~(G)\<cd ]]` matches
		// there and `[[ abcd == ~(G)\<cd ]]` does not.
		{
			`p='~(G)\<cd'; [[ 'ab cd' == $p ]]`,
			"ksh: ~(G)\\<cd: the \\< word edge is not implemented\n",
		},
		{
			`p='~(V)cd\>'; [[ 'ab cd' == $p ]]`,
			"ksh: ~(V)cd\\>: the \\> word edge is not implemented\n",
		},
	} {
		out, st := kshOut(t, c.src)
		if out != c.want || st != 1 {
			t.Errorf("%s\n got %q at %d\nwant %q at 1", c.src, out, st, c.want)
		}
	}
}

// The other half of the refusal, and the half that says the scan is not
// over-broad: an ordinary `~(E)` pattern still matches, and so does one
// holding the very characters the scan looks for in a position where the
// engine reads them as ordinary.
//
// A scan that refused these would have traded a silent wrong answer for a
// loud one, which is the risk this change actually carries. Every row is
// measured on ksh93u+ 2012-08-01, 2026-09-20 and answers `yes` there.
func TestAnOrdinaryRegularExpressionPatternStillMatches(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`[[ abc == ~(E)a.c ]] && echo yes || echo no`, "yes\n"},
		{`[[ abc == ~(E)^a ]] && echo yes || echo no`, "yes\n"},
		{`[[ abc == ~(E)a?c ]] && echo yes || echo no`, "yes\n"},
		{`[[ a1 == ~(E)[a-z][0-9] ]] && echo yes || echo no`, "yes\n"},
		{`[[ ab1 == ~(E)(ab)1 ]] && echo yes || echo no`, "yes\n"},
		{`[[ ABC == ~(Ei)a.c ]] && echo yes || echo no`, "yes\n"},
		// Inside a bracket expression the three characters are three
		// characters, and the scan has to know it: `[(?=]` is a set holding
		// a paren, a question mark and an equals sign.
		{`p='a[(?=]b'; [[ 'a(b' == ~(E)$p ]] && echo yes || echo no`, "yes\n"},
		// A `]` first in the set is a member rather than the close, and a
		// `[:class:]` carries a `]` that does not close it either — both are
		// ways a bracket scan ends early and then reads the rest of the set
		// as though it were the pattern.
		{`p='[]x](?)'; [[ ']' == ~(E)$p ]] && echo yes || echo no`, "yes\n"},
		{`p='[[:alpha:]]c'; [[ ac == ~(E)$p ]] && echo yes || echo no`, "yes\n"},
		// An escaped backslash is a backslash, so the digit behind it is a
		// digit: the one spelling a scan that looked for the two characters
		// `\1` would refuse and the engine reads perfectly well.
		{`p='a\\1b'; [[ 'a\1b' == ~(E)$p ]] && echo yes || echo no`, "yes\n"},
		// The literal flavors quote the whole pattern before it is compiled,
		// so nothing in one is a construct at all — measured there, `~(F)a.c`
		// matches the three characters and not `abc`.
		{`[[ 'a.c' == ~(F)a.c ]] && echo yes || echo no`, "yes\n"},
		{`[[ abc == ~(F)a.c ]] && echo yes || echo no`, "no\n"},
		// And the glob flavor is this shell's own matcher, where a backslash
		// before a digit is the digit and always was.
		{`case a1 in a\1) echo yes;; *) echo no;; esac`, "yes\n"},
		{`v=abXc; printf "[%s]" "${v/~(E)X/-}"`, "[ab-c]"},
	} {
		out, st := kshOut(t, c.src)
		if out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}
