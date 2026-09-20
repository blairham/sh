// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// kshTrace is kshOut with the trace on its own stream, so a row asserts what
// `set -x` wrote and not what the script printed.
func kshTrace(t *testing.T, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, ksh.Dialect())
	if err != nil {
		return "parse: " + err.Error(), -1
	}
	var out, errOut bytes.Buffer
	s, d := ksh.Semantics(), ksh.Diagnostics()
	r := &interp.Runner{
		Stdout: &out, Stderr: &errOut,
		Semantics: &s, Diagnostics: &d, Name: "ksh", Dialect: presetDialect(),
	}
	ksh.Apply(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatal(rerr)
	}
	return errOut.String(), st
}

// A compound literal is traced as the assignments its body performs, one line
// each, rather than as the parentheses it was written with. Measured on
// ksh93u+ 2012-08-01, 2026-09-14, `env -i PATH=/usr/bin:/bin` with a scratch
// HOME, over `-c`, a script file and standard input alike.
//
// It is the explanation of what #1959 recorded from the other end: `set -x;
// b=(); echo after` writes no line for the assignment here, where bash writes
// `b=()` and zsh writes `b=( )`. Not a rule about an empty array literal —
// `b=()` is an empty *compound* in this shell, so it performs no assignments
// and there is nothing to write. A reading that special-cased the empty
// literal would have written one line for `c=(a=1 b=2)` and passed the row it
// was measured from.
func TestACompoundLiteralIsTracedAsItsMembers(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`set -x; c=(a=1); echo after`, "+ c.a=1\n+ echo after\n"},
		// One line per member, in the order the body wrote them.
		{`set -x; c=(a=1 b=2); echo after`, "+ c.a=1\n+ c.b=2\n+ echo after\n"},
		// A nested body is its members too, under their own names.
		{`set -x; c=(a=1 b=(y=2)); echo after`, "+ c.a=1\n+ c.b.y=2\n+ echo after\n"},
		// The empty compound: no members, so no line — and the command after
		// it is what says the trace is still running.
		{`set -x; b=(); echo after`, "+ echo after\n"},
		// The append spelling of an empty body is the same nothing, over a
		// name already holding an array.
		{`set -x; b=(1); b+=(); echo after`, "+ b=( 1 )\n+ echo after\n"},
		// And the control: a literal of plain words is not a compound and is
		// traced as the literal it is, so the rule above is about the
		// construct rather than about the parentheses.
		{`set -x; b=(1 2); echo after`, "+ b=( 1 2 )\n+ echo after\n"},
	} {
		if out, st := kshTrace(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// And where those member lines stand when the compound is written as a
// declaration utility's **operand** rather than as a statement of its own:
// in front of the command's line, not behind it — interp/compoundoperandorder.go
// and #3804, where this shell wrote the command and then what the operand
// did.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-20, `env -i PATH=/usr/bin:/bin
// LC_ALL=C` over a script file with standard input on the null device. Each
// row below is that shell's bytes.
func TestACompoundOperandIsPerformedInFrontOfTheCommand(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"set -x\ntypeset c=(a=1 b=2)\n", "+ c.a=1\n+ c.b=2\n+ typeset c\n"},
		{"set -x\ntypeset -C d=(p=3)\n", "+ d.p=3\n+ typeset -C d\n"},
		// A nested body is still one operand, and all of it goes in front.
		{"set -x\ntypeset -C d=(p=3 q=(r=4))\n", "+ d.p=3\n+ d.q.r=4\n+ typeset -C d\n"},
		// The body standing where an element's value goes, and on a table's
		// base — two more constructs behind the same parentheses, and the
		// ordering reaches them because it is about the operand rather than
		// about which store the body ends up in.
		{"set -x\ntypeset c[1]=(a=1)\n", "+ c[1].a=1\n+ typeset c\n"},
		{"set -x\ntypeset -A m=(p=1)\n", "+ m[0].p=1\n+ typeset -A m\n"},
		// `readonly` is a declaration utility here — it declares a local —
		// so it takes the same order rather than the assign-first one the
		// dialect that reads it as its own word needs.
		{"set -x\nreadonly c=(a=1)\n", "+ c.a=1\n+ readonly c\n"},
		// The control under every row above: an array *literal* operand was
		// already written in front, and a scalar one too, so a change that
		// simply moved every operand's line would pass the rows and lose
		// these.
		{"set -x\ntypeset e=(1 2)\n", "+ e=( 1 2 )\n+ typeset e\n"},
		{"set -x\ntypeset s=5\n", "+ s=5\n+ typeset s\n"},
	} {
		if out, st := kshTrace(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// It is the **work** that is in front and not a line written early, which is
// the pair of rows that rules out rendering the body somewhere else: a
// substitution inside a member's value runs there too, and a member reading
// an earlier one resolves what that one has already stored.
func TestACompoundOperandsWorkIsInFrontOfTheCommand(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"set -x\ntypeset c=(a=1 b=$(echo two))\n", "+ c.a=1\n+ echo two\n+ c.b=two\n+ typeset c\n"},
		{"set -x\ntypeset c=(a=1 b=${c.a})\n", "+ c.a=1\n+ c.b=1\n+ typeset c\n"},
	} {
		if out, st := kshTrace(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// Operands interleave by kind and not by position: what was already written
// in front of the command stays in front of the body's lines, which is the
// order the script wrote the operands in.
func TestOperandsOfEveryKindKeepTheOrderTheyWereWrittenIn(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"set -x\ntypeset x=1 c=(a=1)\n", "+ x=1\n+ c.a=1\n+ typeset x c\n"},
		{"set -x\ntypeset e=(1 2) k=(a=1)\n", "+ e=( 1 2 )\n+ k.a=1\n+ typeset e k\n"},
		{"set -x\ntypeset c=(a=1) d=(b=2)\n", "+ c.a=1\n+ d.b=2\n+ typeset c d\n"},
	} {
		if out, st := kshTrace(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// The row that says the command's line is still written *before* the utility
// runs, which is what rules out deferring it past the builtin: a `-p`
// declaration writes the member, then the command, then its own listing.
//
// Both streams into one buffer, because the whole row is about where the
// utility's output falls among the trace lines.
func TestAPrintingDeclarationWritesItsMemberThenItsLineThenItsListing(t *testing.T) {
	const src = "set -x\ntypeset -p c=(a=1)\n"
	const want = "+ c.a=1\n+ typeset -p c\ntypeset -C c=(a=1)\n"
	if out, st := kshOut(t, src); out != want || st != 0 {
		t.Errorf("%s\n got %q at %d\nwant %q at 0", src, out, st, want)
	}
}

// A target that cannot be assigned writes **no** trace line at all — not the
// member's and not the command's — which is the same claim seen from its
// failing side: a command whose operand never happened was never traced.
func TestARefusedCompoundOperandWritesNoTraceLine(t *testing.T) {
	const src = "typeset -r c=(z=9)\nset -x\ntypeset c=(a=2)\n"
	out, st := kshOut(t, src)
	if strings.Contains(out, "+ ") {
		t.Errorf("%q wrote a trace line: %q", src, out)
	}
	if st == 0 {
		t.Errorf("%q reported 0, want a refusal", src)
	}
}

// And the control the whole change is held against: the store has **not**
// moved. A compound body is still assigned after the declaration utility has
// run, so the declaration decides the scope it lands in — the order
// interp/arrayoperandorder.go states and #3824 was the cost of getting wrong.
//
// Written as the trace and the values at once, so a change that moved the
// store in front of the utility to get the lines in front would fail here
// rather than pass quietly: the caller's member would come back `a=1`.
func TestACompoundOperandStillLandsInTheScopeTheDeclarationMade(t *testing.T) {
	const src = `set -x
c=(z=9)
function f { typeset c=(a=1); print "in=[${c.a}][${c.z}]"; }
f
print "after=[${c.z}][${c.a}]"
`
	const want = "+ c.z=9\n" +
		"+ f\n" +
		"in=[1][]\n" +
		"+ print 'after=[9][]'\n" +
		"after=[9][]\n"
	if out, st := kshOut(t, src); out != want || st != 0 {
		t.Errorf("%s\n got %q at %d\nwant %q at 0", src, out, st, want)
	}
}
