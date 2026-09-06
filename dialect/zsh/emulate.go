// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"

	"github.com/blairham/sh/interp"
)

// `emulate` switches this shell between zsh's semantics and the sh family's,
// which zsh scripts do at the top of anything meant to travel.
//
// What one emulation means against another was measured probe by probe rather
// than assumed, and three axes carry all of it that this shell distinguishes:
// `emulate sh` and `emulate ksh` split unquoted expansions (shwordsplit),
// pass an unmatched glob through as itself (nonomatch), and base arrays at
// zero (ksharrays); `emulate zsh` puts all three back. The switch also resets
// every changeable option to the emulation's defaults — measured: `setopt
// err_exit; emulate zsh` turns errexit back off, no `-R` required — and the
// bare-listing baseline moves with it, which the option table's `def` field
// records.
//
// The rest of what real zsh folds into an emulation — its ~180 options, csh's
// separate glob wording, `$0`-versus-function-name rules — is not modeled;
// `emulate csh` here records the mode and changes nothing. `emulate -L`, the
// function-local form, is refused out loud rather than silently made global:
// this shell has no seam for restoring semantics on function return.
// docs/spec/semantics.md records both boundaries.
//
// Measured shapes: a bare `emulate` prints the current mode and 0; a word
// that names no emulation — `fish`, or `SH` in the wrong case — is passed
// over in silence, status 0, the mode unchanged; a second operand is
// `unknown argument`, 1; `-c code` runs the code under the emulation and
// then restores everything, options included, reporting the code's status.

// emulationMode is where the current mode lives. A parameter rather than a
// package variable because a subshell must keep its own: the Vars table is
// deep-copied into a clone and closure state would be shared across it. The
// name is unreachable from a script, the way ksh93 hides its own state under
// `.sh.*`, and it is only written once something emulates.
const emulationMode = ".zsh.emulation"

// currentEmulation reads the mode.
func currentEmulation(r *interp.Runner) string {
	if m, ok := r.GetVar(emulationMode); ok && m != "" {
		return m
	}
	return "zsh"
}

// emulations is what each mode means on the four axes this shell can move.
// csh changes nothing it can speak about.
//
// redirFatal is the fourth and arrived last: `emulate sh` and `emulate ksh`
// make a failed redirection on a special builtin end the script, where
// `emulate zsh` leaves it a complaint the script runs past. That is the same
// switch bash's `set -o posix` throws, measured in a second binary, which is
// what says the axis belongs to the mode rather than to either shell.
var emulations = map[string]struct{ split, nomatchOk, zeroBase, redirFatal bool }{
	"zsh": {split: false, nomatchOk: false, zeroBase: false, redirFatal: false},
	"sh":  {split: true, nomatchOk: true, zeroBase: true, redirFatal: true},
	"ksh": {split: true, nomatchOk: true, zeroBase: true, redirFatal: true},
	"csh": {},
}

// applyEmulation switches the axes and resets every changeable option to the
// emulation's default.
func applyEmulation(r *interp.Runner, mode string) {
	e := emulations[mode]
	if mode != "csh" {
		swapAxes(r, func(s *interp.Semantics) {
			s.SplitParamExpansion = answer(e.split)
			s.GlobNoMatchIsError = answer(!e.nomatchOk)
			s.ArrayBaseIsZero = answer(e.zeroBase)
			s.RedirectErrorOnSpecialBuiltinFatal = answer(e.redirFatal)
		})
	}
	// The recorded names go back to their defaults in one write rather than
	// 157 — the store holds deviations, so an empty store *is* every recorded
	// option at its default.
	setRecordedOptions(r, nil)
	for _, o := range zshOptions {
		if o.set == nil || o.recorded {
			continue
		}
		switch o.base {
		case "shwordsplit", "nomatch", "ksharrays":
			// Already placed by the axis swap, and their default is the
			// emulation's rather than the table's.
			continue
		}
		_ = o.set(r, o.def)
	}
	r.SetVar(emulationMode, mode)
}

