// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"sort"

	"github.com/blairham/sh/interp"
)

// `emulate` switches this shell between zsh's semantics and the sh family's,
// which zsh scripts do at the top of anything meant to travel.
//
// What one emulation means against another was measured probe by probe rather
// than assumed, and three axes carry all of it that this shell distinguishes:
// `emulate sh` and `emulate ksh` split unquoted expansions (shwordsplit),
// pass an unmatched glob through as itself (nonomatch), and base arrays at
// zero (ksharrays); `emulate zsh` puts all three back. The switch also puts
// options back to the emulation's defaults — measured: `setopt err_exit;
// emulate zsh` turns errexit back off, no `-R` required — and the
// bare-listing baseline moves with it, which the option table's `def` field
// records.
//
// **Which** options is the part this file used to get wrong, and it is
// three-valued rather than two. A bare `emulate` resets 81 of the 185 names
// and leaves the other 104 exactly where the script left them; `emulate -R`
// widens that to every name but the nine describing how the shell was
// started. Until #2515 a bare emulation reset the whole table, which turned
// `setopt nopromptsp; emulate sh` back on, dropped a `histignorespace` a
// session had asked for, and silently ended an `xtrace`. emulateoptions.go
// holds the partition, how it was measured, and what about an emulation is
// still not modeled.
//
// The rest of what real zsh folds into an emulation — its ~180 options, csh's
// separate glob wording, `$0`-versus-function-name rules — is not modeled;
// `emulate csh` here records the mode and changes nothing.
// docs/spec/semantics.md records the boundary.
//
// `emulate -L`, the function-local form, is `setopt localoptions localtraps`
// after the emulation and nothing else — measured, and it is two options
// rather than one. It does **not** narrow or widen the reset: the 81 names a
// bare `emulate sh` puts back are the same 81 `emulate -L sh` puts back, and
// `-L -R` together are the strict set scoped to the call. So `emulate -L sh`
// leaves an `xtrace` running where `emulate -LR sh` stops it, which is the
// other way round from a note in #2126 written before this was measured. It had a save-and-restore of its own once, and that was
// two mistakes: it saved at its own line rather than at the function entry,
// so an option moved earlier in the same body leaked, and it restored whether
// or not the option was still on at the return. Both are measured the other
// way; localoptions.go carries the rule now, localtraps.go carries the other,
// and every spelling reaches them.
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

// emulations is the modes this shell knows, and the one axis an emulation
// moves that has no option name over it.
//
// It held five fields until #2549 and holds one. Four of them —
// `shwordsplit`, `nomatch`, `ksharrays` and `posixbuiltins` — are names in
// the option table, and the table now knows each emulation's own default for
// every name it holds, so a second copy here could only drift from it. What
// is left is `redirFatal`: `emulate sh` and `emulate ksh` make a failed
// redirection on a special builtin end the script where `emulate zsh` leaves
// it a complaint the script runs past. That is the same switch bash's
// `set -o posix` throws, measured in a second binary, which is what says the
// axis belongs to the mode rather than to either shell — and zsh spells it
// with no option, so nothing in the table can carry it.
//
// csh's value is zsh's, which is measured rather than assumed: the four names
// above all read the same under `emulate -R csh` as under `emulate -R zsh`,
// so the mode that "changes nothing this shell can speak about" is a mode
// that agrees with zsh rather than a mode to skip.
var emulations = map[string]struct{ redirFatal bool }{
	"zsh": {redirFatal: false},
	"sh":  {redirFatal: true},
	"ksh": {redirFatal: true},
	"csh": {redirFatal: false},
}

