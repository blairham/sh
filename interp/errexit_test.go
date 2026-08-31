// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
)

// `set -e` is core behavior: dash, bash, ksh93 and zsh agree on all
// twenty-four probes measured, including the ones where implementations
// usually diverge. There is no axis here, which was not the expectation.
func TestErrExit(t *testing.T) {
	for _, tc := range []struct {
		name, src  string
		wantOut    string
		wantStatus int
	}{
		{"a failure ends the script", `set -e; false; echo reached`, "", 1},
		// The helper merges the two streams, so the diagnostic is part of
		// the expected output; what matters is that `reached` is not.
		{"and it is the failing status", `set -e; sh_not_a_command_xyz; echo reached`, "sh: sh_not_a_command_xyz: not found\n", 127},
		{"off by default", `false; echo reached`, "reached\n", 0},
		{"set +e turns it back off", `set -e; set +e; false; echo reached`, "reached\n", 0},

		// The exemptions: a status being *tested* is not a failure.
		{"if condition", `set -e; if false; then :; fi; echo reached`, "reached\n", 0},
		{"while condition", `set -e; while false; do :; done; echo reached`, "reached\n", 0},
		{"until condition", `set -e; until true; do :; done; echo reached`, "reached\n", 0},
		{"non-final operand of &&", `set -e; false && :; echo reached`, "reached\n", 0},
		{"left operand of ||", `set -e; false || :; echo reached`, "reached\n", 0},
		{"negation", `set -e; ! true; echo reached`, "reached\n", 0},
		{"negation at the end of a chain", `set -e; true && ! true; echo reached`, "reached\n", 0},

		// And where it does still fire.
		{"final operand of &&", `set -e; : && false; echo reached`, "", 1},
		{"final operand of ||", `set -e; false || false; echo reached`, "", 1},
		{"last element of a pipeline", `set -e; true | false; echo reached`, "", 1},
		{"but not an earlier element", `set -e; false | true; echo reached`, "reached\n", 0},
		{"a subshell", `set -e; (false); echo reached`, "", 1},
		{"a group", `set -e; { false; }; echo reached`, "", 1},
		{"a loop body", `set -e; for i in 1; do false; done; echo reached`, "", 1},
		{"a case body", `set -e; case a in a) false;; esac; echo reached`, "", 1},

		// Functions.
		{"a function called plainly", `set -e; f() { false; echo inner; }; f; echo reached`, "", 1},

		// The subtle one: the exemption is inherited, so the function keeps
		// going past its own failure, and so does anything it calls.
		{
			"the exemption reaches into a function",
			`set -e; f() { false; echo inner; }; if f; then :; fi; echo reached`,
			"inner\nreached\n", 0,
		},
		{
			"and all the way down",
			`set -e; g() { false; echo g; }; f() { g; echo f; }; if f; then :; fi; echo reached`,
			"g\nf\nreached\n", 0,
		},

		// An assignment reports what its substitution reported.
		{"assignment takes the substitution", `set -e; x=$(false); echo reached`, "", 1},
		{"but a used substitution does not", `set -e; echo "$(false)"; echo reached`, "\nreached\n", 0},
		{"a plain assignment succeeds", `set -e; x=1; echo reached`, "reached\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, st := run(t, tc.src, withSem(bash.Semantics()))
			if got != tc.wantOut {
				t.Errorf("output = %q, want %q", got, tc.wantOut)
			}
			if st != tc.wantStatus {
				t.Errorf("status = %d, want %d", st, tc.wantStatus)
			}
		})
	}
}
