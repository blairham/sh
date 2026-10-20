// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `readonly`'s container letter is read where the operand carries an array
// **literal**, and it is read before the literal is stored.
//
// Two things had to be true and neither was. The letter was dropped for a
// literal operand — Semantics.ReadonlyRecordsTheCompoundAttribute is No here
// and was standing in front of it, the same way it once stood in front of a
// plain value — and this is the one utility whose operands are assigned
// *before* it runs, so even with the letter read the literal had already
// landed as whatever kind it looked like and the letter then met a name of
// the other kind.
//
// Measured 2026-09-23 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh`
// over a script file with standard input on the null device, against bash
// 5.3.20 and bash 5.3.15 in the digest-pinned image the suite is graded in,
// which agree. The rule the rows give is that `readonly -a` and `readonly -A`
// over a literal behave exactly like `declare -a` and `declare -A` do —
// including both conversion refusals, which this shell reached on neither
// side (#4179).
func TestTheContainerLetterOnReadonlyIsReadOverALiteral(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The table letter pairs a flat word list into keys and values,
			// where an indexed store lays the same words down in order.
			"a flat list under the table letter",
			"readonly -A f=(one 1 two 2)\ndeclare -p f",
			`declare -Ar f=([one]="1" [two]="2" )`,
		},
		{
			"a keyed literal under the table letter",
			"readonly -A m=([k]=v)\ndeclare -p m",
			`declare -Ar m=([k]="v" )`,
		},
		{
			// The control: the array letter still makes an array, so the
			// rows above are about the letter being read rather than about
			// every literal becoming a table.
			"a flat list under the array letter",
			"readonly -a u=(one 1 two 2)\ndeclare -p u",
			`declare -ar u=([0]="one" [1]="1" [2]="two" [3]="2")`,
		},
		{
			// A name holding a scalar is converted, exactly as a
			// declaration converts it.
			"over a name holding a scalar",
			"v=5\nreadonly -A v=(k 1)\ndeclare -p v",
			`declare -Ar v=([k]="1" )`,
		},
		{
			// And a name already holding the other kind is refused and
			// keeps what it held, which this shell reached on neither
			// side: it replaced the array and said nothing.
			"over a name holding an array",
			"declare -a x=(9)\nreadonly -A x=(k 1)\ndeclare -p x",
			"sh: line 2: x: cannot convert indexed to associative array\n" +
				`declare -a x=([0]="9")`,
		},
		{
			"over a name holding a table",
			"declare -A y=([k]=1)\nreadonly -a y=(9 8)\ndeclare -p y",
			"sh: line 2: y: cannot convert associative to indexed array\n" +
				`declare -A y=([k]="1" )`,
		},
		{
			// Appending, which already worked and is here so a change to
			// the order cannot quietly take it away.
			"appending under the table letter",
			"readonly -A s+=(k 1)\ndeclare -p s",
			`declare -Ar s=([k]="1" )`,
		},
		{
			// `--` after the letter, so the word scan does not stop before
			// it has seen one.
			"with a terminator after the letter",
			"readonly -A -- w=(k 1)\ndeclare -p w",
			`declare -Ar w=([k]="1" )`,
		},
		{
			// And a valueless letter records nothing, which is the axis
			// this sits beside and does not disturb.
			"valueless",
			"readonly -A n\ndeclare -p n",
			`declare -r n`,
		},
		{
			// A plain value was already read and stays read.
			"a plain value",
			"readonly -A p=5\ndeclare -p p",
			`declare -Ar p=([0]="5" )`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("= %q, want %q", out, tc.want)
			}
		})
	}
}

// An option this utility refuses leaves the order alone, and the row it
// leaves alone is measured rather than assumed.
//
// bash 5.3.20 answers `readonly -rA z=(k 1)` with `readonly: -r: invalid
// option` and its usage line, and then lists `declare -A z=([k]="1" )` — the
// assignment surviving the refusal, with the table attribute and without the
// freeze. This shell stores the literal as an indexed array there and has
// done since before this change; what is asserted here is that the change
// did **not** move it, because the borrowed order would have lost the
// assignment altogether and that would be a worse answer than the one
// already standing.
func TestARefusedOptionLeavesReadonlysOrderAlone(t *testing.T) {
	out, _ := answersRun(t, "readonly -rA z=(k 1)\ndeclare -p z")
	if !strings.Contains(out, "invalid option") {
		t.Fatalf("= %q, want the option refused", out)
	}
	if !strings.Contains(out, `declare -a z=([0]="k" [1]="1")`) {
		t.Errorf("= %q, want the assignment still stored as an indexed array:\n"+
			"\tthe reference stores it as a table and this shell does not, which is a\n"+
			"\tdivergence of its own — but losing the assignment entirely would be a\n"+
			"\tworse answer than the one already standing.", out)
	}
}
