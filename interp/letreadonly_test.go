// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `let` writing a frozen name reports and runs the next line, whatever the
// two fatality axes around it say. It is core rather than an axis because the
// panel is unanimous: every column that has the builtin carries on — and the
// two axes it sits between each have a column that would have ended the
// script here.
//
// `ReadonlyRefusalInABuiltinIsFatal` is the builtin's question and one
// dialect answers it Yes; `ArithCommandErrorIsFatal` is `(( ))`'s and a
// different dialect answers that Yes. `let` takes neither, so both are moved
// to their fatal value here and the line after it still runs (#3470).
func TestALetRefusedByAFreezeEndsNoScript(t *testing.T) {
	for _, name := range []string{
		"a builtin's refusal is fatal", "an arithmetic command's failure is fatal", "both",
	} {
		t.Run(name, func(t *testing.T) {
			sem := PosixSemantics()
			if name != "an arithmetic command's failure is fatal" {
				sem.ReadonlyRefusalInABuiltinIsFatal = Yes
			}
			if name != "a builtin's refusal is fatal" {
				sem.ArithCommandErrorIsFatal = Yes
			}
			out, st := run(t, "readonly x=1\nlet x=2\necho after $?\n",
				func(r *Runner) { r.Semantics = &sem })
			if !strings.Contains(out, "after 1") {
				t.Errorf("got %q at %d, want the next line to run and report 1", out, st)
			}
		})
	}
}
