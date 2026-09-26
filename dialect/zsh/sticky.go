// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strings"

	"github.com/blairham/sh/interp"
)

// A function defined inside `emulate … -c` is **sticky**: the emulation is
// entered again every time that function is later called, and left when it
// returns.
//
// # What was measured
//
// zsh 5.9.2 (`/opt/homebrew/bin/zsh`, `-f`), 2026-09-26, over a script file
// that prints `${options[shwordsplit]}`, `${options[ksharrays]}`,
// `${options[extendedglob]}` and `$(emulate)` from inside a body and from the
// top level:
//
//  1. **The `-c` run itself is scoped correctly already.** Before and after
//     `emulate -R sh -c '…'` the top level reads `off off off zsh`, which is
//     the control: what is missing is only that the definition carries the
//     emulation to its later calls.
//  2. **The call enters the emulation.** `s1` defined that way reads `on on
//     … sh` however the shell stood when it was called.
//  3. **The mode and the strictness are the definition's**, not the call's.
//     With `extendedglob` turned on at the top level between the definition
//     and the call, a body defined under `-R sh` still reads `eg=off`: the
//     `-R` reset travels with the function.
//  4. **`-R` is not required for stickiness.** `emulate sh -c '…'` marks the
//     function too, and its calls enter the plain emulation.
//  5. **The whole option table goes back at the return.** A sticky body that
//     runs `setopt errexit` leaves the caller with `errexit` off.
//  6. **The mark is the definition's and the last definition wins.** A
//     function that already existed and is redefined by an `emulate … -c`
//     becomes sticky, and one that was sticky and is redefined at the top
//     level stops being sticky — `em=zsh` again. Those two rows are why this
//     hangs off [interp.Runner.AtFunctionDefinition] rather than off a
//     difference between the function tables before and after the `-c`: a
//     redefinition changes no name, and a name going out of the sticky set
//     happens where no emulation is running at all.
//  7. **What a sticky function calls is not itself sticky**, and needs no
//     mark to behave: an ordinary function called from a sticky body runs
//     under the emulation the body is in, exactly as any callee runs under
//     its caller's options.
//
// # Where the state lives
//
// An association under a name no script can reach, the shape `zstyle`,
// `emulate` and the recorded options already use. Keyed by the function's
// name so that a call costs one map lookup and no allocation — see
// interp.Runner.AssocElement, which exists for exactly that reason — and a
// subshell keeps its own copy because the tables are deep-copied into a
// clone.
//
// An empty value is the absence of a mark rather than a mark on the empty
// mode, which is what lets a redefinition clear one without the table
// needing a delete.
const (
	// stickyFunctions maps a function's name to the emulation its later
	// calls enter: the mode word, with a `!` in front of it where the
	// definition was made under `-R`.
	stickyFunctions = ".zsh.sticky"
	// stickyDefining holds the same word while an `emulate … -c` is running
	// its code, which is how the definition hook knows a definition made
	// inside one from a definition made anywhere else.
	stickyDefining = ".zsh.sticky.defining"
	// stickyStrictMark is the `-R` flag's spelling inside a stored word. A
	// character no emulation mode is named with, so the word splits without
	// a second parameter.
	stickyStrictMark = "!"
)

// stickyWord is how a mode and the strict flag travel as one value.
func stickyWord(mode string, strict bool) string {
	if strict {
		return stickyStrictMark + mode
	}
	return mode
}

// stickyMode reads one back.
func stickyMode(word string) (mode string, strict bool) {
	if rest, ok := strings.CutPrefix(word, stickyStrictMark); ok {
		return rest, true
	}
	return word, false
}

// definingSticky marks the code an `emulate … -c` is about to run, so every
// function it defines is marked with this emulation. Returns the restore.
func definingSticky(r *interp.Runner, mode string, strict bool) func() {
	outer, _ := r.GetVar(stickyDefining)
	r.SetVar(stickyDefining, stickyWord(mode, strict))
	return func() { r.SetVar(stickyDefining, outer) }
}

// registerStickyEmulation hangs the two halves on this runner: the mark, taken
// at every definition, and the emulation, entered at every call that finds
// one.
//
// Once per shell, beside registerLocalOptions — and after it, which is what
// puts the sticky restore *inside* the option-table restore that call already
// takes. Both saved the same table, so either order gives the caller its
// options back; the order here is the one that reads the way the shell
// behaves, with the emulation the inner thing.
func registerStickyEmulation(r *interp.Runner) {
	r.AtFunctionDefinition(func(r *interp.Runner, name string) {
		word, _ := r.GetVar(stickyDefining)
		if word == "" {
			if _, marked := r.AssocElement(stickyFunctions, name); !marked {
				// The common case by a very long way: nothing is sticky and
				// nothing was, so the definition writes no table at all.
				return
			}
		}
		r.SetAssocElement(stickyFunctions, name, word)
	})
	r.AtEveryFunctionCall(func(r *interp.Runner) func() {
		word, ok := r.AssocElement(stickyFunctions, r.RunningFunction())
		if !ok || word == "" {
			return nil
		}
		mode, strict := stickyMode(word)
		if _, known := emulations[mode]; !known {
			return nil
		}
		saved := saveOptionState(r)
		applyEmulation(r, mode, strict)
		return func() { saved.restore(r) }
	})
}
