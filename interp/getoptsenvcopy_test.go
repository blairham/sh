// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// Both answers of Semantics.GetoptsLeavesTheEnvironmentCopyOfItsOwnParameters,
// read back through a child. Each row exports OPTARG or OPTIND, lets `getopts`
// write it, and asks `env` what a child is handed; the rows after the first
// two are the ways the held copy is kept or let go of, so a hold that leaked
// past an assignment, or one a second `getopts` refreshed, fails here.
func TestGetoptsLeavesTheEnvironmentCopyOfItsOwnParameters(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, held, refreshed string }{
		{"OPTIND", `export OPTIND=1; getopts x: o -x6`, "OPTIND=1\n", "OPTIND=2\n"},
		{"OPTARG", `export OPTARG=pre; getopts x: o -x6`, "OPTARG=pre\n", "OPTARG=6\n"},
		{"a second call", `export OPTIND=1; getopts x: o -x6 -x7; getopts x: o -x6 -x7`, "OPTIND=1\n", "OPTIND=3\n"},
		{"an unset name stays out", `export OPTARG=pre; unset OPTARG; export OPTARG; getopts x: o -x6`, "", "OPTARG=6\n"},
		{"an assignment refreshes", `export OPTIND=1; getopts x: o -x6; OPTIND=$OPTIND`, "OPTIND=2\n", "OPTIND=2\n"},
		{"a bare export does not", `export OPTIND=1; getopts x: o -x6; export OPTIND`, "OPTIND=1\n", "OPTIND=2\n"},
		{"in a subshell's child", `export OPTIND=1; getopts x: o -x6; (`, "OPTIND=1\n", "OPTIND=2\n"},
		{"a subshell's hold stays in it", `export OPTIND=1; getopts x: o -x6; export OPTARG=pre; (OPTARG=mid; getopts x: o -x6 -x7)`, "OPTARG=pre\nOPTIND=1\n", "OPTARG=pre\nOPTIND=2\n"},
		{"the name operand refreshes", `export OPTARG=pre; getopts x: OPTARG -x6`, "OPTARG=x\n", "OPTARG=x\n"},
	} {
		for _, answer := range []Answer{Yes, No} {
			t.Run(tc.name+"/"+answer.String(), func(t *testing.T) {
				sem := permissive()
				sem.GetoptsLeavesTheEnvironmentCopyOfItsOwnParameters = answer
				// `export OPTIND=1` asks what an assignment to the cursor
				// means, which is a question of its own and beside the point.
				sem.GetoptsAssignmentRestartsWord = Yes
				src := tc.src + "; env | grep -E '^(OPTARG|OPTIND)=' | sort"
				if tc.src[len(tc.src)-1] == '(' {
					src = tc.src + " env | grep -E '^(OPTARG|OPTIND)=' )"
				}
				out, _ := run(t, src, withSem(sem))
				want := tc.refreshed
				if answer == Yes {
					want = tc.held
				}
				if out != want {
					t.Errorf("got %q, want %q", out, want)
				}
			})
		}
	}
}

// A parameter that arrived in the environment and was never assigned is
// exported too, and the copy held for it is the entry that came in — not
// none, which is what a hold read only from the shell's own table would leave.
// It carries no export attribute in the cell, so the assignment after it is
// one the plain store takes, and that road lets the hold go as well.
func TestGetoptsHoldsAnInheritedParametersEntry(t *testing.T) {
	t.Parallel()
	for _, answer := range []Answer{Yes, No} {
		t.Run(answer.String(), func(t *testing.T) {
			sem := permissive()
			sem.GetoptsLeavesTheEnvironmentCopyOfItsOwnParameters = answer
			out, _ := run(
				t,
				`getopts x: o -x6; echo $OPTARG; env | grep -E '^OPTARG='; OPTARG=new; env | grep -E '^OPTARG='`,
				func(r *Runner) {
					r.Semantics = &sem
					r.Env = append(r.Env, "OPTARG=pre")
				},
			)
			want := "6\nOPTARG=6\nOPTARG=new\n"
			if answer == Yes {
				want = "6\nOPTARG=pre\nOPTARG=new\n"
			}
			if out != want {
				t.Errorf("got %q, want %q", out, want)
			}
		})
	}
}

