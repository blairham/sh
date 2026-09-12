// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"strings"
)

// The two parameters that report — and decide — how the try half of a
// try-always block ended.
//
// One shell in the panel has the construct and it keeps two integers beside
// it: one for whether the try half raised an **error condition**, one for
// whether it was interrupted. Measured against zsh 5.9.2 on 2026-09-12, from
// a script file under `env -i PATH=/usr/bin:/bin` with a scratch `HOME` and
// `ZDOTDIR` (#1234):
//
//	print ${(t)TRY_BLOCK_ERROR}                    integer-special
//	typeset -p TRY_BLOCK_ERROR                     typeset -i10 TRY_BLOCK_ERROR=-1
//	f(){ { false; } always { print $TRY_BLOCK_ERROR; }; }; f            0
//	f(){ { return 3; } always { print $TRY_BLOCK_ERROR; }; }; f         0
//	f(){ { print $((1/0)); } always { print $TRY_BLOCK_ERROR; }; }; f   1
//	f(){ { readonly r=1; r=2; } always { print $TRY_BLOCK_ERROR; }; }; f 1
//	f(){ { break; } always { print $TRY_BLOCK_ERROR; }; }; f            1
//	f(){ { true; } always { :; }; print $TRY_BLOCK_ERROR; }; f         -1
//
// So **a nonzero status is not an error condition**: `false` and a `return`
// leave it 0, and a division by zero, a readonly reassignment and a `break`
// with no loop to leave — the failures the shell reports and gives up over —
// leave it 1. That is exactly the question [Runner.errorCondition] already
// asks, which is why this is a reading of state the construct kept anyway
// rather than a second model of it.
//
// # Writing it is how a script recovers, and how it raises
//
// The interesting half, and the reason these are not readonly. The value the
// always half leaves behind is what the construct acts on:
//
//	f(){ { readonly r=1; r=2; } always { TRY_BLOCK_ERROR=0; }; print after; }; f
//	    the complaint, then `after` — status 0
//	f(){ { true; } always { TRY_BLOCK_ERROR=1; }; print after; }; f
//	    nothing at all — status 1, and `f || print caught` does not catch it
//	f(){ { true; } always { TRY_BLOCK_ERROR=7; }; print after; }; f
//	    the same, and the status is **1** rather than the 7 that was written
//
// [Runner.SetAlwaysBlockStatus] installs them; tryClause reads the pair back
// through [Runner.leaveAlwaysHalf].
//
// # Outside a block they are an ordinary integer parameter
//
// `-1`, and a script may assign to one and read its own value back:
// `TRY_BLOCK_ERROR=5; print $TRY_BLOCK_ERROR` is `5`. Entering an always half
// puts that aside and restores it on the way out — measured, `TRY_BLOCK_ERROR=5`
// then a block reads `0` inside it and `5` after. The construct's own value is
// therefore not a value the parameter *has*; it is one it takes on for the
// length of the half, which is what the save-and-restore below is.

// alwaysBlockAbsent is what the two names read where no always half is
// running. A value rather than an absence: `${TRY_BLOCK_ERROR-UNSET}` is `-1`
// in the shell with the construct.
const alwaysBlockAbsent = -1

// SetAlwaysBlockStatus installs the two integer parameters a try-always block
// reports through, under the names the caller gives.
//
// The names are the dialect's and the mechanism is the core's — the same split
// [Runner.ProvideWindowSize] makes, and for the same reason: what the two
// numbers mean is a fact about the construct, which lives here, while what
// they are spelled is a fact about one shell. Nothing in this package writes
// either spelling down.
//
// The integer attribute is set as well as the producers registered, and it is
// not decoration: it is what makes `${(t)TRY_BLOCK_ERROR}` say `integer` and
// what makes an assignment of `1+1` two rather than the text.
func (r *Runner) SetAlwaysBlockStatus(errName, interruptName string) {
	r.alwaysErrName, r.alwaysInterruptName = errName, interruptName
	if r.integer == nil {
		r.integer = map[string]bool{}
	}
	if r.integerBase == nil {
		r.integerBase = map[string]int{}
	}
	for _, name := range []string{errName, interruptName} {
		r.integer[name] = true
		// Base ten written down, which is what separates a special integer
		// from one a script declared — `typeset -p TRY_BLOCK_ERROR` is
		// `typeset -i10 TRY_BLOCK_ERROR=-1`. The same rule ProvideWindowSize
		// records for `$COLUMNS`.
		r.integerBase[name] = 10
	}
	r.SetDynamic(errName, func(r *Runner) string {
		return r.alwaysBlockValue(errName, r.alwaysErrValue).read()
	})
	r.SetDynamic(interruptName, func(r *Runner) string {
		return r.alwaysBlockValue(interruptName, r.alwaysInterruptValue).read()
	})
}

