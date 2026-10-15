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

// Semantics.FdVariableReadonlyRefusedFirst is asked only where the name a
// `{name}` redirection stores into is frozen, so a vector that leaves it
// unanswered still opens, duplicates and stores every ordinary `{name}` in
// silence, and refuses by name the one input it is about (#6571).
func TestAnOrdinaryFdVariableAsksNothingAboutAFrozenName(t *testing.T) {
	d := syntax.Core()
	d.FdVariableRedirections = true
	for _, c := range []struct {
		name, src, want string
		refused         bool
	}{
		{"an open", `: {v}>/dev/null; [ -n "$v" ] && echo set`, "set\n", false},
		{"a duplication", `: {v}>&1; [ -n "$v" ] && echo set`, "set\n", false},
		{"an open that fails", `: {v}>/nodir/f; echo "st=$?"`, "st=1\n", false},
		{"a frozen name", `readonly v=1; : {v}>/dev/null; echo "st=$?"`, "st=2\n", true},
		{"a frozen name, duplicated", `readonly v=1; : {v}>&1; echo "st=$?"`, "st=2\n", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := syntax.Parse(c.src, d)
			if err != nil {
				t.Fatal(err)
			}
			sem := PosixSemantics()
			sem.RedirectErrorOnSpecialBuiltinFatal = No
			sem.FdVariableOutlivesTheCommand = Yes
			var out, errs strings.Builder
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: t.TempDir(),
				Stdout: &out, Stderr: &errs, Dialect: &d,
			})
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if out.String() != c.want {
				t.Errorf("out %q, want %q; stderr %q", out.String(), c.want, errs.String())
			}
			if got := strings.Contains(errs.String(), "aimed at a readonly name"); got != c.refused {
				t.Errorf("refused = %v, want %v; stderr %q", got, c.refused, errs.String())
			}
		})
	}
}
