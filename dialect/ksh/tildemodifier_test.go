// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// `~(…)`, ksh93's pattern-modifier prefix and its **only** spelling for a
// regular expression. It was a parse error here, so a ksh script wanting an
// ERE could not be read at all — the refusal took the file rather than the
// line (#2621).
//
// Measured on ksh93u+ 2012-08-01, 2026-09-13. Every row below was run against
// that build as well as against this one, and the two agree.
func TestATildeModifierReads(t *testing.T) {
	for _, c := range []struct {
		src, want string
		status    int
	}{
		// The issue's own repro.
		{`[[ foo123 == ~(E)^foo[0-9]+$ ]] && printf '%s\n' 'ERE'`, "ERE\n", 0},
		{`[[ FOO == ~(i)foo ]] && printf '%s\n' 'ci'`, "ci\n", 0},
		{`s=aXbXc; printf '%s\n' "${s//~(E)X/-}"`, "a-b-c\n", 0},
		// A regular expression matches a *substring* where a glob matches the
		// whole string, which is the difference the flavors turn on and the
		// one nothing else in this matcher can express.
		{`[[ xabcx == ~(E)a.c ]]`, "", 0},
		{`[[ xabcx == ~(K)a?c ]]`, "", 1},
		{`[[ xabcx == a?c ]]`, "", 1},
		// The anchors, which only a substring flavor can show.
		{`[[ xabcx == ~(El)a.c ]]`, "", 1},
		{`[[ abcx == ~(El)a.c ]]`, "", 0},
		{`[[ xabc == ~(Er)a.c ]]`, "", 0},
		{`[[ xabcx == ~(Elr)a.c ]]`, "", 1},
		{`[[ abc == ~(Elr)a.c ]]`, "", 0},
		// The literal flavors, which match a substring too.
		{`[[ "a.c" == ~(F)a.c ]]`, "", 0},
		{`[[ abc == ~(F)a.c ]]`, "", 1},
		{`[[ xabcx == ~(F)abc ]]`, "", 0},
		{`[[ xabcx == ~(L)abc ]]`, "", 0},
		// The fold reaches a bracket and a class as well as a literal, which
		// is measured and is one place further than an *option* of the same
		// meaning reaches — see TestAnInlineFoldFlagReachesAPosixClass.
		{`[[ ABC == ~(i)[abc][abc][abc] ]]`, "", 0},
		{`[[ ABC == ~(i)[[:lower:]][[:lower:]][[:lower:]] ]]`, "", 0},
		{`[[ ABC == ~(i)@(abc|x) ]]`, "", 0},
		// The toggles, and a group that says nothing.
		{`[[ ABC == ~(+i)abc ]]`, "", 0},
		{`[[ ABC == ~(-i)abc ]]`, "", 1},
		{`[[ ABC == ~(Ei)^a.c$ ]]`, "", 0},
		{`[[ abc == ~()abc ]]`, "", 0},
		// A letter ksh93 does not have is a pattern that cannot match, and it
		// says nothing about it — measured, status 1 with an empty standard
		// error, and the same against `*`.
		{`[[ abc == ~(Z)abc ]]`, "", 1},
		{`[[ abc == ~(Z)* ]]`, "", 1},
		// The other surfaces a pattern reaches.
		{`case abc in ~(E)^a.c$) echo yes;; *) echo no;; esac`, "yes\n", 0},
		{`case abc in ~(E)b) echo yes;; *) echo no;; esac`, "yes\n", 0},
		{`s=abcabc; printf '[%s]\n' "${s//~(E)b/-}"`, "[a-ca-c]\n", 0},
		{`s=aXbXc; printf '[%s]\n' "${s/~(E)X/-}"`, "[a-bXc]\n", 0},
		{`s=aXbXc; printf '[%s]\n' "${s//~(i)x/-}"`, "[a-b-c]\n", 0},
		{`s=abc; printf '[%s]\n' "${s#~(E)a}"`, "[bc]\n", 0},
		{`s=abc; printf '[%s]\n' "${s%~(E)c}"`, "[ab]\n", 0},
		// A word that is not a pattern keeps the characters, which is what
		// the lexer rule has to leave alone: the group belongs to the word
		// wherever the word stands.
		{`echo ~(E)abc`, "~(E)abc\n", 0},
		{`x=~(Z)abc; echo "$x"`, "~(Z)abc\n", 0},
		{`echo a~(x)b`, "a~(x)b\n", 0},
		// Quoting decides it, and the two spellings are measured apart:
		// a written `~(` reads and one that arrived quoted does not.
		{`[[ abc == ~(E)"a.c" ]]`, "", 0},
		{`[[ abc == "~(E)"a.c ]]`, "", 1},
		{`p="~(E)a.c"; [[ abc == $p ]]`, "", 0},
		{`p="~(E)a.c"; [[ abc == "$p" ]]`, "", 1},
		// A letter ksh93 *has* and this shell does not answer is refused by
		// name. Silently taking it and matching as though it were absent is
		// the failure this avoids: `~(G)` is grep's basic syntax, where
		// `a?c` is a literal `?`, so reading it as ERE would be a wrong
		// answer at status 0.
		{`[[ abc == ~(G)abc ]]`, "ksh: ~(G)abc: the ~(G) pattern modifier is not implemented\n", 1},
		{`[[ abc == ~(P)abc ]]`, "ksh: ~(P)abc: the ~(P) pattern modifier is not implemented\n", 1},
		{`[[ abc == ~(g)abc ]]`, "ksh: ~(g)abc: the ~(g) pattern modifier is not implemented\n", 1},
	} {
		out, st := kshOut(t, c.src)
		if out != c.want || st != c.status {
			t.Errorf("%s\n got %q at %d\nwant %q at %d", c.src, out, st, c.want, c.status)
		}
	}
}

