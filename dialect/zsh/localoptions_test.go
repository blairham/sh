// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// LOCAL_OPTIONS, measured row by row against zsh 5.9.2. See
// dialect/zsh/localoptions.go for the rule the rows add up to.
//
// `extendedglob` is the probe throughout because it is off by default here
// and in the shell being measured, so a function that turns it on has made a
// change a listing can see. An option that was already on could not tell
// "restored" from "never moved".

// The row the issue opens with: a function that asks for the scoping does not
// leak what it changed.
func TestLocalOptionsRestoresOnReturn(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`f() { setopt localoptions extendedglob; }; f; [[ -o extendedglob ]] || echo gone`)
	if st != 0 || out != "gone\n" {
		t.Errorf("out %q status %d, want no extendedglob left behind", out, st)
	}
}

// The save is taken when the body starts, not when the `setopt localoptions`
// line runs: an option moved *before* that line is restored too. Measured.
func TestLocalOptionsRestoresAnOptionMovedBeforeItWasAskedFor(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`f() { setopt extendedglob; setopt localoptions; }; f; [[ -o extendedglob ]] || echo gone`)
	if st != 0 || out != "gone\n" {
		t.Errorf("out %q status %d, want the earlier change restored as well", out, st)
	}
}

// Whether to restore is asked at the *return*, so a function that turns the
// option back off keeps everything it moved. Measured with the option on
// globally, which is the only arrangement that can tell this from the option
// simply never having been on.
func TestLocalOptionsIsAskedAtTheReturn(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`setopt localoptions; f() { unsetopt localoptions; setopt extendedglob; }; f; `+
			`[[ -o extendedglob ]] && echo kept; [[ -o localoptions ]] && echo lo_on`)
	if st != 0 || out != "kept\nlo_on\n" {
		t.Errorf("out %q status %d, want the change kept and localoptions itself back on", out, st)
	}
}

// And `localoptions` itself comes back whatever else does not, which is the
// second half of the row above and the one rule that the restore alone cannot
// account for: at that return the option was off, nothing else was put back,
// and it is on again afterwards.
func TestLocalOptionsItselfIsAlwaysRestored(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`setopt localoptions; f() { unsetopt localoptions; [[ -o localoptions ]] || echo off_inside; }; f; `+
			`[[ -o localoptions ]] && echo on_after`)
	if st != 0 || out != "off_inside\non_after\n" {
		t.Errorf("out %q status %d, want it off in the body and back on after", out, st)
	}
}

// Every call has a save of its own and asks the question at its own return,
// so an inner function that never mentions the option still restores while
// the outer one is still running — the option is on when it returns.
func TestLocalOptionsRestoresAtEachNestedReturn(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`i() { setopt extendedglob; }; o() { setopt localoptions; i; `+
			`[[ -o extendedglob ]] || echo gone_in_outer; }; o; `+
			`[[ -o extendedglob ]] || echo gone_after`)
	if st != 0 || out != "gone_in_outer\ngone_after\n" {
		t.Errorf("out %q status %d, want the inner call to restore on its own return", out, st)
	}
}

// With nothing asking for the scoping a change leaks, which is what says the
// rows above are the option working rather than functions never leaking.
func TestOptionsLeakWithoutLocalOptions(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`i() { setopt extendedglob; }; o() { i; }; o; [[ -o extendedglob ]] && echo kept`)
	if st != 0 || out != "kept\n" {
		t.Errorf("out %q status %d, want the change to survive with no scoping asked for", out, st)
	}
}

// `unsetopt` is localised the same way `setopt` is. `nomatch` is on by
// default, so turning it off is the visible direction.
func TestLocalOptionsRestoresAnUnsetopt(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`f() { setopt localoptions; unsetopt nomatch; }; f; [[ -o nomatch ]] && echo back`)
	if st != 0 || out != "back\n" {
		t.Errorf("out %q status %d, want nomatch back on", out, st)
	}
}

// A `return` from the middle of the body restores exactly as falling off the
// end does, and the status the `return` asked for still travels.
func TestLocalOptionsRestoresOnAnEarlyReturn(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`f() { setopt localoptions; setopt extendedglob; return 3; }; f; echo rc=$?; `+
			`[[ -o extendedglob ]] || echo gone`)
	if st != 0 || out != "rc=3\ngone\n" {
		t.Errorf("out %q status %d, want status 3 and the option gone", out, st)
	}
}

