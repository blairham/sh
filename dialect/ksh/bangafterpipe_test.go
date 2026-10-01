// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// A `!` after a bar is refused here too, and that is a decision rather than a
// measurement of this dialect's shell (#5256). ksh93u+ takes it and toggles the
// whole pipeline's negation — `echo | ! true` is 1 and `! true | ! true` is 0 —
// which is parked as #5272. Before the refusal this dialect ran the `!` as a
// command named `!`, which matched no shell at all; the refusal matches three.
// The row is here so that the day #5272 is taken up, this is the line that
// says so.
func TestABangAfterABarIsRefusedEvenHere(t *testing.T) {
	for _, src := range []string{"echo | ! true", "! true | ! true", "echo | ! true | cat"} {
		_, err := syntax.Parse(src, ksh.Dialect())
		e, ok := err.(*syntax.Error)
		if !ok || e.Kind != syntax.ErrUnexpected {
			t.Errorf("%q: err = %v, want an unexpected-token refusal", src, err)
		}
	}
	// The control: `|&` is the coprocess operator in this dialect and not a
	// bar, so a `!` after it begins a pipeline of its own — measured,
	// `echo |& ! true` is 1 in ksh93u+ and here.
	if _, err := syntax.Parse("echo |& ! true", ksh.Dialect()); err != nil {
		t.Errorf("`echo |& ! true`: %v, want it to parse", err)
	}
}
