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
// `emulate csh` here records the mode and changes nothing.
// docs/spec/semantics.md records the boundary.
//
// `emulate -L`, the function-local form, is `setopt localoptions` after the
// emulation and nothing else. It had a save-and-restore of its own once, and
// that was two mistakes: it saved at its own line rather than at the function
// entry, so an option moved earlier in the same body leaked, and it restored
// whether or not the option was still on at the return. Both are measured the
// other way; localoptions.go carries the rule now, and both spellings reach
// it.
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
			// Five axes rather than the base alone: `ksharrays` is what the
			// two sh-family emulations turn on, and the whole of what it
			// means is in ksharrays.go. Setting only the base left `emulate
			// sh` reading `$a` as the joined list and `$a[1]` as an element,
			// which is neither shell's answer (#1726).
			setKshArrays(s, e.zeroBase)
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
	e, status := emulateArguments(r, args)
	if status >= 0 {
		return status
	}
	if e.mode == "" {
		_, _ = fmt.Fprintf(r.Out(), "%s\n", currentEmulation(r))
		return 0
	}
	if _, known := emulations[e.mode]; !known {
		// Measured: a word naming no emulation is passed over in silence,
		// the mode unchanged — `emulate fish` and `emulate SH` alike.
		return 0
	}
	if !e.hasCode {
		applyEmulation(r, e.mode)
		if e.local {
			// `-L` is LOCAL_OPTIONS and nothing besides, which is measured
			// rather than assumed: inside `emulate -L zsh` the option reads
			// on, and at the top level — where there is no call to return
			// from — it stays on afterwards and localizes the *next*
			// function call. So the letter is one `setopt` and the
			// function-call machinery does the rest; localoptions.go is
			// where the rest is, and it is the same machinery `setopt
			// localoptions` reaches.
			//
			// After the emulation rather than before it, because a plain
			// emulation resets every option to that emulation's default and
			// `localoptions` defaults off — which is exactly why a bare
			// `emulate sh` in a function does *not* localize, measured.
			setLocalOptions(r, true)
		}
		return e.applyOptions(r)
	}
	// `-c` runs the string under the emulation and restores everything after
	// — measured, an option set before it comes back: `setopt no_glob;
	// emulate sh -c '…'` still refuses to glob afterwards.
	saved := saveOptionState(r)
	applyEmulation(r, e.mode)
	st := e.applyOptions(r)
	if eval, ok := r.Builtin("eval"); ok {
		st = eval(r, ctx, []string{e.code})
	}
	saved.restore(r)
	return st
}

// emulateArguments reads the command line. A status of -1 means "carry on";
// anything else is the answer, already reported.
func emulateArguments(r *interp.Runner, args []string) (e emulateCall, status int) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) > 1 && a[0] == '+' {
			// The `+` form. `+o name` is `-o name` the other way round and
			// `+c` runs its string like `-c`; every other letter is
			// *accepted and does nothing*, which is measured rather than
			// assumed and is not what the `-` form does: `emulate zsh +X`
			// is 0 where `emulate -X zsh` is `bad option: -X`, and a `+L`
			// does not undo a `-L` — the emulation stays function-local.
			for j := 1; j < len(a); j++ {
				switch a[j] {
				case 'o', 'c':
					if i+1 >= len(args) {
						r.Diagnosef("string expected after +%c\n", a[j])
						return e, 1
					}
					i++
					if a[j] == 'c' {
						e.code, e.hasCode = args[i], true
					} else {
						e.options = append(e.options, emulateOption{name: args[i]})
					}
				}
			}
			continue
		}
		if len(a) > 1 && a[0] == '-' {
			for _, letter := range a[1:] {
				switch letter {
				case 'R':
					// A plain emulation already resets the options —
					// measured — so the strict form adds nothing here.
				case 'L':
					e.local = true
				case 'o':
					if i+1 >= len(args) {
						r.Diagnosef("string expected after -o\n")
						return e, 1
					}
					i++
					e.options = append(e.options, emulateOption{name: args[i], on: true})
				case 'c':
					if i+1 >= len(args) {
						r.Diagnosef("string expected after -c\n")
						return e, 1
					}
					i++
					e.code, e.hasCode = args[i], true
				default:
					r.Diagnosef("bad option: -%c\n", letter)
					return e, 1
				}
			}
			continue
		}
		if e.mode != "" {
			r.Diagnosef("unknown argument %s\n", a)
			return e, 1
		}
		e.mode = a
	}
	if e.mode == "" && len(args) > 0 {
		if len(e.options) > 0 {
			// An option with no emulation to apply it to. Measured: real
			// zsh answers `bad option: -o` here rather than complaining
			// about the count, which is why this is not the line below.
			r.Diagnosef("bad option: -o\n")
			return e, 1
		}
		// Flags with nothing to emulate, which is what real zsh says before
		// looking at the flags themselves.
		r.Diagnosef("not enough arguments\n")
		return e, 1
	}
	return e, -1
}

// emulateCall is one command line, read.
type emulateCall struct {
	mode    string
	code    string
	hasCode bool
	// local is `-L`: the emulation, and every option moved after it, last
	// only as long as the function it stands in.
	local bool
	// options are the `-o name` and `+o name` pairs, in the order written —
	// order matters, because the same name may appear twice.
	options []emulateOption
}

// emulateOption is one `{+|-}o name`.
type emulateOption struct {
	name string
	on   bool
}

// apply sets the options this call names, after the emulation has placed its
// own defaults. A name the option table does not have is refused and the rest
// are still applied, which is what `setopt` does with a list.
func (e emulateCall) applyOptions(r *interp.Runner) int {
	status := 0
	for _, o := range e.options {
		if code := setOption(r, o.name, o.on); code != 0 {
			status = code
		}
	}
	return status
}
