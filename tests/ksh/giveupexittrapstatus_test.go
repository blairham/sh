// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// An EXIT trap that runs because the shell gave up over an error it reported
// reads the `$?` from before the failing command, while the caller of the
// `function` scope — or the shell — still gets the error's status (#6873).
//
// Measured 2026-10-27 on ksh93u+ 2012-08-01 over script files; `(exit 5)`
// sets the earlier status, and the rows that read 0 are the ones that would
// read the same as the error's 1 if `false` set it, which is why they use a
// different number. A bare `exit` in the scope's trap ends the shell at the
// status the trap read, where this shell let the call catch it and ran on.
func TestAnExitTrapAfterAGiveUpReadsTheEarlierStatus(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a division by zero", "function f { trap 'echo ft=$?' EXIT; (exit 5); echo $((1/0)); }\nf; echo after $?\n", "ft=5\nafter 1\n"},
		{"the trap command's own zero", "function f { trap 'echo ft=$?' EXIT; echo $((1/0)); }\nf; echo after $?\n", "ft=0\nafter 1\n"},
		{"a word error", "function f { trap 'echo ft=$?' EXIT; (exit 7); : ${q?b}; }\nf; echo after $?\n", "ft=7\nafter 1\n"},
		{"a readonly reassignment", "function f { trap 'echo ft=$?' EXIT; (exit 5); readonly r=1; r=2; }\nf; echo after $?\n", "ft=0\nafter 1\n"},
		{"an unset name", "function f { trap 'echo ft=$?' EXIT; (exit 5); set -u; : $nosuch; }\nf; echo after $?\n", "ft=0\nafter 1\n"},
		{"a usage error keeps its own status for the caller", "function f { trap 'echo ft=$?' EXIT; (exit 5); set -o nosuch; }\nf; echo after $?\n", "ft=5\nafter 2\n"},
		{"a substitution that will not parse", "function f { trap 'echo ft=$?' EXIT; (exit 5); v=`echo hi; for`; }\nf; echo after $?\n", "ft=5\nafter 3\n"},
		{"the command before it in a list", "function f { trap 'echo ft=$?' EXIT; (exit 5); true && echo $((1/0)); }\nf; echo after $?\n", "ft=0\nafter 1\n"},
		{"the command before it in a compound", "function f { trap 'echo ft=$?' EXIT; (exit 5); if true; then echo $((1/0)); fi; }\nf; echo after $?\n", "ft=0\nafter 1\n"},
		{"a scope that caught it arrives as its status", "function f { trap 'echo ft=$?' EXIT; (exit 5); g; }\nfunction g { (exit 6); set -o nosuch; }\nf; echo after $?\n", "ft=2\nafter 2\n"},
		{"a () call is not a scope", "function f { trap 'echo ft=$?' EXIT; (exit 5); g; }\ng() { (exit 6); echo $((1/0)); }\nf; echo after $?\n", "ft=6\nafter 1\n"},
		{"the shell's own trap", "trap 'echo st=$?' EXIT; (exit 5); echo $((1/0)); echo after\n", "st=5\n"},
		{"a return is not a give-up, the control", "function f { trap 'echo ft=$?' EXIT; return 3; }\nf; echo after $?\n", "ft=3\nafter 3\n"},
		{"errexit is not a give-up", "function f { trap 'echo ft=$?' EXIT; (exit 5); set -e; false; }\nf; echo after $?\n", "ft=1\n"},
		{"a bare exit in the scope's trap ends the shell", "function f { trap 'exit' EXIT; (exit 5); echo $((1/0)); }\nf; echo after $?\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := preset.Parse(t, tc.src)
			var out, errs strings.Builder
			r := preset.Runner(dialecttest.Base{
				Name: "ksh", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
				Stdout: &out, Stderr: &errs,
			})
			r.Route = interp.RouteScriptFile
			st, err := r.Run(context.Background(), f)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if out.String() != tc.want {
				t.Errorf("wrote %q (%q), want %q", out.String(), errs.String(), tc.want)
			}
			if tc.name == "a bare exit in the scope's trap ends the shell" && st != 5 {
				t.Errorf("ended at %d, want 5", st)
			}
			if tc.name == "the shell's own trap" && st != 1 {
				t.Errorf("ended at %d, want 1", st)
			}
		})
	}
}
