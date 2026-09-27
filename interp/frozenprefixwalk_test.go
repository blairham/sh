// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A frozen name in an assignment prefix is refused **at the entry it is
// about**: the entries in front of it have already expanded, and the entries
// behind it have not.
//
// Named for the fields and never for a shell. The refusal was reported for the
// whole prefix ahead of the walk, so it came out in front of every value
// however many entries stood between it and the start of the line (#4783).
//
// A prefix entry writes a byte of its own on the way, which is what makes the
// order readable at all: a count would say the same thing about either order,
// and a status would say nothing about either.

// frozenWalkRun runs one prefixed command with `r` frozen and returns what the
// shell wrote, in order.
func frozenWalkRun(t *testing.T, prefix, command string, first Answer, order FrozenPrefixCheckOrder) string {
	t.Helper()
	sem := permissive()
	sem.FrozenPrefixIsCheckedBeforeItsValue = first
	sem.PrefixToAFrozenNameIsCheckedFirst = order
	sem.PrefixToARegularBuiltinIsRefused = Yes
	out, _ := sourceRun(t, t.TempDir(),
		"f() { :; }\nreadonly r=1\n"+prefix+" "+command+"\n", sem, Diagnostics{})
	return out
}

// marks is the letters a run wrote, in the order it wrote them, with the
// refusal spelled `!`. Reading positions rather than lines is what keeps the
// rows from depending on a dialect's wording.
func marks(out string) string {
	var b strings.Builder
	for _, line := range strings.Split(out, "\n") {
		switch {
		case line == "A" || line == "B" || line == "R":
			b.WriteString(line)
		case strings.Contains(line, "r"):
			b.WriteString("!")
		}
	}
	return b.String()
}

// The order, one answer of each axis at a time.
func TestAFrozenPrefixIsRefusedWhereItStands(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		prefix string
		first  Answer
		order  FrozenPrefixCheckOrder
		want   string
	}{
		{
			"an entry in front of the frozen one, checked with the command",
			"a=$(echo A >&2) r=$(echo R >&2)", No, FrozenPrefixCheckedWithTheCommand, "AR!",
		},
		{
			"the same, with the name checked first",
			"a=$(echo A >&2) r=$(echo R >&2)", Yes, FrozenPrefixCheckedFirst, "A!",
		},
		{
			"two in front of it",
			"a=$(echo A >&2) b=$(echo B >&2) r=$(echo R >&2)", Yes, FrozenPrefixCheckedFirst, "AB!",
		},
		{
			"an entry behind it, which does not expand before the refusal",
			"r=$(echo R >&2) a=$(echo A >&2)", Yes, FrozenPrefixCheckedFirst, "!",
		},
		{
			"the control: the frozen entry first, with its value evaluated",
			"r=$(echo R >&2) a=$(echo A >&2)", No, FrozenPrefixCheckedWithTheCommand, "R!",
		},
		{
			"the control: nothing frozen at all",
			"a=$(echo A >&2) b=$(echo B >&2)", Yes, FrozenPrefixCheckedFirst, "AB",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := marks(frozenWalkRun(t, c.prefix, "f", c.first, c.order)); got != c.want {
				t.Errorf("%s: order %q, want %q", c.prefix, got, c.want)
			}
		})
	}
}

// And it holds in front of every kind of command the prefix can stand before,
// because the routes they take are separate pieces of code and the pass this
// replaces was ahead of all of them.
func TestTheFrozenPrefixWalkHoldsForEveryCommandKind(t *testing.T) {
	t.Parallel()
	for _, command := range frozenValueCommands {
		for _, c := range []struct {
			first Answer
			order FrozenPrefixCheckOrder
			want  string
		}{
			{Yes, FrozenPrefixCheckedFirst, "A!"},
			{No, FrozenPrefixCheckedWithTheCommand, "AR!"},
		} {
			out := frozenWalkRun(t, "a=$(echo A >&2) r=$(echo R >&2)", command, c.first, c.order)
			if got := marks(out); got != c.want {
				t.Errorf("%q at %v/%v: order %q, want %q", command, c.first, c.order, got, c.want)
			}
		}
	}
}

// A value in front of the frozen name that will not expand stops the walk, and
// the refusal is never written — which is the row that says the walk is one
// pass rather than two with the order swapped.
func TestAFailureInFrontOfAFrozenNameIsWhatIsReported(t *testing.T) {
	t.Parallel()
	for _, order := range evaluateFirstOrders {
		for _, first := range []Answer{Yes, No} {
			out := frozenWalkRun(t, "a=$((1/0)) r=$(echo R >&2)", "f", first, order)
			if !strings.Contains(out, "division by zero") {
				t.Errorf("%v/%v: got %q, want the value's own failure", first, order, out)
			}
			if strings.Contains(out, "R") {
				t.Errorf("%v/%v: got %q, want nothing behind it expanded", first, order, out)
			}
		}
	}
}

// The entries in front are held under their names as they expand, so an entry
// between them reads this walk's own value rather than the one the shell holds
// — the same rule interp/prefixsees.go states for the two walks beside this
// one, and a place it could have been left out.
func TestThePrefixWalkHoldsItsValuesForTheEntriesBehindThem(t *testing.T) {
	t.Parallel()
	sem := permissive()
	sem.FrozenPrefixIsCheckedBeforeItsValue = Yes
	sem.PrefixToAFrozenNameIsCheckedFirst = FrozenPrefixCheckedFirst
	sem.PrefixToARegularBuiltinIsRefused = Yes
	out, _ := sourceRun(t, t.TempDir(),
		"f() { :; }\nreadonly r=1\nk=v1\nk=v2 b=$(echo ${k} >&2) r=1 f\n", sem, Diagnostics{})
	if !strings.Contains(out, "v2") {
		t.Errorf("got %q, want the entry behind to read this walk's own value", out)
	}
}
