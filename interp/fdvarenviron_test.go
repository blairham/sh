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

// Semantics.FdVariableReachesTheProgramsEnvironment by value, on both sides,
// and the property that keeps it off the common path: asked only where a
// program's own `{name}` store changed what its environment would say about
// that name (#6521).
func TestAnFdVariableInAProgramsEnvironment(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		reaches         Answer
		refused         bool
	}{
		{"yes: the store is there", `export v=keep; env {v}</dev/null | grep -c '^v=keep'`, "0\n", Yes, false},
		{"no: the value from before", `export v=keep; env {v}</dev/null | grep '^v='`, "v=keep\n", No, false},
		{"no: a name that was not set", `set -a; env {v}</dev/null | grep -c '^v='`, "0\n", No, false},
		{"no: one name of two", `export w=w; v=0; env {v}</dev/null {w}</dev/null | grep -c '^w=w$'`, "1\n", No, false},
		{"no: the shell keeps its own after", `export v=keep; env {v}</dev/null >/dev/null; echo "[$v]"`, "[keep]\n", No, false},
		// Nothing to ask: no name stored is exported, before or after.
		{"unanswered: a name nobody exports", `v=keep; env {v}</dev/null | grep -c '^v='`, "0\n", Unspecified, false},
		{"unanswered: no {name}", `export v=keep; env 3</dev/null | grep '^v='`, "v=keep\n", Unspecified, false},
		{"unanswered: a builtin's store", `export v=keep; : {v}</dev/null; [ "$v" != keep ] && echo moved`, "moved\n", Unspecified, false},
		{"unanswered: the program does not run", `export v=keep; env {v}</dev/null; echo "st=$?"`, "st=2\n", Unspecified, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := syntax.Parse(c.src, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			sem := PosixSemantics()
			sem.RedirectErrorOnSpecialBuiltinFatal = No
			sem.FdVariableOutlivesTheCommand = Yes
			sem.AllexportMarksALoopOrGetoptsName = Yes
			sem.FdVariableReachesTheProgramsEnvironment = c.reaches
			var out, errs strings.Builder
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: t.TempDir(),
				Stdout: &out, Stderr: &errs,
			}).SeedVars(map[string]string{"PATH": lookBinPath(t)})
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if out.String() != c.want {
				t.Errorf("out %q, want %q", out.String(), c.want)
			}
			if got := strings.Contains(errs.String(), "reaching the program's environment"); got != c.refused {
				t.Errorf("refused = %v, want %v; stderr %q", got, c.refused, errs.String())
			}
		})
	}
}