// applyEmulation switches the axes and puts back the options this form of
// the emulation resets. Which those are is measured and is not the whole
// table — emulateoptions.go holds the partition and how it was taken.
//
// strict is the `-R` form, which widens the set from 81 names to 176 and is
// the only thing the letter does here.
func applyEmulation(r *interp.Runner, mode string, strict bool) {
	// The one axis with no option name over it, so the option table cannot
	// carry it and this is where it is placed. The other four this used to
	// swap here — `shwordsplit`, `nomatch`, `ksharrays` and `posixbuiltins` —
	// are ordinary rows of the table now, because the table knows each
	// emulation's own default for them and the swap knew only sh-ness. See
	// emulationDefaults.
	//
	// And it runs for `csh` too. It used to be skipped there on the reading
	// that csh changes nothing this shell can speak about; measured
	// 2026-09-13, csh's value for all four of those names is zsh's, so the
	// skip and the swap agree and the skip was a special case standing for
	// nothing. Leaving it in would now mean csh alone kept whatever the
	// script had set, which is the one reading nothing measures.
	swapAxes(r, func(s *interp.Semantics) {
		s.RedirectErrorOnSpecialBuiltinFatal = answer(emulations[mode].redirFatal)
	})
	// The recorded names in one write rather than one write each. The store
	// holds deviations, so dropping a name from it is that option back at the
	// table's default — and this emulation's default is not always the
	// table's, which is what the second loop puts back in. The names this
	// emulation leaves alone stay exactly as they were.
	names, _ := r.GetArray(zshRecordedStore)
	kept := make([]string, 0, len(names))
	for _, n := range names {
		if !resetByEmulation(n, strict) {
			kept = append(kept, n)
		}
	}
	before := len(kept)
	for _, o := range zshOptions {
		if !o.recorded || !resetByEmulation(o.base, strict) {
			continue
		}
		if dev, known := emulationDeviates(o, mode); known && dev {
			kept = append(kept, o.base)
		}
	}
	if len(kept) != len(names) || before != len(kept) {
		sort.Strings(kept)
		setRecordedOptions(r, kept)
	}
	for _, o := range zshOptions {
		if o.set == nil || o.recorded || !resetByEmulation(o.base, strict) {
			continue
		}
		_ = o.set(r, emulationDefault(o, mode))
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
	if !e.hasMode {
		_, _ = fmt.Fprintf(r.Out(), "%s\n", currentEmulation(r))
		return 0
	}
	if _, known := emulations[e.mode]; !known {
		// Measured: a word naming no emulation is passed over in silence,
		// the mode unchanged — `emulate fish` and `emulate SH` alike.
		return 0
	}
	if !e.hasCode {
		applyEmulation(r, e.mode, e.strict)
		if e.local {
			// `-L` is the local-scoping options and nothing besides, which
			// is measured rather than assumed: inside `emulate -L zsh` they
			// read on, and at the top level — where there is no call to
			// return from — they stay on afterwards and localize the *next*
			// function call. So the letter is a `setopt` and the
			// function-call machinery does the rest; localoptions.go and
			// localtraps.go are where the rest is, and it is the same
			// machinery the two `setopt` names reach.
			//
			// Two names and not one. Measured on zsh 5.9.2, `emulate -L zsh`
			// leaves `localoptions`, `localtraps` and `localpatterns` on and
			// `localloops` off — so the trap the letter scopes is the
			// trap-side option working, not the option table's rule reaching
			// further than it does. `localpatterns` is not modeled here and
			// nothing sets it, which is the one of the three this letter
			// still does not carry.
			//
			// After the emulation rather than before it, because a plain
			// emulation resets every option to that emulation's default and
			// both of these default off — which is exactly why a bare
			// `emulate sh` in a function does *not* localize, measured.
			setLocalOptions(r, true)
			setLocalTraps(r, true)
		}
		return e.applyOptions(r)
	}
	// `-c` runs the string under the emulation and restores everything after
	// — measured, an option set before it comes back: `setopt no_glob;
	// emulate sh -c '…'` still refuses to glob afterwards.
	saved := saveOptionState(r)
	applyEmulation(r, e.mode, e.strict)
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
	// Whether any option *letter* has been read, which is a different
	// question from whether an option word was written and is what the
	// count check at the bottom asks. Measured 2026-09-18 on zsh 5.9.2:
	// `emulate -L` is `not enough arguments` at 1, while `emulate --` and
	// `emulate -` each print the current mode at 0 — so a word carrying no
	// letters leaves the call a bare one.
	sawFlags := false
	// And whether the operands are over. `--` ends them, which this builtin
	// refused outright until the invocation option needed it: `--emulate`
	// takes its next word unconditionally, so the front end hands the word
	// over behind a `--` rather than letting the builtin read `-c` or `--`
	// as options of its own. Measured in the same run: `emulate -- sh` is
	// sh, `emulate -- -L` and `emulate -- --` are the silence an unknown
	// mode gets, and `emulate sh --` is sh.
	endOfOptions := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case endOfOptions:
			// Every word after `--` is an operand, whatever it starts with.
		case a == "--":
			endOfOptions = true
			continue
		case a == "-":
			// An option word with nothing in it. Not a mode — `emulate -`
			// prints the current one and `emulate - sh` is sh — and not a
			// flag either, which is why it does not set sawFlags. `+` alone
			// is *not* this: measured, `emulate +` is silent and leaves the
			// mode alone, which is an unknown mode rather than a flag word.
			continue
		}
		if !endOfOptions && len(a) > 1 && a[0] == '+' {
			// The `+` form. `+o name` is `-o name` the other way round and
			// `+c` runs its string like `-c`; every other letter is
			// *accepted and does nothing*, which is measured rather than
			// assumed and is not what the `-` form does: `emulate zsh +X`
			// is 0 where `emulate -X zsh` is `bad option: -X`, and a `+L`
			// does not undo a `-L` — the emulation stays function-local.
			for j := 1; j < len(a); j++ {
				sawFlags = true
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
		if !endOfOptions && len(a) > 1 && a[0] == '-' {
			for _, letter := range a[1:] {
				sawFlags = true
				switch letter {
				case 'R':
					// The strict form, and it is not the no-op this said it
					// was until #2515: a bare emulation resets the 81
					// portability-relevant options and `-R` resets every
					// name but the nine that describe how the shell was
					// started. Measured — `setopt xtrace; emulate sh` still
					// traces and `emulate -R sh` stops.
					e.strict = true
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
		if e.hasMode {
			r.Diagnosef("unknown argument %s\n", a)
			return e, 1
		}
		// Written and empty is a mode like any other, which is measured
		// rather than assumed: `emulate ""` is silent at 0 with the mode
		// unchanged, and `emulate "" sh` is `unknown argument sh` — so the
		// empty word occupied the operand a second one would have wanted.
		// A pair rather than a non-empty string, because "no mode" and "the
		// empty mode" are two states and only one of them prints.
		e.mode, e.hasMode = a, true
	}
	if !e.hasMode && sawFlags {
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
	mode string
	// hasMode says a mode word was written, which the mode alone cannot:
	// `emulate ""` names the empty mode and `emulate` names none, and only
	// the second prints the current one. Measured 2026-09-18 on zsh 5.9.2.
	hasMode bool
	code    string
	hasCode bool
	// local is `-L`: the emulation, and every option moved after it, last
	// only as long as the function it stands in.
	local bool
	// strict is `-R`: the emulation resets the options a bare one leaves
	// where it found them. See emulateoptions.go for which those are.
	strict bool
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
