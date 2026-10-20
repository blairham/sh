// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// This shell takes an attribute letter over a frozen name that holds a value
// and refuses one over a frozen name that holds **nothing** — so its answer
// to "is an attribute over a frozen name refused" is not a constant, and the
// two halves are two axes. See #3937, and frozenattribute_test in interp for
// the wider one.
//
// Measured on AT&T ksh93u+ 2012-08-01 (`/bin/ksh`, macOS 25.6), 2026-09-20,
// `env -i PATH=/usr/bin:/bin LC_ALL=C /bin/ksh x.sh </dev/null` over a script
// file. Each row is `readonly c` and then the line named, with a
// `print -r -- "after=$?"` behind it:
//
//	              no value           after `c=1`
//	typeset -i    is read only, 1    after=0
//	typeset -E    is read only, 1    after=0
//	typeset -F    is read only, 1    after=0
//	typeset -L3   is read only, 1    after=0
//	typeset -Z3   is read only, 1    after=0
//	typeset -C    is read only, 1    is read only, 1
//	typeset -u    after=0            after=0
//	typeset -l    after=0            after=0
//	typeset -a    after=0            after=0
//	typeset -x    after=0            after=0
//	typeset -t    after=0            after=0
//	typeset c     after=0            after=0
//
// The `-u`, `-l` and `-a` rows are the ones that keep the refusal narrow:
// they are value-shaping by the wider axis's measured definition and are
// taken here in both columns, so this is a smaller set of letters and not
// that axis with a condition on it. The right-hand column says the rule is
// about what the name holds; `c=; readonly c; typeset -i c` is taken, so
// "holds nothing" means unset rather than empty.

func TestATypeLetterOverAFrozenNameWithNoValueIsRefusedHere(t *testing.T) {
	if got := ksh.Semantics().TypeLetterOverAFrozenNameWithNoValueIsRefused; got != interp.Yes {
		t.Errorf("TypeLetterOverAFrozenNameWithNoValueIsRefused = %v, want Yes", got)
	}
	if got := ksh.Semantics().AttributeOverAFrozenNameIsRefused; got != interp.No {
		t.Errorf("AttributeOverAFrozenNameIsRefused = %v, want No — the wide axis is "+
			"asked first and a Yes there would hide the narrow one entirely", got)
	}
	for _, decl := range []string{
		"typeset -i c", "typeset -E c", "typeset -F c",
		"typeset -L3 c", "typeset -Z3 c", "typeset -C c",
	} {
		t.Run(decl, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), "readonly c\n"+decl+"\necho never")
			// The builtin is named and the location is its own, which is the
			// shape this shell gives a refusal about an *attribute* — the
			// same one `typeset +r` takes, and not the plain line form an
			// assignment to a frozen name takes.
			if !strings.Contains(out, "typeset: c: is read only") {
				t.Errorf("%s over a valueless frozen name = %q, want the refusal with "+
					"the builtin named", decl, out)
			}
			if strings.Contains(out, "never") || st != 1 {
				t.Errorf("out = %q (status %d), want the script to end and exit 1", out, st)
			}
		})
	}
}

// The value column, which is what makes the refusal above an axis of its own
// rather than the wide one: the identical letter over a frozen name holding
// something is taken in silence.
func TestTheSameTypeLetterOverAFrozenNameHoldingAValueIsTakenHere(t *testing.T) {
	for _, src := range []string{
		"c=1\nreadonly c\ntypeset -i c\necho after",
		// An empty value is a value.
		"c=\nreadonly c\ntypeset -i c\necho after",
	} {
		out, st := runKsh(t, t.TempDir(), src)
		if !strings.Contains(out, "after") || st != 0 {
			t.Errorf("%q = %q (status %d), want the letter taken", src, out, st)
		}
	}
}

// And the letters outside the set, in the column the refusal applies to.
// These are what a refusal that read "any attribute over a frozen name with
// no value" would break, and every one of them is taken in the real shell.
func TestTheLettersOutsideTheTypeSetAreTakenOverAValuelessFrozenNameHere(t *testing.T) {
	for _, decl := range []string{
		"typeset -u c", "typeset -l c", "typeset -a c",
		"typeset -x c", "typeset -t c", "typeset c",
	} {
		t.Run(decl, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), "readonly c\n"+decl+"\necho after")
			if !strings.Contains(out, "after") || st != 0 {
				t.Errorf("%s = %q (status %d), want it taken", decl, out, st)
			}
		})
	}
}

