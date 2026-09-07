// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `set -A name value …`: the array assignment through a name a variable
// holds. Tests name axes and letters, never shells.

// setArrayRun runs src with the array letter in place and the semantics the
// setter leaves.
func setArrayRun(t *testing.T, src string, set func(*Semantics), dg Diagnostics) (string, string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.SetArrayLetter = Yes
	sem.SetArrayOptionsContinuePastTheName = No
	sem.SetArrayWithNoValuesUnsetsTheName = No
	sem.ArrayBaseIsZero = Yes
	// The fatality questions these tests are not about, answered the way that
	// leaves the shell running to be asked what happened. The axes
	// themselves are asked by their own tests.
	sem.BadSetOptionNameFatal = No
	if set != nil {
		set(&sem)
	}
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &dg,
		Dir: t.TempDir(), Name: "testsh",
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}

// The whole of what the letter is for: an array assigned through a name that
// is not a literal, which `name=(…)` cannot express.
func TestSetArrayAssignsThroughAHeldName(t *testing.T) {
	out, errs, st := setArrayRun(t, `h=hh
set -A $h m n o
echo "n=${#hh[@]} [${hh[@]}] first=[${hh[0]}]"`, nil, Diagnostics{})
	want := "n=3 [m n o] first=[m]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("set -A $h = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// It replaces rather than appends, and it goes through the same whole-array
// store `name=(…)` reaches — so the array base is the dialect's and not this
// builtin's. A store of its own would have had to answer the base twice.
func TestSetArrayReplacesAndReadsTheDialectsBase(t *testing.T) {
	out, errs, st := setArrayRun(t, `set -A a x y z
set -A a p
echo "1 n=${#a[@]} [${a[@]}]"
set -A b q r
echo "2 zero=[${b[0]}] one=[${b[1]}]"`, nil, Diagnostics{})
	want := "1 n=1 [p]\n2 zero=[q] one=[r]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("set -A twice = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
	// The other base, which the builtin never asks about itself.
	out, _, _ = setArrayRun(t, `set -A b q r; echo "zero=[${b[0]}] one=[${b[1]}]"`,
		func(s *Semantics) { s.ArrayBaseIsZero = No }, Diagnostics{})
	if out != "zero=[] one=[q]\n" {
		t.Errorf("set -A under a one-based dialect = %q, want the first element at 1", out)
	}
}

// The plus form replaces from the *front* and leaves the rest of the array
// standing, which is a different operation from the minus form rather than
// the same one. A single implementation gets this half wrong.
func TestThePlusFormReplacesFromTheFront(t *testing.T) {
	out, errs, st := setArrayRun(t, `set -A b 1 2 3 4 5
set +A b Q R
echo "1 n=${#b[@]} [${b[@]}]"
set +A b Z Y X W
echo "2 n=${#b[@]} [${b[@]}]"
set +A c p q
echo "3 n=${#c[@]} [${c[@]}]"`, nil, Diagnostics{})
	want := "1 n=5 [Q R 3 4 5]\n2 n=5 [Z Y X W 5]\n3 n=2 [p q]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("set +A = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// And the plus form with nothing to put at the front leaves the array exactly
// as it was — unanimous, and *not* the minus form's answer to the same
// emptiness.
func TestThePlusFormWithNoValuesLeavesTheArrayAlone(t *testing.T) {
	out, errs, st := setArrayRun(t, `set -A c 1 2 3
set +A c
echo "n=${#c[@]} [${c[@]}]"`, nil, Diagnostics{})
	want := "n=3 [1 2 3]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("set +A with no values = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The minus form with no values: one dialect leaves an array with no elements
// and the other unsets the name. Both count 0, so the difference shows only
// to a script that asks whether the name is set at all — which is why the
// test asks `${e[@]+yes}` and not the count.
func TestSetArrayWithNoValuesLeavesAnEmptyArray(t *testing.T) {
	out, errs, st := setArrayRun(t, `set -A e 1 2
set -A e
echo "n=${#e[@]} set=[${e[@]+yes}]"`, nil, Diagnostics{})
	want := "n=0 set=[yes]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("set -A with no values = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

func TestSetArrayWithNoValuesUnsetsUnderTheOtherAnswer(t *testing.T) {
	out, errs, st := setArrayRun(t, `set -A e 1 2
set -A e
echo "n=${#e[@]} set=[${e[@]+yes}]"`, func(s *Semantics) {
		s.SetArrayWithNoValuesUnsetsTheName = Yes
	}, Diagnostics{})
	want := "n=0 set=[]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("set -A with no values, unsetting = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

func TestSetArrayWithNoValuesRefusesWithoutTheAxis(t *testing.T) {
	_, errs, st := setArrayRun(t, `set -A e`, func(s *Semantics) {
		s.SetArrayWithNoValuesUnsetsTheName = Unspecified
	}, Diagnostics{})
	want := "testsh: `set -A name` with no values unsetting the name: " +
		"the shells disagree here and no dialect was chosen\n"
	if errs != want || st != 2 {
		t.Errorf("set -A with no values and no axis = %q (status %d), want %q", errs, st, want)
	}
}

// Where the name ends the options, every word behind it is a value — dash
// words and `--` included.
func TestTheNameEndsTheOptions(t *testing.T) {
	out, errs, st := setArrayRun(t, `set -A f -x -y
echo "1 n=${#f[@]} [${f[@]}]"
set -A g -- 1 2
echo "2 n=${#g[@]} [${g[@]}]"`, nil, Diagnostics{})
	want := "1 n=2 [-x -y]\n2 n=3 [-- 1 2]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("dash words after the name = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// And where the options carry on, the same two lines answer the other way:
// `-x` is an option and the `--` ends them, so the values are exactly the
// words that would have become the positional parameters. **One axis for
// both**, because whether `--` is an operand *is* whether options are still
// being read.
func TestTheOptionsCanCarryOnPastTheName(t *testing.T) {
	carryOn := func(s *Semantics) { s.SetArrayOptionsContinuePastTheName = Yes }
	out, errs, st := setArrayRun(t, `set -A g -- 1 2
echo "n=${#g[@]} [${g[@]}] x=[$-]"`, carryOn, Diagnostics{})
	if out != "n=2 [1 2] x=[]\n" || st != 0 || errs != "" {
		t.Errorf("`--` where options carry on = %q (stderr %q, status %d), want two "+
			"elements and no `--` among them", out, errs, st)
	}
	// The dash word really is taken as an option there, which is the half a
	// test on `--` alone cannot see.
	out, _, st = setArrayRun(t, `set -A f -x 1
echo "n=${#f[@]} [${f[@]}]"`, carryOn, Diagnostics{})
	if out != "n=1 [1]\n" || st != 0 {
		t.Errorf("a dash word where options carry on = %q (status %d), want it read "+
			"as an option and one element left", out, st)
	}
}

func TestTheOptionsAfterTheNameRefuseWithoutTheAxis(t *testing.T) {
	_, errs, st := setArrayRun(t, `set -A a x`, func(s *Semantics) {
		s.SetArrayOptionsContinuePastTheName = Unspecified
	}, Diagnostics{})
	want := "testsh: the words after `set -A name` read as options rather than as " +
		"values: the shells disagree here and no dialect was chosen\n"
	if errs != want || st != 2 {
		t.Errorf("set -A with no axis = %q (status %d), want %q", errs, st, want)
	}
}

// The letters in front of the array letter still apply, which is what makes
// it one bundle rather than a word of its own.
func TestTheLettersInFrontOfTheArrayLetterStillApply(t *testing.T) {
	out, errs, st := setArrayRun(t, `set -uA nn 1 2
echo "n=${#nn[@]} [${nn[@]}]"
echo "flag=[${-}]"`, nil, Diagnostics{})
	if errs != "" || st != 0 {
		t.Fatalf("set -uA = stderr %q status %d", errs, st)
	}
	if got := "n=2 [1 2]\n"; len(out) < len(got) || out[:len(got)] != got {
		t.Errorf("set -uA = %q, want it to start %q", out, got)
	}
	if !bytes.ContainsRune([]byte(out), 'u') {
		t.Errorf("set -uA = %q, want nounset set as well as the array assigned", out)
	}
}

// The positional parameters are left alone. `set` replacing them is the
// letter's whole other meaning, and an array assignment must not reach it.
func TestSetArrayLeavesThePositionalParametersAlone(t *testing.T) {
	out, errs, st := setArrayRun(t, `set -- one two three
set -A a x y
echo "pos=[$*] n=$# arr=[${a[@]}]"`, nil, Diagnostics{})
	want := "pos=[one two three] n=3 arr=[x y]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("set -A after set -- = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// A name that is not a name is refused, in the dialect's words and with the
// dialect's status, through the same check every other builtin's operands go
// through — the fatality included, which both shells with the letter answer
// yes: the script ends there.
func TestABadArrayNameIsRefused(t *testing.T) {
	out, errs, st := setArrayRun(t, `set -A 1bad v
echo "after"`, nil, Diagnostics{
		BuiltinBadName:       map[string]string{"set": "not an identifier: %[2]s"},
		BuiltinBadNameStatus: 1,
	})
	want := "testsh: not an identifier: 1bad\n"
	if errs != want || st != 1 {
		t.Errorf("set -A 1bad = %q (status %d), want %q", errs, st, want)
	}
	if out != "" {
		t.Errorf("set -A 1bad printed %q, want nothing — the refusal is fatal here", out)
	}
	// And with the fatality answered the other way the line after it runs,
	// which is what makes this the name check and not a parse failure.
	out, _, _ = setArrayRun(t, `set -A 1bad v
echo "after"`, func(s *Semantics) { s.BadNameToDeclarationFatal = No }, Diagnostics{
		BuiltinBadName: map[string]string{"set": "not an identifier: %[2]s"},
	})
	if out != "after\n" {
		t.Errorf("set -A 1bad under a non-fatal dialect printed %q, want the line "+
			"after it", out)
	}
}

// A frozen name is refused before the store, not after it. storeArray keeps
// the scalar view in step and *that* call is guarded, so a check made
// afterwards prints its complaint with the array already written — which is
// the bug refuseReadonly exists to have fixed once.
func TestSetArrayOverAFrozenNameStoresNothing(t *testing.T) {
	out, errs, st := setArrayRun(t, `set -A ro keep
readonly ro
set -A ro gone
echo "st=$? [${ro[@]}]"`, func(s *Semantics) {
		s.ReadonlyReassignmentFatal = No
		s.ReadonlyReassignmentByDeclarationFatal = No
	}, Diagnostics{})
	want := "st=1 [keep]\n"
	if out != want || st != 0 {
		t.Errorf("set -A over a frozen name = %q (stderr %q, status %d), want %q — "+
			"the old elements standing", out, errs, st, want)
	}
	if errs == "" {
		t.Error("set -A over a frozen name said nothing")
	}
}

// A subscripted name is refused by name. Both shells with the letter place
// the values from that subscript on, and the placement reads the array base —
// which is the whole of what could have gone wrong in silence.
func TestASubscriptedArrayNameIsNamedAsMissing(t *testing.T) {
	out, errs, st := setArrayRun(t, `set -A 'sub[2]' v
echo "after"`, nil, Diagnostics{})
	want := "testsh: set: -A with a subscripted name is not implemented yet\n"
	if errs != want || st != 0 {
		t.Errorf("set -A sub[2] = %q (status %d), want %q", errs, st, want)
	}
	if out != "after\n" {
		t.Errorf("set -A sub[2] printed %q, want only the line after it", out)
	}
}

// An association is refused by name too, and for a different reason: it is a
// different operation under the same spelling and the two shells do not agree
// which. One reads the operands as key-and-value pairs and the other stores
// them counted; either choice answers the other shell's script wrongly and
// says nothing about it.
func TestAnAssociationTargetIsNamedAsMissing(t *testing.T) {
	_, errs, st := setArrayRun(t, `typeset -A m
set -A m k1 v1`, func(s *Semantics) {
		s.DeclareOptions = "aAilprux"
	}, Diagnostics{})
	want := "testsh: set: -A over an association is not implemented yet\n"
	if errs != want || st != 2 {
		t.Errorf("set -A over an association = %q (status %d), want %q", errs, st, want)
	}
}

// `set -A` with nothing after it: where the dialect words the refusal it gets
// its words and its usage block, and where it does not the *listing* it would
// have written is named as missing.
func TestSetArrayWithNoNameSaysWhichOperandIsMissing(t *testing.T) {
	_, errs, st := setArrayRun(t, `set -A`, nil, Diagnostics{
		SetArrayNeedsAName: "set: %[1]s: name argument expected",
		BuiltinUsage:       map[string]string{"set": "Usage: set [-A name] [arg ...]"},
	})
	want := "testsh: set: -A: name argument expected\ntestsh: Usage: set [-A name] [arg ...]\n"
	if errs != want || st != 2 {
		t.Errorf("set -A with no name = %q (status %d), want %q", errs, st, want)
	}
	// The plus spelling is echoed back as written, the way every other
	// refused `set` letter is.
	_, errs, _ = setArrayRun(t, `set +A`, nil, Diagnostics{
		SetArrayNeedsAName: "set: %[1]s: name argument expected",
	})
	if want := "testsh: set: +A: name argument expected\n"; errs != want {
		t.Errorf("set +A with no name = %q, want %q", errs, want)
	}
}

func TestSetArrayWithNoNameNamesTheListingAsMissing(t *testing.T) {
	_, errs, st := setArrayRun(t, `set -A`, nil, Diagnostics{})
	want := "testsh: set: -A: a listing is not implemented yet\n"
	if errs != want || st != 2 {
		t.Errorf("set -A with no name = %q (status %d), want %q", errs, st, want)
	}
}

// Where the dialect has no array letter the word is somebody else's invalid
// option, and the refusal already in place is that shell's own — read rather
// than asked, so nothing complains about a missing dialect.
func TestWithoutTheLetterTheWordIsAnInvalidOption(t *testing.T) {
	_, errs, st := setArrayRun(t, `set -A a x`, func(s *Semantics) {
		s.SetArrayLetter = Unspecified
	}, Diagnostics{
		SetInvalidOptionLetter: "set: %[1]s: invalid option",
		SetInvalidOptionStatus: 2,
	})
	want := "testsh: set: -A: invalid option\n"
	if errs != want || st != 2 {
		t.Errorf("set -A with no letter = %q (status %d), want %q", errs, st, want)
	}
	if bytes.Contains([]byte(errs), []byte("`set -A")) {
		t.Errorf("a shell without the letter was asked one of the letter's own "+
			"axes: %q", errs)
	}
}