// alwaysBlockValue is one of the two names, read.
//
// What a script assigned wins, which is what lets an always half write the
// value and read it back before the half ends. Outside a half there is
// nothing to shadow, so the same rule gives a script its own assignment there
// too — and [Runner.enterAlwaysHalf] is what makes the two cases one, by
// putting any standing assignment aside before the half begins.
func (r *Runner) alwaysBlockValue(name string, inside int) alwaysBlockRead {
	if v, ok := r.assigned[name]; ok {
		return alwaysBlockRead{text: v, set: true}
	}
	if r.alwaysDepth == 0 {
		return alwaysBlockRead{n: alwaysBlockAbsent}
	}
	return alwaysBlockRead{n: inside}
}

// alwaysBlockRead is either the text a script assigned or a number the
// construct chose, so that a read gives back exactly what was written and a
// *decision* is made on the number.
type alwaysBlockRead struct {
	text string
	set  bool
	n    int
}

// read is what a parameter expansion of the name yields.
func (v alwaysBlockRead) read() string {
	if v.set {
		return v.text
	}
	return itoa(v.n)
}

// alwaysHalfSaved is what enterAlwaysHalf put aside, for leaveAlwaysHalf to
// put back. A value rather than a closure so that the restore cannot be
// forgotten by a path that returns early — every caller holds one.
type alwaysHalfSaved struct {
	errText, interruptText string
	errSet, interruptSet   bool
	errValue, interruptVal int
	// errored is what the try half raised, kept so that a dialect with no
	// such parameters gets its own answer back from leaveAlwaysHalf. Without
	// it the pair reports "no error condition" for every shell that never
	// registered the names, and the construct then *clears* every error it
	// was built to survive — which is what the core's own tests caught.
	errored bool
}

// enterAlwaysHalf puts the two parameters into the state the always half sees:
// the construct's numbers, with whatever a script had assigned set aside.
//
// errored is whether the try half raised an error condition.
func (r *Runner) enterAlwaysHalf(errored bool) alwaysHalfSaved {
	saved := alwaysHalfSaved{
		errValue: r.alwaysErrValue, interruptVal: r.alwaysInterruptValue, errored: errored,
	}
	if r.alwaysErrName == "" {
		return saved
	}
	saved.errText, saved.errSet = r.assigned[r.alwaysErrName]
	saved.interruptText, saved.interruptSet = r.assigned[r.alwaysInterruptName]
	delete(r.assigned, r.alwaysErrName)
	delete(r.assigned, r.alwaysInterruptName)
	r.alwaysErrValue = 0
	if errored {
		r.alwaysErrValue = 1
	}
	// Always nought, and that is a measurement rather than a placeholder: the
	// shell reports an *interrupt* here, and every non-interrupt ending —
	// including a division by zero and a `break` with no loop, which both set
	// the error to 1 — leaves this one 0.
	r.alwaysInterruptValue = 0
	r.alwaysDepth++
	return saved
}

// leaveAlwaysHalf puts the parameters back and reports whether the half left
// an error condition standing.
//
// Either number being nonzero is one, which is measured in both places it can
// come from: the construct's own 1 left alone, and a `TRY_BLOCK_INTERRUPT=1`
// written by hand after a try half that raised nothing.
func (r *Runner) leaveAlwaysHalf(saved alwaysHalfSaved) bool {
	if r.alwaysErrName == "" {
		// Nothing to have rewritten it, so the answer is the one it came in
		// with and the construct behaves exactly as it did before there were
		// parameters at all.
		return saved.errored
	}
	// Read while the half is still counted as running. Decrementing first
	// makes both names fall back to the -1 they read *outside* a half, which
	// is nonzero — so every block would have ended by raising an error
	// condition, and a `{ true; } always { : }` would have stopped the shell.
	errored := r.alwaysBlockValue(r.alwaysErrName, r.alwaysErrValue).nonzero() ||
		r.alwaysBlockValue(r.alwaysInterruptName, r.alwaysInterruptValue).nonzero()
	r.alwaysDepth--
	r.alwaysErrValue, r.alwaysInterruptValue = saved.errValue, saved.interruptVal
	restore(r.assigned, r.alwaysErrName, saved.errText, saved.errSet)
	restore(r.assigned, r.alwaysInterruptName, saved.interruptText, saved.interruptSet)
	return errored
}

func restore(m map[string]string, name, text string, set bool) {
	if set {
		m[name] = text
		return
	}
	delete(m, name)
}

// nonzero is whether the value the half left behind asks for an error
// condition.
//
// The text is already the number: the two names carry the integer attribute,
// so `TRY_BLOCK_ERROR=1+1` was folded to `2` on its way into the assignment
// and there is no expression left to evaluate here. A value that somehow is
// not a number counts as nought — the same direction an integer name takes
// text it cannot read.
func (v alwaysBlockRead) nonzero() bool {
	if !v.set {
		return v.n != 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(v.text))
	return err == nil && n != 0
}
