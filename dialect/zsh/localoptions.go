// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// LOCAL_OPTIONS: options a function changes go back when it returns.
//
// # What was measured
//
// Every row below is a probe against zsh 5.9.2, and together they say the
// mechanism is one rule rather than the four or five it first looks like:
//
//  1. Every function call saves the option table as its body starts.
//     Measured: `f() { setopt extendedglob; setopt localoptions }` restores
//     extendedglob, even though it moved *before* the line that asked for the
//     scoping. So the save cannot wait for `setopt localoptions` to run.
//  2. On return, the whole table goes back if `localoptions` is on **at that
//     moment**. Measured both ways: `f() { setopt localoptions; setopt
//     extendedglob; unsetopt localoptions }` leaks the glob option, and with
//     `localoptions` already on globally a function that never mentions it
//     still restores everything it moved.
//  3. `localoptions` itself comes back whatever else does not. Measured, and
//     it is the one row that cannot be explained by rule 2: with the option
//     on globally, `f() { unsetopt localoptions }` leaves it *on* afterwards,
//     though at the return it was off and nothing else was restored.
//  4. A nested call is not special. Each call has a save of its own and asks
//     the same question at its own return, so an inner function with no
//     `setopt` of its own still restores while the outer one runs, as long as
//     `localoptions` is on when it returns.
//  5. `emulate -L` is this and nothing else. Measured: inside `emulate -L
//     zsh` the option reads on, and at the top level — where there is no call
//     to return from — `emulate -L zsh` leaves it on globally, so the *next*
//     function call localises. See emulate.go, where the letter is now one
//     line.
//
// Rule 5 is why this file exists rather than a second mechanism beside the
// one `emulate -L` already had. That one saved at its own line and restored
// unconditionally, which is rules 1 and 2 both slightly wrong; folding the
// two fixed `f() { setopt extendedglob; emulate -L zsh }` on the way past.
//
// What is *not* localised: traps and patterns have options of their own —
// `localtraps` and `localpatterns` — and measured, `setopt localoptions`
// leaves a trap set in the function installed. The emulation mode travels
// with the table, because a plain `emulate` resets every option and so turns
// `localoptions` off, which is what makes rule 2 answer "no" for it.

// optionBits is one bit per entry in zshOptions, which is what a saved table
// is: a value, so a copy is a snapshot, and small enough that taking one at
// every function call costs nothing worth measuring. TestOptionBitsHoldTheTable
// fails if the table outgrows it.
type optionBits [4]uint64

func (b *optionBits) set(i int) { b[i/64] |= 1 << uint(i%64) }

func (b optionBits) on(i int) bool { return b[i/64]&(1<<uint(i%64)) != 0 }

// optionState is this dialect's whole option namespace at a moment: the
// semantics vector three of the names are, the emulation mode, the store the
// recorded names share, and one bit for each of the rest.
//
// Nothing here is a live pointer into the runner's option state. The vector
// is swapped copy-on-write and the store is rebuilt rather than sorted in
// place — see swapAxes and setRecordedDeviation — so holding either as it
// stands is holding what it *was*, and neither has to be copied to be saved.
type optionState struct {
	sem      *interp.Semantics
	mode     string
	recorded []string
	on       optionBits
	// localOptions is where `localoptions` itself stood, which rule 3 needs
	// separately: it goes back even on the return that restores nothing else.
	localOptions bool
}

// localOptionsIndex is where `localoptions` sits in the table, resolved once.
var localOptionsIndex = zshOptionIndex["localoptions"]

func localOptionsOn(r *interp.Runner) bool { return zshOptions[localOptionsIndex].get(r) }

func setLocalOptions(r *interp.Runner, on bool) {
	_ = zshOptions[localOptionsIndex].set(r, on)
}

// saveOptionState takes the table as it stands.
//
// The three axis-backed names are read like the rest and put back with the
// vector rather than one at a time — see restore — but their bits are still
// recorded, because the loop that reads them is the same loop and skipping
// them would only buy a branch.
func saveOptionState(r *interp.Runner) optionState {
	s := optionState{sem: r.Semantics, mode: currentEmulation(r)}
	// The store held as it stands rather than copied: see the type's comment.
	s.recorded, _ = r.GetArray(zshRecordedStore)
	for i := range zshOptions {
		o := &zshOptions[i]
		if o.set == nil || o.recorded {
			continue
		}
		if o.get(r) {
			s.on.set(i)
		}
	}
	s.localOptions = localOptionsOn(r)
	return s
}

// restore puts the whole table back.
func (s optionState) restore(r *interp.Runner) {
	r.Semantics = s.sem
	setRecordedOptions(r, s.recorded)
	for i := range zshOptions {
		o := &zshOptions[i]
		if o.set == nil || o.recorded {
			continue
		}
		switch o.base {
		case "shwordsplit", "nomatch", "ksharrays":
			// The vector restore above has these, and re-setting them would
			// swap in a fresh copy for nothing.
			continue
		}
		if want := s.on.on(i); o.get(r) != want {
			_ = o.set(r, want)
		}
	}
	r.SetVar(emulationMode, s.mode)
}

// restoreIfLocal is what a function return does with the table its own entry
// saved: rule 2, and rule 3 for the return that answers it "no".
//
// The second branch writes only when there is something to write. A return
// that restores nothing is the overwhelmingly common one, and the store is
// rebuilt on every write, so asking for a state it already holds would put an
// allocation on the way out of every function call in the shell.
func (s optionState) restoreIfLocal(r *interp.Runner) {
	if localOptionsOn(r) {
		s.restore(r)
		return
	}
	if s.localOptions {
		setLocalOptions(r, true)
	}
}

// registerLocalOptions hangs the save on every function call this runner
// makes. Once per shell; the moment is the substrate's and the table is ours.
func registerLocalOptions(r *interp.Runner) {
	r.AtEveryFunctionCall(func(r *interp.Runner) func() {
		saved := saveOptionState(r)
		return func() { saved.restoreIfLocal(r) }
	})
}
