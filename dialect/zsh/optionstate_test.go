// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"io"
	"testing"

	"github.com/blairham/sh/interp"
)

func optionStateRunner(t *testing.T) *interp.Runner {
	t.Helper()
	sem, diag, dl := Semantics(), Diagnostics(), Dialect()
	r := &interp.Runner{
		Stdout: io.Discard, Stderr: io.Discard,
		Semantics: &sem, Diagnostics: &diag, Dialect: &dl,
		Dir: t.TempDir(), Name: "zsh",
	}
	Apply(r)
	return r
}

// The saved table is a fixed-size bitset, so the table must fit in it. A
// check rather than a convention, for the reason clonetables.go gives about
// its own: an option added past the end would be saved into nothing, restore
// as false, and nothing else in the tree would say so.
func TestOptionBitsHoldTheTable(t *testing.T) {
	var b optionBits
	if have := len(b) * 64; len(zshOptions) > have {
		t.Fatalf("%d options, %d bits — widen optionBits", len(zshOptions), have)
	}
}

// Every option that moves at all is put back by a saved table.
//
// This is the completeness guard on optionState, and it is written against
// the option table rather than against a list kept beside it: a name added
// with new state behind it fails here until the snapshot can reach that
// state. Testing the restore one option at a time is what makes a miss
// visible — a whole-table probe would pass while a single name leaked.
func TestASavedTableRestoresEveryOptionThatMoves(t *testing.T) {
	moved := 0
	for i := range zshOptions {
		o := &zshOptions[i]
		if o.set == nil {
			continue
		}
		if o.base == "exec" {
			// This dialect's name for `set -n` read from the other end, and
			// that switch is one-way in every shell of the panel and in the
			// substrate with it: turning execution back on is ignored, and
			// with it off the command that would ask never runs. Nothing can
			// restore it and no measurement asks for it.
			continue
		}
		r := optionStateRunner(t)
		was := o.get(r)
		saved := saveOptionState(r)
		_ = o.set(r, !was)
		if o.get(r) == was {
			// A name this shell recognizes and will not move. It has
			// nothing to restore, and it is not what this is guarding.
			continue
		}
		moved++
		saved.restore(r)
		if got := o.get(r); got != was {
			t.Errorf("%s: %v after the saved table was restored, want %v", o.base, got, was)
		}
	}
	// And enough of them really moved, so that a change making the whole
	// table immovable cannot pass this by having nothing left to check.
	if want := 165; moved < want {
		t.Errorf("only %d options moved, want at least %d", moved, want)
	}
}

// A saved table also carries the emulation mode, which is what makes
// `emulate -L` one mechanism with `setopt localoptions` rather than two.
func TestASavedTableCarriesTheEmulationMode(t *testing.T) {
	r := optionStateRunner(t)
	saved := saveOptionState(r)
	applyEmulation(r, "sh", false)
	if got := currentEmulation(r); got != "sh" {
		t.Fatalf("mode %q after emulate sh, want sh", got)
	}
	saved.restore(r)
	if got := currentEmulation(r); got != "zsh" {
		t.Errorf("mode %q after the restore, want zsh", got)
	}
}