// The axis is asked only where the two answers would hand a child something
// different: under CoreSemantics, which answers it neither way, a `getopts`
// over an OPTARG nothing exported writes no refusal, and neither does one
// whose exported name operand it fills.
func TestAGetoptsOverUnexportedParametersAsksNothing(t *testing.T) {
	t.Parallel()
	out, st := run(
		t,
		`export o=1; getopts x: o -x6; echo $o $OPTARG; env | grep -E '^(o|OPTARG)='`,
		withSem(CoreSemantics()),
	)
	if st != 0 || out != "x 6\no=x\n" {
		t.Errorf("out=%q st=%d, want the values, o refreshed and no refusal", out, st)
	}
}

// The other stores the axis covers: the save, reset and restore of OPTIND a
// function call makes where the cursor is the call's own (#6497). Each row
// reads the child's copy and `$OPTIND` side by side, so a hold that also
// froze the parameter, or a reset that skipped the parameter, fails as surely
// as a refreshed copy does. The last two rows are the stores around a call
// that refresh under either answer — taking a prefix back, and a `local`'s
// restore — and they fail if the hold outlives them.
func TestAFunctionCallsOptindStoresLeaveTheEnvironmentCopy(t *testing.T) {
	t.Parallel()
	const child = "env | grep -E '^OPTIND=' || echo none; echo $OPTIND"
	for _, tc := range []struct{ name, src, held, refreshed string }{
		{"the reset on the way in", `export OPTIND=3; f() { ` + child + `; }; f`, "OPTIND=3\n1\n", "OPTIND=1\n1\n"},
		{"the restore on the way out", `export OPTIND=3; f() { OPTIND=7; }; f; ` + child, "OPTIND=7\n3\n", "OPTIND=3\n3\n"},
		{"a prefix's copy", `f() { ` + child + `; }; OPTIND=4 f`, "OPTIND=4\n1\n", "OPTIND=1\n1\n"},
		{"taking the prefix back refreshes", `export OPTIND=1; getopts x: o -x6; f() { :; }; OPTIND=4 f; ` + child, "OPTIND=2\n2\n", "OPTIND=2\n2\n"},
		{"a local's restore refreshes", `export OPTIND=1; g() { local OPTIND=5; }; f() { OPTIND=9; g; ` + child + `; }; f`, "OPTIND=1\n9\n", "OPTIND=9\n9\n"},
	} {
		for _, answer := range []Answer{Yes, No} {
			t.Run(tc.name+"/"+answer.String(), func(t *testing.T) {
				sem := permissive()
				sem.GetoptsLeavesTheEnvironmentCopyOfItsOwnParameters = answer
				sem.GetoptsFunctionPosition = GetoptsFunctionPositionIsLocal
				sem.GetoptsAssignmentRestartsWord = Yes
				out, _ := run(t, tc.src, withSem(sem))
				want := tc.refreshed
				if answer == Yes {
					want = tc.held
				}
				if out != want {
					t.Errorf("got %q, want %q", out, want)
				}
			})
		}
	}
}

// A call over an OPTIND nothing exported is handed its cursor and given the
// caller's back without the axis being asked: under CoreSemantics, which
// answers it neither way, the call writes no refusal.
func TestAFunctionCallOverAnUnexportedOptindAsksNothing(t *testing.T) {
	t.Parallel()
	sem := CoreSemantics()
	sem.GetoptsFunctionPosition = GetoptsFunctionPositionIsLocal
	sem.GetoptsAssignmentRestartsWord = Yes
	out, st := run(t, `OPTIND=3; f() { echo $OPTIND; OPTIND=7; }; f; echo $OPTIND`, withSem(sem))
	if st != 0 || out != "1\n3\n" {
		t.Errorf("out=%q st=%d, want the cursor reset and put back, and no refusal", out, st)
	}
}
