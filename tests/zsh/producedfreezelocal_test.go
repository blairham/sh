// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// A function's local declaration refused a value over a frozen parameter the
// shell produces ends a `-c` string at 0, not at the fatal status — and the
// same lines from a script file end at 1. Measured 2026-10-03 on zsh 5.9.2;
// see Semantics.ProducedFreezeUnderALocalEndsAtZero.
func TestALocalRefusedOverAProducedFreezeEndsAtZero(t *testing.T) {
	run := func(t *testing.T, route interp.Route, src string) (string, int) {
		t.Helper()
		out, st, err := preset.Combined(t, dialecttest.Base{
			Name: "zsh", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"}, Route: route,
		}, src)
		if err != nil {
			t.Fatalf("run %q: %v", src, err)
		}
		return out, st
	}
	for _, tc := range []struct {
		src    string
		status int
	}{
		{"f() { local PPID=5; }; f; echo after", 0},
		{"f() { false; local -i ARGC=5; }; f", 0},
		{"f() { readonly PPID=5; }; f", 0},
		// The controls: no local, a freeze the script made, and a bare
		// assignment after a valueless local are all the ordinary 1.
		{"f() { export PPID=5; }; f", 1},
		{"f() { local -r q=1; local q=2; }; f", 1},
		{"f() { typeset ARGC; ARGC=3; }; f", 1},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := run(t, interp.RouteCommandString, tc.src)
			if st != tc.status || !strings.Contains(out, "read-only variable") || strings.Contains(out, "after") {
				t.Errorf("-c: = %q at %d, want the refusal alone at %d", out, st, tc.status)
			}
			if _, st := run(t, interp.RouteScriptFile, tc.src); st != 1 {
				t.Errorf("a script file: status %d, want 1", st)
			}
		})
	}
}
