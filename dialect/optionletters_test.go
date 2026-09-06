// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// A letter a dialect's optstring claims must never also appear in its
// Diagnostics.UnimplementedOptionLetters, because the second entry can never
// be reached: the shared reader consults that map only from refuseOption,
// which runs for a letter the optstring did *not* claim. An entry on both
// lists is dead data that says the opposite of what the shell does.
//
// It is an invariant rather than a per-letter assertion because that is what
// makes it worth having: it caught nothing when it was written and it fails
// the moment a letter is implemented and its old "not implemented yet" line
// is left behind — which is exactly what happened to `read -i` (#761), and
// which no behavioral test can see, the unreachable branch being unreachable.
func TestNoLetterIsBothImplementedAndNot(t *testing.T) {
	for _, d := range []struct {
		name string
		sem  interp.Semantics
		diag interp.Diagnostics
	}{
		{"bash", bash.Semantics(), bash.Diagnostics()},
		{"dash", dash.Semantics(), dash.Diagnostics()},
		{"ksh", ksh.Semantics(), ksh.Diagnostics()},
		{"zsh", zsh.Semantics(), zsh.Diagnostics()},
	} {
		t.Run(d.name, func(t *testing.T) {
			// The builtins whose letters are a Semantics axis rather than a
			// constant; those are the only ones with two lists to disagree.
			for _, b := range []struct{ builtin, opts string }{
				{"read", d.sem.ReadOptions},
				{"echo", d.sem.EchoOptions},
			} {
				claimed := strings.ReplaceAll(b.opts, ":", "")
				missing := d.diag.UnimplementedOptionLetters[b.builtin]
				for i := 0; i < len(missing); i++ {
					if strings.IndexByte(claimed, missing[i]) >= 0 {
						t.Errorf("%s: -%c is in ReadOptions/EchoOptions %q and in UnimplementedOptionLetters %q; "+
							"the second can never be reached", b.builtin, missing[i], b.opts, missing)
					}
				}
			}
		})
	}
}
