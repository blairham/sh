// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.SpecialBuiltinNameIsNotAFunctionName: a definition whose name is
// one of the special builtins, refused where the dialect refuses one.
//
// The stage is the whole of why this is here and not on the grammar vector:
// the check happens where the definition **runs**, so a script may enter the
// state from inside the same input. The rows below are asked by the axis and
// never by a shell's name; the axis's own documentation says where they were
// taken.

func refusingSpecialFunctionNames(on Answer) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.SpecialBuiltinNameIsNotAFunctionName = on
		r.Semantics = &s
		d := Diagnostics{FunctionNameIsASpecialBuiltin: "%[1]s: is special"}
		r.Diagnostics = &d
	}
}

func TestAFunctionNamedAfterASpecialBuiltinIsRefusedWhereTheDialectSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name    string
		refused bool
	}{
		// The roster, which is the one this shell already holds for every
		// other consequence of specialness rather than a table of its own.
		{"export", true},
		{"readonly", true},
		{"eval", true},
		{"exec", true},
		{"exit", true},
		{"return", true},
		{"set", true},
		{"shift", true},
		{"times", true},
		{"trap", true},
		{"unset", true},
		{"break", true},
		{"continue", true},
		{":", true},
		{".", true},
		{"source", true},
		// And the names beside it that are builtins and not special ones.
		{"local", false},
		{"true", false},
		{"read", false},
		{"cd", false},
		{"echo", false},
		{"alias", false},
		{"pwd", false},
	} {
		src := tc.name + `() { printf fn; }` + "\n" + `printf 'after'`
		for _, c := range []struct {
			on   Answer
			want bool
		}{{Yes, tc.refused}, {No, false}} {
			out, st := run(t, src, refusingSpecialFunctionNames(c.on))
			if !c.want {
				if out != "after" || st != 0 {
					t.Errorf("%v: %s defining: got %q (status %d), want %q at 0",
						c.on, tc.name, out, st, "after")
				}
				continue
			}
			if !strings.Contains(out, tc.name+": is special") {
				t.Errorf("%s = %q, want the refusal naming it", tc.name, out)
			}
			if strings.Contains(out, "after") {
				t.Errorf("%s: the script was not ended — %q", tc.name, out)
			}
			if st != 2 {
				t.Errorf("%s: status %d, want 2", tc.name, st)
			}
		}
	}
}

// The stage. The commands in front of the definition run, and a definition
// the script never reaches is never refused — which is what says the check is
// at the definition and not at the parse.
func TestTheSpecialBuiltinNameIsCheckedWhereTheDefinitionRuns(t *testing.T) {
	out, st := run(t, "printf 'a'\nexport() { :; }\nprintf 'b'",
		refusingSpecialFunctionNames(Yes))
	if !strings.HasPrefix(out, "a") {
		t.Errorf("got %q, want the command in front of the definition to have run", out)
	}
	if strings.Contains(out, "b") || st != 2 {
		t.Errorf("got %q (status %d), want the script ended at 2", out, st)
	}
	out, st = run(t, "if false; then export() { :; }; fi\nprintf 'after'",
		refusingSpecialFunctionNames(Yes))
	if out != "after" || st != 0 {
		t.Errorf("a branch never taken: got %q (status %d), want %q at 0", out, st, "after")
	}
}
