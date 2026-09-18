// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// The `t` letter of a declaration, in bash's two halves: an attribute a
// *variable* carries and a mark a *function* carries, written back by
// different listings and sitting in a measured place among the other letters.
//
// Measured 2026-09-18 on bash 5.3.20 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME (#3051, #3101).

func TestTheTraceLetterIsRecordedListedAndOrdered(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, row := range []struct{ src, want string }{
		// The plain declaration, and the sign that takes it off.
		{`typeset -t T=1; typeset -p T`, "declare -t T=\"1\"\n"},
		{`Z=9; typeset -t Z; typeset -p Z`, "declare -t Z=\"9\"\n"},
		{`typeset -t T=1; typeset +t T; typeset -p T`, "declare -- T=\"1\"\n"},
		// Where it sits: before `x` and before the case letters, after `i`
		// and `r`. The pair with `u` is the one that tells this order from
		// the other shell's, which writes the letter after the case letters.
		{`typeset -tirxlu A=5; typeset -p A`, "declare -irtx A=\"5\"\n"},
		{
			`typeset -atr B=(1 2); typeset -p B`,
			"declare -art B=([0]=\"1\" [1]=\"2\")\n",
		},
		{`typeset -tu C=ab; typeset -p C`, "declare -tu C=\"AB\"\n"},
		{`typeset -tl D=AB; typeset -p D`, "declare -tl D=\"ab\"\n"},
		// And a filtered listing selects on it, where a name carrying no
		// other attribute is in none of the tables the walk reads.
		{`typeset -t a=1; b=2; typeset -t`, "declare -t a=\"1\"\n"},
	} {
		out, st := runBash(t, dir, row.src)
		if out != row.want || st != 0 {
			t.Errorf("%s\n got %q (status %d)\nwant %q", row.src, out, st, row.want)
		}
	}
}

func TestTheTraceLetterMarksAFunction(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const defs = `a(){ :; }; b(){ :; }; declare -ft a; readonly -f b; `
	for _, row := range []struct{ src, want string }{
		// The mark itself is silent, and `declare -F` is where it is read
		// back — beside the freeze, in the order the letters are listed in.
		{defs + `declare -F`, "declare -ft a\ndeclare -fr b\n"},
		{defs + `declare -Ft`, "declare -ft a\n"},
		// A union and not an intersection, the rule the other two letters
		// already follow.
		{defs + `declare -Ftr`, "declare -ft a\ndeclare -fr b\n"},
		{
			`a(){ :; }; declare -ft a; readonly -f a; export -f a; declare -F`,
			"declare -frtx a\n",
		},
	} {
		out, st := runBash(t, dir, row.src)
		if out != row.want || st != 0 {
			t.Errorf("%s\n got %q (status %d)\nwant %q", row.src, out, st, row.want)
		}
	}
}
