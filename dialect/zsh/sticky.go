// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"fmt"
	"slices"
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
	// calls enter, as a word stickyWord built.
	stickyFunctions = ".zsh.sticky"
	// stickyCurrent holds the word of the sticky emulation in force: the one
	// an `emulate … -c` is running its code under, or the one a sticky call
	// entered. It is how the definition hook knows which emulation a
	// definition is made under, and how a call knows whether the emulation
	// it would enter is the one already in force. Empty at the top level.
	stickyCurrent = ".zsh.sticky.current"
	// stickyStrictMark is the `-R` flag's spelling inside a stored word. A
	// character no emulation mode is named with, so the word splits without
	// a second parameter.
	stickyStrictMark = "!"
)

// # Which emulation is "the same one"
//
// zsh does not enter a sticky emulation that is already in force: nothing is
// saved at the call and nothing is put back at its return, so an option the
// callee sets is still set in its caller (#5144). Measured on zsh 5.9.2,
// 2026-09-30, with a sticky callee that runs `setopt alwayslastprompt` and a
// caller that unsets it, calls, and reads it:
//
//   - **In force** means the `-c` run itself or a sticky call entered from
//     it, and it lasts through a plain function in between — sh → plain → sh
//     reads `on`. The top level has none, even after a bare `emulate sh`, and
//     a plain function called from it has none either: both read `off`.
//   - **The mode and `-R` are part of it**: sh → csh, sh → zsh and `-R sh` →
//     `sh` all read `off`.
//   - **So are the `-o`/`+o` words, as written and not as they come out.** A
//     redundant `-o shwordsplit` under sh is a different emulation from `sh`,
//     and `-o nullglob -o nullglob` is a different one from `-o nullglob`. But
//     the order of the words is not part of it, and a name's spellings are
//     one name: `-o nullglob -o markdirs` is `-o markdirs -o nullglob`, and
//     `-o nonullglob` is `+o nullglob`.
//   - **A name keeps its count and its last direction.** `-o ng +o ng` is
//     `+o ng +o ng` and not `+o ng -o ng`; `ng,md,ng` is `md,ng,ng` and not
//     `md,ng,md`.
//
// So the word is the mode, the strict mark, and every name written, sorted,
// with how many times it was written and which way it was left. Two
// emulations are the same exactly when their words are.
//
// The words also do the applying: the `-o` settings are entered at every
// triggered call, not only during the `-c` run (#5244), which is what the
// direction beside each name is read back for.

// stickyWord is how a mode, the strict flag and the option words written
// beside them travel as one value. Names no option answers to are left out:
// the `-c` has already reported them, and they set nothing.
func stickyWord(mode string, strict bool, opts []emulateOption) string {
	var b strings.Builder
	if strict {
		b.WriteString(stickyStrictMark)
	}
	b.WriteString(mode)
	if len(opts) == 0 {
		return b.String()
	}
	type written struct {
		count int
		on    bool
	}
	seen := make(map[string]*written, len(opts))
	for _, o := range opts {
		opt, inverted, ok := resolveOptionName(normalizeOption(o.name))
		if !ok {
			continue
		}
		w := seen[opt.base]
		if w == nil {
			w = &written{}
			seen[opt.base] = w
		}
		w.count++
		w.on = o.on != inverted
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		w := seen[n]
		dir := "-"
		if w.on {
			dir = "+"
		}
		fmt.Fprintf(&b, " %s=%d%s", n, w.count, dir)
	}
	return b.String()
}

// stickyMode reads the mode and the strict flag back.
func stickyMode(word string) (mode string, strict bool) {
	word, _, _ = strings.Cut(word, " ")
	if rest, ok := strings.CutPrefix(word, stickyStrictMark); ok {
		return rest, true
	}
	return word, false
}

// applyStickyOptions sets each option the word names the way it was left.
func applyStickyOptions(r *interp.Runner, word string) {
	_, rest, _ := strings.Cut(word, " ")
	for _, f := range strings.Fields(rest) {
		name, count, ok := strings.Cut(f, "=")
		if !ok || count == "" {
			continue
		}
		setOption(r, name, strings.HasSuffix(count, "+"))
	}
}

// enterSticky makes word the sticky emulation in force. Returns the restore.
func enterSticky(r *interp.Runner, word string) func() {
	outer, _ := r.GetVar(stickyCurrent)
	r.SetVar(stickyCurrent, word)
	return func() { r.SetVar(stickyCurrent, outer) }
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
		// The emulation in force, whether an `emulate … -c` put it there or
		// a sticky call did: a function defined while a sticky function runs
		// is sticky too, and so is one a plain function defines when a sticky
		// one called it (#5245).
		word, _ := r.GetVar(stickyCurrent)
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
		if current, _ := r.GetVar(stickyCurrent); current == word {
			// Already in force, so not entered: nothing saved and nothing put
			// back, and what the body sets its caller keeps.
			return nil
		}
		mode, strict := stickyMode(word)
		if _, known := emulations[mode]; !known {
			return nil
		}
		saved := saveOptionState(r)
		applyEmulation(r, mode, strict)
		applyStickyOptions(r, word)
		leave := enterSticky(r, word)
		return func() {
			leave()
			saved.restore(r)
		}
	})
}
