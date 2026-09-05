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

// Whether `bg` in a shell with no job control refuses before reading its
// operand. Half the panel says "no job control" to anything at all; the
// other half reads the operand first and complains about that instead.
func TestBgMayReportAbsentJobControlFirst(t *testing.T) {
	for _, c := range []struct {
		name       string
		first      Answer
		wantStderr string
	}{
		{"refused before the operand", Yes, "no job control"},
		{"the operand is read first", No, "--"},
	} {
		t.Run(c.name, func(t *testing.T) {
			sem := PosixSemantics()
			sem.JobControlAbsenceIsReportedFirst = c.first
			errOut := &strings.Builder{}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
				Stdout: &strings.Builder{}, Stderr: errOut,
			})
			f, err := syntax.Parse("bg --version\n", syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if got := errOut.String(); !strings.Contains(got, c.wantStderr) {
				t.Errorf("stderr %q, want it to mention %q", got, c.wantStderr)
			}
		})
	}
}