// Pathname expansion reaches the same reading, and it is the one surface that
// can tell a substring search from a whole-string one *without* a condition:
// measured in a directory holding `abc`, `axc` and `zzz`, `echo ~(E)b.*`
// answers `abc` on ksh93u+ — the expression matched inside the name rather
// than against the whole of it.
//
// The word still has to carry a glob character for the walk to begin, here as
// there in shape though not in reach: `echo ~(E)a.c` is the literal word in
// this shell and the two matching names on ksh93, because a `~(` does not by
// itself make a word a pattern here yet. That gap is recorded rather than
// papered over — see interp/tildemodifier.go.
func TestATildeModifierReachesPathnameExpansion(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"abc", "axc", "zzz"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct{ src, want string }{
		{`echo ~(E)b.*`, "abc\n"},
		{`echo ~(E)^a.*$`, "abc axc\n"},
		{`echo ~(K)a*`, "abc axc\n"},
		{`echo ~(F)b*`, "~(F)b*\n"},
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

// And the lexer rule is ksh93's alone. bash 5.3.15, that binary as `sh`, bash
// 3.2.57, dash and BusyBox ash all refuse `~(E)…` at the paren, at parse time
// and unanimously; zsh neither refuses nor honors it — there `~` is the
// exclusion operator and `~(E)abc` is a pattern that reads and does not match.
// So a dialect that took the `(` would accept what its own shell rejects.
func TestATildeGroupIsKshsAlone(t *testing.T) {
	if !ksh.Dialect().TildeGroup {
		t.Errorf("ksh: TildeGroup is off, want it on")
	}
	if bash.Dialect().TildeGroup {
		t.Errorf("bash: TildeGroup is on, want it off")
	}
	if zsh.Dialect().TildeGroup {
		t.Errorf("zsh: TildeGroup is on, want it off")
	}
}

// The paired half of interp's TestTheOptionFoldStopsAtAPosixClassInABracket:
// the `i` letter of a `~(…)` group folds a POSIX class inside a bracket,
// where a fold an *option* asked for does not.
//
// Both halves are needed, and one alone reads as the opposite finding. The
// matcher folded a class for every caller until #2716 — which is bash 5.3.15's
// answer for `shopt -s nocasematch` and is not ksh93's for this flag — and a
// fix that simply stopped folding classes would have taken these rows with it.
//
// Measured 2026-09-14 on ksh93u+ 2012-08-01: `[[ A == ~(i)[[:lower:]] ]]` and
// `[[ A == ~(i)[a-z] ]]` both match, and so do the negated and the
// lower-against-upper spellings.
func TestAnInlineFoldFlagReachesAPosixClass(t *testing.T) {
	for _, c := range []struct {
		src    string
		status int
	}{
		{`[[ A == ~(i)[[:lower:]] ]]`, 0},
		{`[[ a == ~(i)[[:upper:]] ]]`, 0},
		{`[[ A == ~(i)[a-z] ]]`, 0},
		// Without the letter the same bracket is exact, so the rows above
		// are evidence about the flag rather than about the class.
		{`[[ A == [[:lower:]] ]]`, 1},
		{`[[ A == [a-z] ]]`, 1},
	} {
		out, st, err := preset.Combined(t, dialecttest.Base{}, c.src)
		if err != nil {
			t.Fatalf("run %q: %v", c.src, err)
		}
		if out != "" || st != c.status {
			t.Errorf("%s\n got %q at %d\nwant %q at %d", c.src, out, st, "", c.status)
		}
	}
}
