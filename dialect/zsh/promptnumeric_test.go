// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"strings"
	"testing"
)

// A count in front of a prompt code keeps that many trailing components —
// #1592.
//
// The digits are an argument to the code and not a code of their own, and the
// walker read the first digit as the code: `%2~` was `the %2 prompt escape is
// not implemented`, which is what powerlevel10k hit on a real startup. `%2~`
// is also about the most common thing anyone writes in a hand-made prompt.
//
// Every want is zsh 5.9.2, measured 2026-09-09 in the directory the test
// builds, and the two boundary rows are the reason this is a table rather than
// one case:
//
//   - `%9d` of a ten-segment path is nine segments and **no** leading `/`,
//     where `%10d` of the same path is all ten **with** it. A shell that only
//     joined the last n would answer the second row without the slash — a
//     plausible path to the wrong place at status 0, rather than a failure
//     anyone would notice.
//   - The tilde is one of the units on the abbreviated side, so a five-segment
//     path under a home is six things and `%6~` is the whole of it, tilde and
//     all.
func TestACountKeepsThatManyTrailingComponents(t *testing.T) {
	deep := t.TempDir() + "/a/b/c/d"
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	segs := int64(len(strings.Split(strings.TrimPrefix(deep, "/"), "/")))

	for _, tc := range []struct{ code, want string }{
		{"%1d", "d"},
		{"%2d", "c/d"},
		{"%3d", "b/c/d"},
		{"%2/", "c/d"},
		// At the count and past it: the whole path, leading slash back.
		{"%" + itoa(segs) + "d", deep},
		{"%" + itoa(segs+1) + "d", deep},
		// No limit.
		{"%0d", deep},
		{"%d", deep},
		// A bare count with no code draws nothing.
		{"%2", ""},
	} {
		t.Run(tc.code, func(t *testing.T) {
			out, st := runZsh(t, deep, "print -rP -- '"+tc.code+"'")
			if out != tc.want+"\n" || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.code, out, st, tc.want+"\n")
			}
		})
	}
}

// A *negative* count keeps that many **leading** components — #1699.
//
// The minus was refused by name in front of every code but the conditional,
// on the grounds that a plausible wrong answer is worse than a gap. Measured
// on zsh 5.9.2, 2026-09-12, in `/tmp/a/b/c` and three levels under a home,
// and the gap is what it was: a positive count is the trailing components and
// a negative one is the leading ones, both of the same path.
//
// `PWD` and `HOME` are assigned rather than taken from the directory the test
// runs in, so every want below is exact on any machine. What is being asked is
// the component arithmetic; where the shell gets its working directory from is
// asked elsewhere.
//
// Three rows carry the rule that is not "keep the first n":
//
//   - **The tilde is a unit and the slash is not.** `%-1~` under a home is the
//     marker on its own, where `%-1d` of the same directory spelled out is the
//     first segment *with* its slash.
//   - A count at or past the units is the whole path, and so is nought —
//     however it is spelled, since `%-0~` reaches nought through the minus.
//   - A bare minus is minus one, which is the reading the conditional already
//     took.
func TestANegativeCountKeepsThatManyLeadingComponents(t *testing.T) {
	for _, tc := range []struct{ pwd, home, code, want string }{
		{"/tmp/a/b/c", "/nowhere", "%-1~", "/tmp"},
		{"/tmp/a/b/c", "/nowhere", "%-2~", "/tmp/a"},
		{"/tmp/a/b/c", "/nowhere", "%-3~", "/tmp/a/b"},
		{"/tmp/a/b/c", "/nowhere", "%-4~", "/tmp/a/b/c"},
		{"/tmp/a/b/c", "/nowhere", "%-9~", "/tmp/a/b/c"},
		{"/tmp/a/b/c", "/nowhere", "%-~", "/tmp"},
		{"/tmp/a/b/c", "/nowhere", "%-0~", "/tmp/a/b/c"},
		{"/tmp/a/b/c", "/nowhere", "%-2d", "/tmp/a"},
		{"/tmp/a/b/c", "/nowhere", "%-2/", "/tmp/a"},
		// The tilde is the first unit, so the segments start at two.
		{"/home/me/p/x/y", "/home/me", "%-1~", "~"},
		{"/home/me/p/x/y", "/home/me", "%-2~", "~/p"},
		{"/home/me/p/x/y", "/home/me", "%-3~", "~/p/x"},
		{"/home/me/p/x/y", "/home/me", "%-4~", "~/p/x/y"},
		{"/home/me/p/x/y", "/home/me", "%-5~", "~/p/x/y"},
		// Where the slash is not: the same directory spelled out.
		{"/home/me/p/x/y", "/home/me", "%-1d", "/home"},
		{"/home/me/p/x/y", "/home/me", "%-2d", "/home/me"},
		{"/home/me/p/x/y", "/home/me", "%-5d", "/home/me/p/x/y"},
		{"/home/me/p/x/y", "/home/me", "%-6d", "/home/me/p/x/y"},
		// One component, and none at all.
		{"/tmp", "/nowhere", "%-1~", "/tmp"},
		{"/tmp", "/nowhere", "%-2~", "/tmp"},
		{"/", "/nowhere", "%-1~", "/"},
		{"/home/me", "/home/me", "%-1~", "~"},
		{"/home/me", "/home/me", "%-2~", "~"},
		{"/home/me", "/home/me", "%-1d", "/home"},
		// A minus with no code after it at all draws nothing, exactly as a
		// bare count does.
		{"/tmp/a/b/c", "/nowhere", "%-", ""},
	} {
		t.Run(tc.code+" in "+tc.pwd, func(t *testing.T) {
			src := "PWD=" + tc.pwd + "; HOME=" + tc.home + "; print -rP -- '" + tc.code + "'"
			out, st := runZsh(t, t.TempDir(), src)
			if out != tc.want+"\n" || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.code, out, st, tc.want+"\n")
			}
		})
	}
}

