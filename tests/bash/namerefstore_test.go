// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A write through a **self-aimed** reference lands on the shadowed cell, and
// where that cell holds an array the ordinary compound rule applies to it.
//
// `a=(p q); a=z` leaves `declare -a a=([0]="z" [1]="q")` here — a plain value
// over a compound writes element zero rather than replacing the name, which is
// Semantics.ScalarAssignedOverACompoundReplacesTheName. A write through a
// reference aimed at its own name never reached that rule: the cell it lands on
// is *shadowed*, so the value went straight into the scope's scalar slot and
// the caller's array came back with its first element unchanged.
//
// Measured 2026-09-23 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh`
// over a script file, against bash 5.3.20:
//
//	declare -a b=(0); g() { local -n b=$1; b=X; }; g b; declare -p b
//	  bash 5.3.20   declare -a b=([0]="X")
//	  before        declare -a b=([0]="0")
//
// The three circular-reference warnings are identical on both sides and are not
// this row: what differed was the element. Located by the suite lane on
// `nameref.tests` and routed here as store internals rather than a nameref rule
// (#4178).
func TestAWriteThroughASelfAimedReferenceReachesTheCompoundRule(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The row. `local -n b=$1` with `$1` being `b` aims the
			// reference at its own name.
			"the outer name holds an array",
			"declare -a b=(0)\ng() { local -n b=$1; b=X; }\ng b\ndeclare -p b",
			`declare -a b=([0]="X")`,
		},
		{
			// The controls, all three of which already agreed and still do.
			// Different names take the ordinary path, so this one says the
			// element rule itself was never broken.
			"different names, so no self reference",
			"declare -a a=(0)\nf() { local -n r=$1; r=X; }\nf a\ndeclare -p a",
			`declare -a a=([0]="X")`,
		},
		{
			// A caller holding a scalar keeps the saved slot, which is the
			// path this change must not disturb — and the one a first draft
			// sent into an empty array's nil map by gating on the saved
			// *entry* rather than on whether an array was held.
			"the outer name holds a scalar",
			"c=0\nh() { local -n c=$1; c=X; }\nh c\ndeclare -p c",
			`declare -- c="X"`,
		},
		{
			// And a table takes the same rule as an array: element zero by
			// the key `0`, which is scalarOverCompound's own branch.
			"the outer name holds a table",
			"declare -A m=([k]=1)\nj() { local -n m=$1; m=X; }\nj m\ndeclare -p m",
			`declare -A m=([0]="X" [k]="1" )`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("= %q, want it to contain %q", out, tc.want)
			}
		})
	}
}

// Taking the reference attribute off a name brings its cell back, so a listing
// can see the name again.
//
// `typeset -n g` hides the cell the reference takes over — that hide is what
// keeps an inherited value from answering reads of it — and `typeset +n g` left
// the hide in place. A hidden name is a name no listing prints, so the name
// came back as `not found` where the reference shells list it.
//
// Measured 2026-09-23 against bash 5.3.20:
//
//	typeset -n x; typeset +n x; declare -p x
//	  bash 5.3.20   declare -- x
//	  before        declare: x: not found
//
// The **value** agreed all along and still does: `${x-UNSET}` is UNSET on both
// sides, so what was wrong was the listing alone. Two things had to change
// together — the hide comes off, and the `n` letter stops counting as an
// attribute this line names, because while it did `declareEmpty` treated the
// operand as adding an attribute to a name rather than bringing one into being
// and so created no cell at all (#4178).
func TestTakingTheReferenceAttributeOffBringsTheCellBack(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a reference with nothing to point at",
			"typeset -n x\ntypeset +n x\ndeclare -p x",
			`declare -- x`,
		},
		{
			// The value half, which never differed.
			"and it still reads unset",
			"typeset -n x\ntypeset +n x\nprintf '[%s]' \"${x-UNSET}\"",
			`[UNSET]`,
		},
		{
			// An **aimed** reference leaves the target's name behind, which
			// is the other branch and was already right.
			"an aimed reference leaves the target's name",
			"t=7\ntypeset -n z=t\ntypeset +n z\ndeclare -p z",
			`declare -- z="t"`,
		},
		{
			// A name that was never a reference is untouched by the letter.
			"a name that was never a reference",
			"typeset y\ndeclare -p y",
			`declare -- y`,
		},
		{
			// And `unset -n` is a different question with a different
			// answer: the name really goes, in both shells.
			"unset -n removes the name outright",
			"typeset -n q\nunset -n q\ndeclare -p q",
			`not found`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("= %q, want it to contain %q", out, tc.want)
			}
		})
	}
}
