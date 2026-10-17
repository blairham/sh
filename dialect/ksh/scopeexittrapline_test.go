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

// A scope's EXIT trap is numbered on from the line the scope ended at (#6720).
//
// Measured 2026-10-16 on ksh93u+ 2012-08-01 over script files. A subshell this
// shell runs in its own process — `( … )` alone, as a pipeline's last element,
// nested, or `$( … )` — counts its trap from the last line it ran, its last
// command or its `exit`; one it forks — a pipeline element before the last,
// or a background job — counts from 1, as the shell's own trap does. A
// `function` scope's trap runs once the call is left, in the caller's frame
// and at the call's line, so a body line after the first is the caller's line
// plus it, and `$1` is the caller's. `f()` sets the shell's trap, which is the
// control: line 1.
func TestAScopesExitTrapIsNumberedFromWhereTheScopeEnded(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a subshell that exits", ":\n(trap \"echo L=\\$LINENO\" EXIT\n:\nexit 0\n:\n)\n", "L=4\n"},
		{"a subshell that runs off its end", ":\n(:\ntrap \":\nnosuch_t\necho L=\\$LINENO\" EXIT\n:\n:\n)\n:\n", "line 8: nosuch_t: not found\nL=9\n"},
		{"a pipeline's last element", ":\n: | (trap \"echo L=\\$LINENO\" EXIT; :\n:)\n", "L=3\n"},
		{"a nested subshell", ":\n( (trap \"echo L=\\$LINENO\" EXIT; :\n:) )\n", "L=3\n"},
		{"a pipeline element it forks", ":\n(trap \"echo L=\\$LINENO\" EXIT; :\n:) | cat\n", "L=1\n"},
		{"a background job", ":\n(trap \"echo L=\\$LINENO\" EXIT; :\n:) &\nwait\n", "L=1\n"},
		{"a function scope", "function f {\ntrap \":\nnosuch_t\necho L=\\$LINENO\" EXIT\n:\n:\n}\n:\nf\n", "line 10: nosuch_t: not found\nL=11\n"},
		{"a function scope runs in the caller's frame", "function f { typeset v=in; trap \"echo v=\\$v a=\\$1\" EXIT; }\nv=out; set -- top; f arg\n", "v=out a=top\n"},
		{"the shell's own trap", "f() {\ntrap \"echo L=\\$LINENO\" EXIT\n:\n}\n:\nf\n:\n", "L=1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := preset.Parse(t, tc.src)
			var buf strings.Builder
			r := preset.Runner(dialecttest.Base{
				Name: "ksh", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
				Stdout: &buf, Stderr: &buf,
			})
			r.Route = interp.RouteScriptFile
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatalf("run: %v", err)
			}
			if !strings.HasSuffix(buf.String(), tc.want) {
				t.Errorf("wrote %q, want it to end %q", buf.String(), tc.want)
			}
		})
	}
}