// The names backed by a semantics axis rather than by a flag are localised
// too — `ksharrays` moves the array base, and it is 1-based again afterwards.
func TestLocalOptionsRestoresAnAxisBackedOption(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`f() { setopt localoptions; setopt ksharrays; }; f; a=(x y z); echo ${a[1]}`)
	if st != 0 || out != "x\n" {
		t.Errorf("out %q status %d, want the array base back at one", out, st)
	}
}

// And so are the ones a script moves through `set` rather than through this
// dialect's own builtin: it is the option table that is saved, not the
// builtin that wrote it.
func TestLocalOptionsRestoresWhatSetMoved(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`f() { setopt localoptions; set -o noglob; }; f; [[ -o noglob ]] || echo restored`)
	if st != 0 || out != "restored\n" {
		t.Errorf("out %q status %d, want `set -o noglob` restored", out, st)
	}
}

// An anonymous function is a function call and gets a scope of its own.
func TestLocalOptionsRestoresForAnAnonymousFunction(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`() { setopt localoptions; setopt extendedglob; }; [[ -o extendedglob ]] || echo gone`)
	if st != 0 || out != "gone\n" {
		t.Errorf("out %q status %d, want the anonymous call to restore", out, st)
	}
}

// `emulate -L` is the other spelling of the same thing, and it is now the
// same mechanism: the option reads on inside the call, and the emulation and
// everything moved with it goes back at the return.
func TestEmulateLIsLocalOptions(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`f() { emulate -L zsh; [[ -o localoptions ]] && echo lo_on; setopt extendedglob; }; f; `+
			`[[ -o extendedglob ]] || echo gone`)
	if st != 0 || out != "lo_on\ngone\n" {
		t.Errorf("out %q status %d, want `emulate -L` to be localoptions", out, st)
	}
}

// Which is what fixes the row the two spellings used to disagree on: with the
// save taken at the call rather than at the `emulate` line, an option moved
// before `emulate -L` is restored too.
func TestEmulateLRestoresAnOptionMovedBeforeIt(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`f() { setopt extendedglob; emulate -L zsh; }; f; [[ -o extendedglob ]] || echo gone`)
	if st != 0 || out != "gone\n" {
		t.Errorf("out %q status %d, want the earlier change restored", out, st)
	}
}

// At the top level there is no call to return from, so `emulate -L` leaves
// the option on — and the *next* function call localises. Measured, and it is
// what says the letter is the option and not a scope of its own.
func TestEmulateLAtTheTopLevelLeavesTheOptionOn(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`emulate -L zsh; [[ -o localoptions ]] && echo lo_on; `+
			`f() { setopt extendedglob; }; f; [[ -o extendedglob ]] || echo gone`)
	if st != 0 || out != "lo_on\ngone\n" {
		t.Errorf("out %q status %d, want the option left on and the next call localised", out, st)
	}
}

// A bare `emulate` resets every option to that emulation's default, and
// `localoptions` defaults off — so an emulation asked for inside a function
// switches the scoping *off* and everything it moved is kept. Measured, and
// it is why the mode travels with the table rather than beside it.
func TestABareEmulateTurnsTheScopingOff(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`f() { setopt localoptions; emulate sh; }; f; [[ -o shwordsplit ]] && echo ws_on; emulate`)
	if st != 0 || out != "ws_on\nsh\n" {
		t.Errorf("out %q status %d, want the emulation kept, mode and all", out, st)
	}
}

// And the same in reverse: with the option still on at the return the mode
// goes back with the rest of the table.
func TestTheEmulationModeIsRestoredWithTheTable(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `f() { emulate -L sh; emulate; }; f; emulate`)
	if st != 0 || out != "sh\nzsh\n" {
		t.Errorf("out %q status %d, want the mode restored at the return", out, st)
	}
}

// Traps are not options and have an option of their own — measured, `setopt
// localoptions` leaves a trap the function installed in place.
func TestLocalOptionsDoesNotLocaliseTraps(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`f() { setopt localoptions; trap 'echo hit' USR1; }; f; trap`)
	if st != 0 || out != "trap -- 'echo hit' USR1\n" {
		t.Errorf("out %q status %d, want the trap still installed", out, st)
	}
}
