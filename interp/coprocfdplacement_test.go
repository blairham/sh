// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Where a coprocess's near ends land in the descriptor table.
//
// One dialect puts them at the top of it, out of the way of the numbers a
// script allocates for itself. Measured 2026-09-13 on bash 5.3.15:
// `coproc CP { read x; }; echo "${CP[@]}"` is `63 60`, and this shell answered
// `10 11` — exactly on top of the numbers `exec {v}>f` hands out (#2596).
func TestACoprocessTakesTheTopOfTheDescriptorTable(t *testing.T) {
	t.Parallel()
	out := runCoprocPlacement(t, CoprocEndsAtTheTopOfTheTable, nil,
		`coproc CP { /bin/cat; }
echo "cp=${CP[@]}"`)
	if want := "cp=63 60\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// And it does not disturb the allocator a script asks for a number through:
// with a coprocess running, `exec {v}>f` still answers the dialect's base.
//
// Measured the same day — bash says 10 with a coprocess running and 10 without
// one — and it is the half of the divergence a script feels rather than
// prints, since this shell answered 12.
func TestAnAllocatedDescriptorIgnoresARunningCoprocess(t *testing.T) {
	t.Parallel()
	out := runCoprocPlacement(t, CoprocEndsAtTheTopOfTheTable, nil,
		`coproc CP { /bin/cat; }
exec {v}>/dev/null
echo "v=$v"`)
	if want := "v=10\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// Four coprocesses in a row, which is what says the rule is "the four highest
// free numbers, keep the first and the fourth" rather than "63 and 60".
//
// Measured 2026-09-13 on bash 5.3.15 with four coprocesses started and none of
// them ended. The two numbers in the middle are the child's, closed in the
// parent straight after the fork, so the next coprocess finds them free again
// — which is why the pairs walk downwards unevenly.
func TestFourCoprocessesWalkDownTheTable(t *testing.T) {
	t.Parallel()
	out := runCoprocPlacement(t, CoprocEndsAtTheTopOfTheTable, nil,
		`coproc A { /bin/cat; }
echo "A=${A[@]}"
coproc B { /bin/cat; }
echo "B=${B[@]}"
coproc C { /bin/cat; }
echo "C=${C[@]}"
coproc D { /bin/cat; }
echo "D=${D[@]}"`)
	want := "A=63 60\nB=62 58\nC=61 56\nD=59 54\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// A descriptor the script parked in the way moves the pair by exactly what the
// rule predicts, and the four probes below are what tell that rule apart from
// "one below the highest free" and from "63 and 60 unless taken".
//
// Measured 2026-09-13 on bash 5.3.15, one coprocess with the listed numbers
// held by `exec N>/dev/null` first.
func TestAParkedDescriptorMovesTheCoprocessEnds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ park, want string }{
		{"63", "62 59"},
		{"62", "63 59"},
		{"61", "63 59"},
		{"60", "63 59"},
		{"63 62 61 60", "59 56"},
	} {
		t.Run(tc.park, func(t *testing.T) {
			t.Parallel()
			src := "exec"
			for _, fd := range strings.Fields(tc.park) {
				src += " " + fd + ">/dev/null"
			}
			out := runCoprocPlacement(t, CoprocEndsAtTheTopOfTheTable, nil,
				src+"\ncoproc CP { /bin/cat; }\necho \"${CP[@]}\"")
			if want := tc.want + "\n"; out != want {
				t.Errorf("got %q, want %q", out, want)
			}
		})
	}
}

// The other answer is the one this shell held everywhere before the axis:
// the ends are numbered where any other allocation goes.
//
// zsh and ksh93 publish no array, so no script prints these numbers there —
// but what is left for the shell to hand out afterwards says the ends are in
// that region, which bash's are not. See Semantics.CoprocessEndPlacement.
func TestACoprocessOtherwiseTakesTheOrdinaryNumbers(t *testing.T) {
	t.Parallel()
	out := runCoprocPlacement(t, CoprocEndsWhereAnyDescriptorGoes, nil,
		`coproc CP { /bin/cat; }
echo "cp=${CP[@]}"
exec {v}>/dev/null
echo "v=$v"`)
	if want := "cp=10 11\nv=12\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The top of the table is conditional on the process being able to hold it.
//
// Measured 2026-09-13 by sweeping `ulimit -n` on bash 5.3.15 from 20 to 256:
// the pair is `63 60` at every limit of 64 and above and the ends are not
// moved at all at 63 or below. What bash publishes instead is its own raw pipe
// numbers, which no allocator here produces; this shell falls back to the
// ordinary allocation, and that difference is recorded rather than reproduced.
func TestTheTopOfTheTableGivesWayToTheOpenFileLimit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		soft int64
		want string
	}{
		{"63 is out of reach", 63, "10 11\n"},
		{"63 is the last legal number", 64, "63 60\n"},
		{"room to spare", 1024, "63 60\n"},
		{"no limit at all", RlimitInfinity, "63 60\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			limit := func(Resource) (int64, int64, error) { return tc.soft, tc.soft, nil }
			out := runCoprocPlacement(t, CoprocEndsAtTheTopOfTheTable, limit,
				"coproc CP { /bin/cat; }\necho \"${CP[@]}\"")
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// runCoprocPlacement runs src with the coprocess keyword, the array model and
// the given placement, optionally under an embedder-supplied open-file limit.
//
// Multi-digit descriptor numbers are on because the parking cases write
// `exec 63>/dev/null`, which is the only way for a script to reach a number
// the shell would otherwise pick for itself.
func runCoprocPlacement(t *testing.T, where CoprocEndPlacement,
	limit func(Resource) (int64, int64, error), src string,
) string {
	t.Helper()
	d := syntax.Core()
	d.Coproc = true
	d.CoprocName = true
	d.FdVariableRedirections = true
	d.MultiDigitFdNumber = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatal(err)
	}
	sem := PosixSemantics()
	// POSIX has no coprocess and answers nothing about one; these cases are
	// about where the ends are numbered, which needs the array to read them
	// back through.
	sem.CoprocEndsInAnArray = Yes
	sem.CoprocessEndPlacement = where
	out := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: out, Stderr: &strings.Builder{}, GetRlimit: limit,
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return out.String()
}
