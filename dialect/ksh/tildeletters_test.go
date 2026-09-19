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

// The regular-expression letters stay refused, and the reason is not effort.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-18: `[[ abab == ~(G)\(ab\)\1 ]]`
// matches and `[[ abcd == ~(G)\(ab\)\1 ]]` does not, with
// `[[ abcd == ~(G)\(ab\)cd ]]` as the control that says the group parses. So
// that flavor has **backreferences**, Go's `regexp` is RE2 and has none —
// `regexp.Compile` refuses `(ab)\1` outright — and the same two probes answer
// no under `~(E)` and `~(X)`, which says it is `G`'s alone.
//
// Refused by name rather than matched without them: a pattern using a
// backreference would answer a plausible `no`, which is the wrong-and-silent
// shape this repository minds most (#3186).
func TestTheRegularExpressionLettersAreStillRefusedByName(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`[[ abc == ~(G)abc ]]`, "ksh: ~(G)abc: the ~(G) pattern modifier is not implemented\n"},
		{`[[ abc == ~(P)abc ]]`, "ksh: ~(P)abc: the ~(P) pattern modifier is not implemented\n"},
		{`[[ abc == ~(V)abc ]]`, "ksh: ~(V)abc: the ~(V) pattern modifier is not implemented\n"},
		{`[[ abc == ~(X)abc ]]`, "ksh: ~(X)abc: the ~(X) pattern modifier is not implemented\n"},
	} {
		out, st := kshOut(t, c.src)
		if out != c.want || st != 1 {
			t.Errorf("%s\n got %q at %d\nwant %q at 1", c.src, out, st, c.want)
		}
	}
}