// registerEmulate installs the builtin.
func registerEmulate(r *interp.Runner) {
	r.Register("emulate", emulateBuiltin)
}

func emulateBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	mode, code, hasCode, status := emulateArguments(r, args)
	if status >= 0 {
		return status
	}
	if mode == "" {
		_, _ = fmt.Fprintf(r.Out(), "%s\n", currentEmulation(r))
		return 0
	}
	if _, known := emulations[mode]; !known {
		// Measured: a word naming no emulation is passed over in silence,
		// the mode unchanged — `emulate fish` and `emulate SH` alike.
		return 0
	}
	if !hasCode {
		applyEmulation(r, mode)
		return 0
	}
	// `-c` runs the string under the emulation and restores everything after
	// — measured, an option set before it comes back: `setopt no_glob;
	// emulate sh -c '…'` still refuses to glob afterwards.
	saved := saveEmulationState(r)
	applyEmulation(r, mode)
	st := 0
	if eval, ok := r.Builtin("eval"); ok {
		st = eval(r, ctx, []string{code})
	}
	saved.restore(r)
	return st
}

// emulateArguments reads the command line. A status of -1 means "carry on";
// anything else is the answer, already reported.
func emulateArguments(r *interp.Runner, args []string) (mode, code string, hasCode bool, status int) {
	local := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) > 1 && a[0] == '-' {
			for _, letter := range a[1:] {
				switch letter {
				case 'R':
					// A plain emulation already resets the options —
					// measured — so the strict form adds nothing here.
				case 'L':
					local = true
				case 'c':
					if i+1 >= len(args) {
						r.Diagnosef("string expected after -c\n")
						return "", "", false, 1
					}
					i++
					code, hasCode = args[i], true
				default:
					r.Diagnosef("bad option: -%c\n", letter)
					return "", "", false, 1
				}
			}
			continue
		}
		if mode != "" {
			r.Diagnosef("unknown argument %s\n", a)
			return "", "", false, 1
		}
		mode = a
	}
	if mode == "" && len(args) > 0 {
		// Flags with nothing to emulate, which is what real zsh says before
		// looking at the flags themselves.
		r.Diagnosef("not enough arguments\n")
		return "", "", false, 1
	}
	if local {
		// The function-local form needs a restore on return, which this
		// shell has no seam for. Refused rather than silently made global.
		r.Diagnosef("emulate: -L is not implemented yet\n")
		return "", "", false, 2
	}
	return mode, code, hasCode, -1
}

// emulationState is what `-c` puts back: the semantics vector by pointer, the
// mode, and each changeable option's state — the vector restore covers the
// axes, and the option flags live outside it.
type emulationState struct {
	sem     *interp.Semantics
	mode    string
	options map[string]bool
	// recorded is the recorded-option store as it stood, saved whole for the
	// same reason applyEmulation clears it whole.
	recorded []string
}

func saveEmulationState(r *interp.Runner) emulationState {
	s := emulationState{
		sem: r.Semantics, mode: currentEmulation(r),
		options: map[string]bool{}, recorded: recordedOptions(r),
	}
	for _, o := range zshOptions {
		if o.set != nil && !o.recorded {
			s.options[o.base] = o.get(r)
		}
	}
	return s
}

func (s emulationState) restore(r *interp.Runner) {
	r.Semantics = s.sem
	setRecordedOptions(r, s.recorded)
	for _, o := range zshOptions {
		if o.set == nil || o.recorded {
			continue
		}
		switch o.base {
		case "shwordsplit", "nomatch", "ksharrays":
			// The vector restore has these, and re-setting them would swap
			// in a fresh copy for nothing.
			continue
		}
		_ = o.set(r, s.options[o.base])
	}
	r.SetVar(emulationMode, s.mode)
}
