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
	for _, answer := range []Answer{Yes, No} {
		t.Run(answer.String(), func(t *testing.T) {
			sem := permissive()
			sem.GetoptsLeavesTheEnvironmentCopyOfItsOwnParameters = answer
			out, _ := run(t, `getopts x: o -x6; echo $OPTARG; env | grep -E '^OPTARG='; OPTARG=new; env | grep -E '^OPTARG='`, func(r *Runner) {
				r.Semantics = &sem
				r.Env = append(r.Env, "OPTARG=pre")
			})
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
	out, st := run(t, `export o=1; getopts x: o -x6; echo $o $OPTARG; env | grep -E '^(o|OPTARG)='`, withSem(CoreSemantics()))
	if st != 0 || out != "x 6\no=x\n" {
		t.Errorf("out=%q st=%d, want the values, o refreshed and no refusal", out, st)
	}
}