// The assigning form is not this axis, and the pair below is what says so.
// An operand carrying a value is decided at the store, which parts the two
// letters that this axis treats alike:
//
//	readonly c; typeset -i c=4      `c: is read only`, the plain line form
//	readonly c; typeset -C c=(a=1)  taken — interp/frozencompoundbody.go
//
// Measured 2026-09-20. A refusal here keyed on the letter and the missing
// value alone, with nothing said about the operand, takes the second row
// back — and the second row is #3915's whole subject.
func TestAnAssigningDeclarationOverAValuelessFrozenNameIsNotThisAxis(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), "readonly c\ntypeset -i c=4\necho never")
	if strings.Contains(out, "typeset: c") {
		t.Errorf("`typeset -i c=4` = %q, want the builtin *not* named — an operand "+
			"with a value is refused as an assignment", out)
	}
	if !strings.Contains(out, "c: is read only") || strings.Contains(out, "never") || st != 1 {
		t.Errorf("out = %q (status %d), want the assignment refusal and the script to end", out, st)
	}
	out, st = runKsh(t, t.TempDir(), "readonly c\ntypeset -C c=(a=1)\nprint -r -- \"[${c.a}]\"")
	if out != "[1]\n" || st != 0 {
		t.Errorf("`typeset -C c=(a=1)` = %q (status %d), want the body stored", out, st)
	}
}

// The mirror image of the axis above, in the column it leaves alone: a
// **keyed** cell over a frozen name that holds a value is refused here,
// where the same letter over a frozen name holding nothing is taken.
//
// Measured on the same run, 2026-09-20:
//
//	                  no value           after `c=1`      after `c=`
//	typeset -A        after=0            is read only, 1  is read only, 1
//	typeset -C        is read only, 1    is read only, 1  is read only, 1
//	typeset -a        after=0            after=0          after=0
//	typeset -i        is read only, 1    after=0          after=0
//
// The `-a` row is the control and the reason this is not "a container
// letter": the indexed array letter is taken in every one of those columns.
// An indexed array can keep the scalar as element zero and a keyed cell
// cannot. The `-A` row read across is the other one — taken with nothing to
// rehouse and refused with something — which is what makes this a question
// about the value rather than about the letter.
//
// `-C` is refused in every column, which is why it is in both sets: the
// valueless half belongs to the axis above and the valued half to this one.
func TestAKeyedLetterOverAFrozenNameHoldingAValueIsRefusedHere(t *testing.T) {
	if got := ksh.Semantics().KeyedLetterOverAFrozenNameHoldingAValueIsRefused; got != interp.Yes {
		t.Errorf("KeyedLetterOverAFrozenNameHoldingAValueIsRefused = %v, want Yes", got)
	}
	for _, tc := range []struct{ pre, decl string }{
		{"c=1", "typeset -A c"},
		{"c=", "typeset -A c"},
		{"c=1", "typeset -C c"},
		{"c=", "typeset -C c"},
	} {
		t.Run(tc.pre+" "+tc.decl, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), tc.pre+"\nreadonly c\n"+tc.decl+"\necho never")
			if !strings.Contains(out, "typeset: c: is read only") {
				t.Errorf("= %q, want the refusal with the builtin named", out)
			}
			if strings.Contains(out, "never") || st != 1 {
				t.Errorf("out = %q (status %d), want the script to end and exit 1", out, st)
			}
		})
	}
}

// The two controls, in the real shell: the indexed array letter over the same
// frozen name holding the same value, and the keyed letter over a frozen name
// holding nothing.
func TestTheRowsOutsideTheKeyedRefusalAreTakenHere(t *testing.T) {
	for _, src := range []string{
		"c=1\nreadonly c\ntypeset -a c\necho after",
		"c=\nreadonly c\ntypeset -a c\necho after",
		"readonly c\ntypeset -A c\necho after",
	} {
		out, st := runKsh(t, t.TempDir(), src)
		if !strings.Contains(out, "after") || st != 0 {
			t.Errorf("%q = %q (status %d), want it taken", src, out, st)
		}
	}
}
