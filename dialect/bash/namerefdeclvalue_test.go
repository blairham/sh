// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// **A declaration's refused aim takes the name away**, where the bare
// assignment's leaves the reference standing.
//
// A reference with nothing to point at is aimed by its first value, and a
// value that is no possible name aims it nowhere — that refusal was already
// right. What was wrong is what is left behind: bash has no such name at all
// afterwards, letters included, and this shell kept the reference and any
// attribute the same line had put on.
//
// Measured 2026-09-24 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh`
// over script files, against bash 5.3.20 and bash 5.3.15, which agree, over
// seven values — `/`, `42`, `7*6`, `a b`, `-x`, `1bad` and `@` — each after a
// `declare -n q` on the line before:
//
//	declare q=VALUE    the refusal at 1, and `declare -p q` is `q: not found`
//	q=VALUE            the refusal at 1, and `declare -p q` is `declare -n q`
//
// The value reads UNSET and the next line runs on both spellings and in both
// shells; only the name's survival parts. This corrects a row in
// refuseNamerefAim's own table, which recorded the reference as left standing
// after every spelling — true of the assignment and of `read`, false of the
// declaration (#4178).
func TestADeclarationsRefusedAimTakesTheNameAway(t *testing.T) {
	for _, tc := range []struct{ name, src, want, absent string }{
		{
			// The row `nameref12.sub` grades.
			"nothing is left of the name",
			"declare -n q\ndeclare q=42\ndeclare -p q",
			"q: not found", "declare -n q",
		},
		{
			// And the letters this line put on go with it: the `i` applied
			// before the store, so a refusal that takes the name away has to
			// take what the same line gave it.
			"and none of the line's letters either",
			"declare -n q\ndeclare -i q=7*6\ndeclare -p q",
			"q: not found", "declare -i q",
		},
		{
			// The status and the value never differed and must not move.
			"the status is 1 and the value unset",
			"declare -n q\ndeclare q=42\nprintf '<%d>' $?\nprintf '[%s]' \"${q-UNSET}\"",
			"<1>[UNSET]", "",
		},
		{
			// The control that makes this the declaration's rule: the bare
			// assignment refuses the same value and leaves the reference.
			"the bare assignment leaves the reference standing",
			"declare -n q\nq=42\ndeclare -p q",
			"declare -n q", "q: not found",
		},
		{
			// A good target still aims, which is the row this must not move.
			"a good target still aims the reference",
			"declare -n q\ndeclare q=tgt\ndeclare -p q",
			`declare -n q="tgt"`, "not a valid identifier",
		},
		{
			// And an already-aimed reference takes the value *through*
			// rather than being re-aimed, so no refusal arises at all.
			"an aimed reference takes the value through",
			"v=1\ndeclare -n q=v\ndeclare q=42\ndeclare -p q v",
			"declare -n q=\"v\"\ndeclare -- v=\"42\"", "not a valid identifier",
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

// A value on a `+n` operand goes **through** the reference before the letter
// comes off, and is read as an **aim** where there is nothing to go through.
//
// Taking the letter off leaves the target's name behind as the value — that
// was already right. What was dropped is the operand's own value: the letter
// came off first and the operand was then given up as finished, so a write the
// script asked for never happened.
//
// Measured 2026-09-24 against bash 5.3.20 and bash 5.3.15, which agree:
//
//	declare -n foo=bar; declare +n -i foo=7+4
//	  `foo` is the plain `bar` and `bar` holds 11, evaluated under this
//	  line's `i`
//	declare -n foo; declare +n foo=tgt
//	  the reference is aimed and then stripped: `declare -- foo="tgt"`
//	declare -n foo; declare +n -i foo=7+4
//	  `` `7+4': not a valid identifier `` at 1, and no `foo` at all
//
// So the plus does not make the operand an ordinary assignment: the aiming
// rules answer its value exactly as they would without it (#4178).
func TestAValueOnAPlusNOperandGoesThroughFirst(t *testing.T) {
	for _, tc := range []struct{ name, src, want, absent string }{
		{
			// The row `nameref19.sub` grades: the value reaches the target
			// under this line's integer letter.
			"the value reaches the target under this line's letters",
			"declare -n foo=bar\ndeclare +n -i foo=7+4\ndeclare -p foo bar",
			"declare -- foo=\"bar\"\ndeclare -i bar=\"11\"", "",
		},
		{
			"and a plain value reaches it too",
			"declare -n foo=bar\ndeclare +n foo=zz\ndeclare -p foo bar",
			"declare -- foo=\"bar\"\ndeclare -- bar=\"zz\"", "not found",
		},
		{
			// Unaimed and the value is a name: aimed, then stripped.
			"an unaimed reference is aimed and then stripped",
			"declare -n foo\ndeclare +n foo=tgt\ndeclare -p foo",
			`declare -- foo="tgt"`, "",
		},
		{
			// Unaimed and the value is no name: refused, and nothing left.
			"an unaimed reference refuses a value that is no name",
			"declare -n foo\ndeclare +n -i foo=7+4\nprintf '<%d>' $?\ndeclare -p foo",
			"<1>", "declare -i foo",
		},
		{
			// The valueless spellings, which never had a value to carry and
			// must not have moved.
			"a valueless plus over an aimed reference is unchanged",
			"declare -n foo=bar\ndeclare +n foo\ndeclare -p foo",
			`declare -- foo="bar"`, "",
		},
		{
			"a valueless plus over an unaimed one is unchanged",
			"declare -n foo\ndeclare +n foo\ndeclare -p foo",
			"declare -- foo", "",
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
