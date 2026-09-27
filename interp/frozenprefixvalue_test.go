// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether a frozen name in an assignment prefix is refused **without**
// evaluating what it was being given — Semantics.FrozenPrefixIsCheckedBeforeItsValue.
// One answer writes the frozen name and never runs the value; the other runs
// the value and writes its failure instead. Named for the field and never for
// a shell.

// frozenValueRun runs `readonly r=1; r=<value> <command>` with one answer to
// the axis and one to the neighbor it is so easily confused with.
func frozenValueRun(t *testing.T, value, command string, first Answer, order FrozenPrefixCheckOrder) string {
	t.Helper()
	sem := permissive()
	sem.FrozenPrefixIsCheckedBeforeItsValue = first
	sem.PrefixToAFrozenNameIsCheckedFirst = order
	sem.PrefixToARegularBuiltinIsRefused = Yes
	out, _ := sourceRun(t, t.TempDir(),
		"f() { :; }\nreadonly r=1\nr="+value+" "+command+"\n", sem, Diagnostics{})
	return out
}

// Every kind of command the prefix can stand in front of, because the routes
// they take through the interpreter are four different pieces of code and the
// gap this closes was on two of them.
var frozenValueCommands = []string{"true", ":", "f", "/nonexistent/zz"}

// The three pairs a real column holds with the evaluate-first answer. The
// *order against the redirections* is a field of its own and every one of its
// three answers is paired with this one somewhere in the panel, which is
// exactly why the two are separate fields — see the control at the bottom.
var evaluateFirstOrders = []FrozenPrefixCheckOrder{
	FrozenPrefixCheckedWithTheCommand,
	FrozenPrefixCheckedFirst,
	FrozenPrefixCheckedFirstWhereItPersists,
}

// The answer that evaluates first: the value's failure is what is written, and
// the frozen name is never mentioned.
func TestAFrozenPrefixCanHaveItsValueEvaluatedFirst(t *testing.T) {
	t.Parallel()
	for _, order := range evaluateFirstOrders {
		for _, command := range frozenValueCommands {
			out := frozenValueRun(t, "$((1/0))", command, No, order)
			if !strings.Contains(out, "division by zero") {
				t.Errorf("%v %q: got %q, want the value's own failure", order, command, out)
			}
			if strings.Contains(out, "read") {
				t.Errorf("%v %q: got %q, want no word about the frozen name", order, command, out)
			}
		}
	}
}

// And the answer that checks the name first: the refusal, and the value is
// never evaluated at all — which is a side effect rather than a count, so the
// probe writes a byte instead of failing.
//
// One order, because the one column that gives this answer also checks the
// name ahead of its redirections, and the other two pairings are cells no
// shell holds. Pinning a cell nobody has is how a reading nobody measured
// gets asserted.
func TestAFrozenPrefixCanBeRefusedBeforeItsValueRuns(t *testing.T) {
	t.Parallel()
	for _, order := range []FrozenPrefixCheckOrder{FrozenPrefixCheckedFirst} {
		for _, command := range frozenValueCommands {
			out := frozenValueRun(t, "$((1/0))", command, Yes, order)
			if strings.Contains(out, "division by zero") {
				t.Errorf("%v %q: got %q, want the value never evaluated", order, command, out)
			}
			if !strings.Contains(out, "r") {
				t.Errorf("%v %q: got %q, want the frozen name refused", order, command, out)
			}
			side := frozenValueRun(t, "$(echo SIDE >&2; echo v)", command, Yes, order)
			if strings.Contains(side, "SIDE") {
				t.Errorf("%v %q: got %q, want the value's side effect never run", order, command, side)
			}
		}
	}
}

// The side effect is the half that says this is an *ordering* fact and not a
// fact about failures: a value that merely writes a byte still runs under one
// answer and not under the other, and the refusal is written either way.
func TestAFrozenPrefixValueRunsForItsSideEffectToo(t *testing.T) {
	t.Parallel()
	for _, command := range frozenValueCommands {
		out := frozenValueRun(t, "$(echo SIDE >&2; echo v)", command, No,
			FrozenPrefixCheckedWithTheCommand)
		if !strings.Contains(out, "SIDE") {
			t.Errorf("%q: got %q, want the value's side effect", command, out)
		}
		if !strings.Contains(out, "r") {
			t.Errorf("%q: got %q, want the frozen name still refused", command, out)
		}
	}
}

// The control that keeps the two neighboring fields apart. The order against
// the **redirections** is a field of its own and it does not answer this one:
// with the name checked first there — bash's and BusyBox ash's answer — the
// value is still evaluated under the answer that says so, which is the pair
// that one column really holds.
func TestTheTwoFrozenPrefixCheckOrdersAreSeparateQuestions(t *testing.T) {
	t.Parallel()
	out := frozenValueRun(t, "$((1/0))", "true", No, FrozenPrefixCheckedFirst)
	if !strings.Contains(out, "division by zero") || strings.Contains(out, "read") {
		t.Errorf("got %q, want the value's failure although the name is checked first", out)
	}
	out = frozenValueRun(t, "$((1/0))", "true", Yes, FrozenPrefixCheckedFirst)
	if strings.Contains(out, "division by zero") {
		t.Errorf("got %q, want the value never evaluated at the same order", out)
	}
}
