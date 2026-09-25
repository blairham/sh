// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "testing"

// Placing an option where it already stands must not replace the semantics
// vector.
//
// This is a performance property written as a correctness test, because it is
// invisible in behavior and expensive in fact. The vector is 962 axes and 3280
// bytes, swapped copy-on-write, so a copy is a heap allocation; and a zsh
// function idiomatically opens with `emulate -L zsh`, which walks this table
// and sets every name the emulation has a default for, whether or not it
// differs from what the shell already held. Measured on a real ~/.zshrc, one
// startup made 35,597 swap calls of which 30,026 changed nothing — 98MB of
// copying a struct onto an identical one, and about a fifth of the startup.
//
// Writing a call site back as an unconditional swapAxes brings that straight
// back and nothing else in the tree would notice, which is why this walks the
// whole table rather than naming the sites that were fixed. See setAxis.
func TestPlacingAnOptionWhereItStandsDoesNotSwapTheVector(t *testing.T) {
	// The control, and the reason this test is not a decoration: an option
	// that moves no axis passes the first half whatever setAxis does, so a
	// run where *nothing* moved the vector would be 200-odd vacuous rows
	// reporting success. The count is what distinguishes them.
	moved := 0
	for i := range zshOptions {
		o := &zshOptions[i]
		if o.set == nil {
			continue
		}
		if o.base == "exec" {
			// One-way, for the reason optionstate_test.go gives: with
			// execution off nothing runs to turn it back on.
			continue
		}
		r := optionStateRunner(t)
		was := o.get(r)
		before := r.Semantics
		if code := o.set(r, was); code != 0 {
			t.Errorf("setopt %s to where it stands: status %d, want 0", o.base, code)
		}
		if r.Semantics != before {
			t.Errorf("setopt %s to where it stands replaced the semantics vector", o.base)
		}
		if o.set(r, !was); r.Semantics != before {
			moved++
		}
	}
	if moved == 0 {
		t.Fatal("no option in the table moved the vector at all: this test proves nothing")
	}
	t.Logf("%d of %d options are axis-backed and vouch for the row above", moved, len(zshOptions))
}

// An emulation asked for the emulation already in force must not replace the
// vector either. This is the shape the real cost had: `emulate -L zsh` at the
// top of a zsh function, in a shell that is already zsh, 2717 times in one
// startup.
func TestAnEmulationAlreadyInForceDoesNotSwapTheVector(t *testing.T) {
	for _, strict := range []bool{false, true} {
		r := optionStateRunner(t)
		applyEmulation(r, "zsh", strict)
		before := r.Semantics
		applyEmulation(r, "zsh", strict)
		if r.Semantics != before {
			t.Errorf("emulate zsh strict=%v twice replaced the semantics vector", strict)
		}
		// The control: a different emulation has to move it, or the row
		// above is measuring a shell that cannot change its mind.
		if applyEmulation(r, "sh", strict); r.Semantics == before {
			t.Errorf("emulate sh strict=%v did not move the vector at all", strict)
		}
	}
}
