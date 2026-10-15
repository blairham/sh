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

// Semantics.FdVariableBadName is asked only of a `{word}` whose word is not a
// name, so a vector that leaves it unanswered still runs every `{name}` in
// silence — names, elements, dotted names — and refuses by name the one input
// it is about (#6522).
func TestAnFdVariableBadNameAsksOnlyThere(t *testing.T) {
	d := syntax.Core()
	d.FdVariableRedirections = true
	d.FdVariableSubscript = true
	d.FdVariableAnyBraceWord = true
	d.DottedName = true
	for _, c := range []struct {
		name, src, want string
		refused         bool
	}{
		{"a name", `: {v}</dev/null; [ -n "$v" ] && echo set`, "set\n", false},
		{"an element", `a=(1 2); : {a[1]}</dev/null; [ "${a[1]}" != 2 ] && echo moved`, "moved\n", false},
		{"a dotted name", `: {.v}</dev/null; echo "st=$?"`, "st=0\n", false},
		{"a word that is not a name", `: {a-b}</dev/null; echo "st=$?"`, "st=2\n", true},
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
				t.Errorf("out %q, want %q", out.String(), c.want)
			}
			if got := strings.Contains(errs.String(), "whose word is not a name"); got != c.refused {
				t.Errorf("refused = %v, want %v; stderr %q", got, c.refused, errs.String())
			}
		})
	}
}

// The two answers by what they cost, through the core: a failed redirection
// that the next command survives, or a fatal error the next command never
// sees — except on a program, whose store is its own process's.
func TestAnFdVariableBadNameByPolicy(t *testing.T) {
	d := syntax.Core()
	d.FdVariableRedirections = true
	d.FdVariableAnyBraceWord = true
	for _, c := range []struct {
		name, src, want, wantErr string
		policy                   FdVariableBadNamePolicy
	}{
		{"fails the redirection", `echo {a-b}</dev/null; echo "st=$?"`, "st=1\n", "sh: a-b: invalid variable name\n", FdVariableBadNameFailsTheRedirection},
		{"fails before the open", `echo {a-b}</nonexist; echo "st=$?"`, "st=1\n", "sh: a-b: invalid variable name\n", FdVariableBadNameFailsTheRedirection},
		{"fatal at the store", `echo {a-b}</dev/null; echo "st=$?"`, "", "sh: a-b: invalid variable name\n", FdVariableBadNameIsAFatalStoreError},
		{"fatal, but on a program only the command", `cat {a-b}</dev/null </dev/null; echo "st=$?"`, "st=1\n", "sh: a-b: invalid variable name\n", FdVariableBadNameIsAFatalStoreError},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := syntax.Parse(c.src, d)
			if err != nil {
				t.Fatal(err)
			}
			sem := PosixSemantics()
			sem.RedirectErrorOnSpecialBuiltinFatal = No
			sem.FdVariableOutlivesTheCommand = Yes
			sem.FdVariableBadName = c.policy
			var out, errs strings.Builder
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: t.TempDir(),
				Stdout: &out, Stderr: &errs, Dialect: &d,
			}).SeedVars(map[string]string{"PATH": lookBinPath(t)})
			_, _ = r.Run(context.Background(), f)
			if out.String() != c.want || errs.String() != c.wantErr {
				t.Errorf("out %q err %q, want %q / %q", out.String(), errs.String(), c.want, c.wantErr)
			}
		})
	}
}
