// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A `|` outside every group is an alternation here too — #1497, which #1331
// left behind: the two were measured together and only one of them was about
// groups, so a value read correctly inside `( … )` was matched as a character
// standing on its own.
//
// Every row measured against zsh 5.9.2 on 2026-09-08 and again on 2026-09-11,
// each probe in a script file of its own under `env -i`.
func TestATopLevelBarFromAValueIsAnAlternation(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The three routes a live bar arrives by: the flag, a `case` arm
		// expanded from a value, and the option that makes every expansion
		// result a pattern.
		{`L='a|b'; [[ a = ${~L} ]] && echo Y || echo N`, "Y"},
		{`L='a|b'; [[ b = ${~L} ]] && echo Y || echo N`, "Y"},
		{`L='a|b'; [[ c = ${~L} ]] && echo Y || echo N`, "N"},
		{`L='a|b'; case a in ${~L}) echo Y;; *) echo N;; esac`, "Y"},
		{`setopt globsubst; L='a|b'; [[ a = $L ]] && echo Y || echo N`, "Y"},
		// **The discriminating row.** Without the split the value matches
		// its own text, which is what it did here: the bar was a character.
		{`L='a|b'; [[ 'a|b' = ${~L} ]] && echo Y || echo N`, "N"},
		// And unmarked it still is one, which is the axis #1331 settled and
		// this must not move.
		{`L='a|b'; [[ 'a|b' = $L ]] && echo Y || echo N`, "Y"},
		{`L='a|b'; [[ a = $L ]] && echo Y || echo N`, "N"},
		// Inside a group the same value already worked, and still does.
		{`L='a|b'; [[ a = (${~L}) ]] && echo Y || echo N`, "Y"},
		// A star in an arm, an empty arm, and a bar alone.
		{`L='a*|b'; [[ axx = ${~L} ]] && echo Y || echo N`, "Y"},
		{`L='a|'; [[ '' = ${~L} ]] && echo Y || echo N`, "Y"},
		{`L='a|'; [[ a = ${~L} ]] && echo Y || echo N`, "Y"},
		{`L='|'; [[ '' = ${~L} ]] && echo Y || echo N`, "Y"},
		// A bar inside a bracket is an ordinary member: `[a|b]` matches the
		// bar itself as well as the two letters.
		{`L='[a|b]'; [[ '|' = ${~L} ]] && echo Y || echo N`, "Y"},
		{`L='[a|b]'; [[ a = ${~L} ]] && echo Y || echo N`, "Y"},
		{`L='x[a|b]y'; [[ 'x|y' = ${~L} ]] && echo Y || echo N`, "Y"},
		// The written spelling is what makes this reachable only from a
		// value: it is a parse error in this shell and here alike.
		{`eval '[[ a = a|b ]]' 2>/dev/null; echo st=$?`, "st=1"},
		// A trim takes the alternation too, since the matcher is the same
		// one. `#` is the shortest match and agrees with zsh in both arm
		// orders; `##` does not, and that is #1918 rather than this — see
		// the note there.
		{`x=abc; p='a|ab'; print -r -- "[${x#${~p}}]"`, "[bc]"},
		{`y=zab; q='b|ab'; print -r -- "[${y%${~q}}]"`, "[za]"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}

// And against the filesystem, where the same bar generates one word per arm.
//
// A separate test because it needs a directory: measured, `L='a|b'; print -l
// -- ${~L}` lists the files `a` and `b` in zsh 5.9.2, so a top-level bar makes
// a field a pattern all by itself — which is the half `hasUnescapedMeta` must
// not answer for the dialects where a bar only means something inside a group.
func TestATopLevelBarGeneratesOneWordPerArm(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: dir}, `L='a|b'; print -l -- ${~L}`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "a\nb\n"; out != want || st != 0 {
		t.Errorf("listed %q at %d, want %q at 0", out, st, want)
	}
	// Unmarked it is a filename and not a pattern, which is the same axis
	// one row down: nothing is generated and the field stands.
	out, _, err = preset.Combined(t, dialecttest.Base{Dir: dir}, `L='a|b'; print -l -- $L`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "a|b\n"; out != want {
		t.Errorf("unmarked listed %q, want %q", out, want)
	}
}