// `%c`, `%C` and `%.` take a count, and the only thing that separates them
// from `%~` and `%d` is what an absent one means — #1699.
//
// Measured on zsh 5.9.2, 2026-09-12. `%Nc` for N of one or more is exactly
// `%N~`, and `%NC` is `%Nd`; nought reads as one rather than as no limit,
// which is the whole of the difference. A negative count is the leading
// components here as everywhere else.
//
// The `/tmp` rows are the ones that say this is not the basename bash's `\W`
// draws: a single leading component keeps the `/` in front of it, so `%c`
// there is `/tmp` where `\W` is `tmp`. Sharing the field between the two codes
// answered every other row correctly, which is why it stood.
func TestTheBaseDirectoryCodesTakeACount(t *testing.T) {
	for _, tc := range []struct{ pwd, home, code, want string }{
		{"/tmp/a/b/c", "/nowhere", "%c", "c"},
		{"/tmp/a/b/c", "/nowhere", "%0c", "c"},
		{"/tmp/a/b/c", "/nowhere", "%1c", "c"},
		{"/tmp/a/b/c", "/nowhere", "%2c", "b/c"},
		{"/tmp/a/b/c", "/nowhere", "%3c", "a/b/c"},
		{"/tmp/a/b/c", "/nowhere", "%4c", "/tmp/a/b/c"},
		{"/tmp/a/b/c", "/nowhere", "%2.", "b/c"},
		{"/tmp/a/b/c", "/nowhere", "%2C", "b/c"},
		{"/tmp/a/b/c", "/nowhere", "%-1c", "/tmp"},
		{"/tmp/a/b/c", "/nowhere", "%-2c", "/tmp/a"},
		{"/tmp/a/b/c", "/nowhere", "%-c", "/tmp"},
		// A minus in front of a nought is still a nought, and nought here is
		// one — so this is the *trailing* component and not the leading one.
		{"/tmp/a/b/c", "/nowhere", "%-0c", "c"},
		// The home directory itself, where the abbreviated reading and the
		// plain one part company.
		{"/home/me", "/home/me", "%c", "~"},
		{"/home/me", "/home/me", "%2c", "~"},
		{"/home/me", "/home/me", "%C", "me"},
		{"/home/me", "/home/me", "%-1C", "/home"},
		// One component below the root, which is what says `%c` is not a
		// basename.
		{"/tmp", "/nowhere", "%c", "/tmp"},
		{"/tmp", "/nowhere", "%C", "/tmp"},
		{"/", "/nowhere", "%c", "/"},
		// And the abbreviated reading counts the tilde as a unit here too.
		{"/home/me/p/x/y", "/home/me", "%4c", "~/p/x/y"},
		{"/home/me/p/x/y", "/home/me", "%4C", "me/p/x/y"},
	} {
		t.Run(tc.code+" in "+tc.pwd, func(t *testing.T) {
			src := "PWD=" + tc.pwd + "; HOME=" + tc.home + "; print -rP -- '" + tc.code + "'"
			out, st := runZsh(t, t.TempDir(), src)
			if out != tc.want+"\n" || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.code, out, st, tc.want+"\n")
			}
		})
	}
}
