// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// Taking the reference letter off a **frozen** reference is refused where the
// reference is *aimed* and taken where it has nothing to point at.
//
// The refusal was already right and is already recorded: an aimed reference
// has a target whose name `+n` leaves behind as a **value**, and writing a
// value into a frozen name is what the freeze is about. What was wrong is that
// the question was asked before the answer was known — the freeze was consulted
// ahead of the walk, so a reference with nothing to point at was refused too.
// There is no value to leave there, so the letter comes off and the name is the
// frozen empty declaration it already was.
//
// Measured 2026-09-24 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh` over
// script files, against bash 5.3.20 and bash 5.3.15, which agree:
//
//	declare -rn foo; typeset +n foo      0, and `declare -r foo` after it
//	w=2; declare -rn k=w; typeset +n k   `k: readonly variable` at 1, and `k`
//	                                     still the frozen reference
//
// The first was `typeset: foo: readonly variable` at 1 here, with
// `declare -nr foo` left standing — a sentence bash does not write and a letter
// that should have gone.
//
// Core rather than an axis: ksh93u+ answers `typeset -rn` with its usage block
// at 2, so it never makes a frozen reference and cannot be asked (#4178).
func TestTakingTheLetterOffAFrozenReferenceSplitsOnWhetherItIsAimed(t *testing.T) {
	for _, tc := range []struct{ name, src, want, absent string }{
		{
			// The row `nameref17.sub` grades: nothing to point at, so the
			// letter comes off and the freeze stays.
			"an unaimed frozen reference gives the letter up",
			"declare -rn foo\ntypeset +n foo\nprintf '<%d>' $?\ndeclare -p foo",
			"<0>declare -r foo", "readonly variable",
		},
		{
			// The control that makes this a split rather than a blanket
			// pass: an aimed one is still refused, and is still a
			// reference afterwards.
			"an aimed frozen reference still refuses",
			"w=2\ndeclare -rn k=w\ntypeset +n k\nprintf '<%d>' $?\ndeclare -p k",
			"k: readonly variable", "",
		},
		{
			"and keeps the letter it refused to give up",
			"w=2\ndeclare -rn k=w\ntypeset +n k 2>/dev/null\ndeclare -p k",
			`declare -nr k="w"`, "",
		},
		{
			// A reference that is **not** frozen has always given the letter
			// up and still does, leaving the target's name as the value.
			"an unfrozen aimed reference leaves the target's name",
			"v=bar\ndeclare -n foo=v\ntypeset +n foo\ndeclare -p foo",
			`declare -- foo="v"`, "readonly variable",
		},
		{
			// And an unfrozen unaimed one is the ordinary empty declaration,
			// which is the row this change must not have moved.
			"an unfrozen unaimed reference is an empty declaration",
			"declare -n g\ntypeset +n g\ndeclare -p g",
			"declare -- g", "readonly variable",
		},
		{
			// The freeze really is still there afterwards: the name is not
			// writable just because the letter came off.
			"the freeze outlives the letter",
			"declare -rn foo\ntypeset +n foo\nfoo=x\nprintf '<%d>' $?",
			"foo: readonly variable", "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("= %q, want it to contain %q", out, tc.want)
			}
			if tc.absent != "" && strings.Contains(out, tc.absent) {
				t.Errorf("= %q, want it not to contain %q", out, tc.absent)
			}
		})
	}
}
