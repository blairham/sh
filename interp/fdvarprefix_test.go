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

// Semantics.FdVariableStoredAfterThePrefix is asked only where a prefix
// entry names a name a `{name}` on the same command stored into, so a vector
// that leaves it unanswered still runs a prefix, a `{name}`, and the two over
// different names in silence (#6569).
func TestAPrefixAndAnFdVariableAskOnlyWhereTheyMeet(t *testing.T) {
	d := syntax.Core()
	d.FdVariableRedirections = true
	for _, c := range []struct {
		name, src string
		asked     bool
	}{
		{"a prefix alone", `f() { :; }; v=pre f`, false},
		{"a {name} alone", `f() { :; }; f {v}</dev/null`, false},
		{"over different names", `f() { :; }; v=pre f {w}</dev/null`, false},
		{"over the same name", `f() { :; }; v=pre f {v}</dev/null`, true},
		{"before a builtin", `v=pre true {v}</dev/null`, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := syntax.Parse(c.src, d)
			if err != nil {
				t.Fatal(err)
			}
			sem := PosixSemantics()
			sem.FdVariableOutlivesTheCommand = Yes
			var out, errs strings.Builder
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: t.TempDir(),
				Stdout: &out, Stderr: &errs, Dialect: &d,
			})
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(errs.String(), "seen through the command's prefix"); got != c.asked {
				t.Errorf("asked = %v, want %v; stderr %q", got, c.asked, errs.String())
			}
		})
	}
}
