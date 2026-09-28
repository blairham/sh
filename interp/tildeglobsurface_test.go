// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// Which surface reads a `~(K)` group, and its class escapes with it.
//
// The rows are ksh93u+ 2012-08-01's, measured 2026-09-28 from `-c` under
// `env -i PATH=/usr/bin:/bin` with a scratch `HOME`. **It is a quirk of that
// shell rather than a rule** — `%` reads the group and `%%`, the same anchor
// with the other length preference, does not — and it is recorded because it
// is measured, not because it is explicable.
//
// Every row that says the group is *not* read has a control beside it
// showing the same trim works without the group, so "not read" is the group
// being left standing rather than the operator being broken.
func TestWhichSurfacesReadATildeGlobGroup(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The controls: each trim works.
		{"a prefix trim", `v=xab; echo "[${v#x}]"`, `[ab]`},
		{"a suffix trim", `v=xab; echo "[${v%b}]"`, `[xa]`},

		// **Not read**: the group is ordinary characters, so the pattern
		// matches no prefix and the value stands.
		{"a prefix trim declines it", `v=xab; echo "[${v#~(K)x}]"`, `[xab]`},
		{"the long prefix trim too", `v=xab; echo "[${v##~(K)x}]"`, `[xab]`},
		{"and the long suffix trim", `v=xab; echo "[${v%%~(K)b}]"`, `[xab]`},
		{"and a replacement", `v=xab; echo "[${v/~(K)x/Q}]"`, `[xab]`},
		{"and a global one", `v=xab; echo "[${v//~(K)x/Q}]"`, `[xab]`},

		// **Read**: the one span-choosing operator that does.
		{"the short suffix trim reads it", `v=xab; echo "[${v%~(K)b}]"`, `[xa]`},
		{"with a bracket", `v=1abc1; echo "[${v%~(K)[0-9]}]"`, `[1abc]`},
		// And its class escapes come with it, which is the row that says the
		// escapes follow the group rather than the whole-subject surfaces.
		{"and its class escapes", `v=1abc1; echo "[${v%~(K)\d}]"`, `[1abc]`},

		// **It is the `K` letter and not the group.** These are read on
		// every surface in that shell and in this one alike.
		{"the fold on a prefix trim", `v=1abc1; echo "[${v#~(i)[0-9]}]"`, `[abc1]`},
		{"the fold on a suffix trim", `v=1abc1; echo "[${v%~(i)[0-9]}]"`, `[1abc]`},
		{"an expression flavor", `v=xab; echo "[${v#~(E)x}]"`, `[ab]`},
		{"and a glob one by another letter", `v=xab; echo "[${v#~(g)x}]"`, `[ab]`},

		// The whole-subject surfaces still read both, which is what #4961
		// closed and what must not move.
		{"a condition", `[[ za1b == z~(K)a\db ]] && echo YES || echo NO`, "YES"},
		{"and its other half", `[[ zadb == z~(K)a\db ]] && echo YES || echo NO`, "NO"},
		{"a case arm", `case za1b in z~(K)a\db) echo YES;; *) echo NO;; esac`, "YES"},
	} {
		if got := tildeMid(t, tc.src, nil); got != tc.want {
			t.Errorf("%s: %s = %q, want %q", tc.name, tc.src, got, tc.want)
		}
	}
}

// The class escapes follow the **group**, not the whole-subject surfaces.
//
// They were gated on `whole` while pathname expansion and `%` were two
// surfaces that read the group without reading its escapes. These rows are
// the pair that separates the two gates: a `%` trim reads a bracket class
// under either and an escape class only under the group's.
func TestTheClassEscapesFollowTheGroupAndNotTheWholeSubject(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// A bracket is a class under any reading, so this row moves for
		// neither gate and is the control.
		{"a bracket on a suffix trim", `v=1abc1; echo "[${v%~(K)[0-9]}]"`, `[1abc]`},
		// The same trim with the class spelled as an escape.
		{"an escape on a suffix trim", `v=1abc1; echo "[${v%~(K)\d}]"`, `[1abc]`},
		// Longer patterns, so the candidate-skipping readers are exercised
		// rather than only the single-character case.
		{"two escapes", `v=abc12; echo "[${v%~(K)\d\d}]"`, `[abc]`},
		{"a literal and an escape", `v=abc1; echo "[${v%~(K)c\d}]"`, `[ab]`},
		{"mixed classes", `v=abcX1; echo "[${v%~(K)c\D\d}]"`, `[ab]`},
		{"a word class", `v=abcd; echo "[${v%~(K)\w}]"`, `[abc]`},
		// And the ones that must not match, so the rows above are not a
		// reading that matches everything.
		{"a trailing literal that is absent", `v=abc1; echo "[${v%~(K)\dx}]"`, `[abc1]`},
		{"a leading literal that is absent", `v=abc1; echo "[${v%~(K)b\d}]"`, `[abc1]`},
		{"and no digit at all", `v=abc; echo "[${v%~(K)\d}]"`, `[abc]`},
	} {
		if got := tildeMid(t, tc.src, nil); got != tc.want {
			t.Errorf("%s: %s = %q, want %q", tc.name, tc.src, got, tc.want)
		}
	}
}
