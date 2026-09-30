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
// `emulate -L`, the function-local form, is `setopt localoptions localtraps
// localpatterns` after the emulation and nothing else — measured, and it is
// three options rather than two. The count in this sentence has been wrong
// twice: it said one, was corrected to two, and stopped one short, because
// each correction was taken from a probe that listed what the letter moved
// rather than from a listing taken *inside* the scope it opens. `f(){ emulate
// -L zsh; setopt }; f` is that listing, and it prints `localpatterns` between
// `localoptions` and `localtraps` in the reference (#4530).
//
// The third name has nothing to scope in this shell, which is worth saying
// rather than leaving to be discovered: `localpatterns` restores the pattern
// disables `disable -p` makes, and `disable` here keeps only the builtins
// table — see enable.go, where every other table is refused out loud. So what
// the letter owes today is the option's *state*, which is read by the
// listings, by `[[ -o localpatterns ]]` and by `$options`; the scope arrives
// with the table it scopes.
//
// It does **not** narrow or widen the reset: the 81 names a
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
// It held five fields until #2549, then one, and now none of the original
// five. Four of them — `shwordsplit`, `nomatch`, `ksharrays` and
// `posixbuiltins` — became names in the option table, which knows each
// emulation's own default for every name it holds, so a second copy here
// could only drift from it.
//
// **`redirFatal` was the fifth, and it went the same way in #4436.** It said
// `emulate sh` and `emulate ksh` make a failed redirection on a special
// builtin end the script where `emulate zsh` leaves it a complaint the
// script runs past — true, and keyed on the wrong thing. The sentence that
// justified keeping it here was *"zsh spells it with no option, so nothing
// in the table can carry it"*, and that is false: `posixbuiltins` is the
// option, and it carries this exactly as it carries the four other axes it
// moves — including the failed `.` beside it, which is the same rule about
// the same kind of builtin and stops the same suite file two chunks later.
//
// Four emulations agreeing is what hid it, because each mode's
// `posixbuiltins` default happens to equal its old `redirFatal` value in all
// four rows — breadth along an axis that was never the key. The pair that
// separates them holds the emulation fixed and moves only the option, and
// was measured 2026-09-29 on zsh 5.9.2:
//
//	emulate sh                           the script ends, 1
//	emulate sh; unsetopt posixbuiltins   `after`, 0
//	emulate csh                          `after`, 0
//	emulate csh; setopt posixbuiltins    the script ends, 1
//
// So the mode was never the key; the option was, and the mode only set it.
// See the `posixbuiltins` entry in setopt.go.
//
// `cdNowhere` is the second, and it arrived with the *name* (#4640). `cd`
// with no operand and no `HOME` writes `HOME not set` and exits 1 under
// `sh`, `ksh` and `csh`, and is a silent 0 under `zsh`. Measured 2026-09-26
// on zsh 5.9.2, `env -u HOME`, both by copying the reference to a file with
// each name and through `--emulate` on the reference under its own name —
// two routes to the same mode agreeing, which is what says the mode carries
// it rather than the name.
//
// csh parts from zsh here where it agreed with it on the redirection rule
// that used to sit beside this, which is the reason this is a field of its
// own rather than a reading of an sh-family boolean: `csh` is not "the mode
// that changes nothing this shell can speak about" on every axis, and one
// boolean standing for several would have made it so.
//
// What this field does **not** model is a shell that once had a `HOME` and
// unset it. Measured in the same run: with the reference called `sh` and
// `HOME` in its environment, `unset HOME; cd` is a silent 0, and so it is
// after `HOME=/tmp; unset HOME` in a shell that started with none — only a
// shell that has never had one says `HOME not set`. That is a home the shell
// remembers rather than an axis of the emulation, it is the same answer under
// every mode, and it is Semantics.CdRemembersAHomeThatWasUnset on the preset.
//
// `fillsHome` is the third, and it is the half of #4654 that *is* the mode.
// A shell whose environment has no `HOME` at all seeds one from the password
// entry of the user the process runs as under `zsh` and seeds nothing under
// `sh`, `ksh` or `csh`. Measured 2026-09-26 the same two ways — the reference
// copied to a file with each name, and `--emulate` on the reference under its
// own name — with `env -u HOME <shell> -c 'print -r -- $HOME'` writing the
// password entry in one and nothing in the other three.
//
// It is also why the three sh-family rows of `cdNowhere` are reachable at all:
// under `zsh` there is no shell without a `HOME` for that field to answer
// about, because this one has already filled it in. The two fields are one
// shell read at two moments and they have to move together, which is what a
// second boolean here says and a reading of the first could not.
//
// `declaredEmpty` is the fourth, and it is the half of #4654 that #4753 was
// filed for. A declaration carrying no value — `typeset X`, `declare X`,
// `integer N`, `local X` — gives the name an empty *value* under `zsh` and
// leaves it declared and unset under `sh` and `ksh`, so `${X+set}` answers
// `set` in the first and nothing in the other two.
//
// Measured 2026-09-26 on zsh 5.9.2 (aarch64-apple-darwin25.4.0), run `-f`,
// both ways round: the reference copied to files called `zsh`, `sh` and
// `ksh` — which is the whole of what differs, and the first letter of argv[0]
// is what it reads — and `emulate MODE` under its own name, plain and `-R`,
// which agree. `typeset X; printf '%s' "${X+set}"` is `set`, nothing and
// nothing across the three, and `typeset X=; printf '%s' "${X+set}"` is `set`
// under all of them, which is the control that stops the reading being "a
// declaration never creates the name".
//
// **The name does not matter and the mode does**, which is what makes it an
// axis of the emulation rather than a fact about `HOME`: the row was noticed
// as `typeset HOME; unset HOME; cd`, where the `unset` here finds a value to
// remove that the reference never created.
//
// csh is on zsh's side of this one and against it on `cdNowhere` — measured
// in the same run, `emulate -R csh` answers `set`. A single "is this an
// sh-family mode" boolean standing for all of them would be wrong about two.
var emulations = map[string]struct {
	cdNowhere     bool
	fillsHome     bool
	declaredEmpty bool
}{
	"zsh": {cdNowhere: false, fillsHome: true, declaredEmpty: true},
	"sh":  {cdNowhere: true, fillsHome: false, declaredEmpty: false},
	"ksh": {cdNowhere: true, fillsHome: false, declaredEmpty: false},
	"csh": {cdNowhere: true, fillsHome: false, declaredEmpty: true},
}

// applyEmulation switches the axes and puts back the options this form of
// the emulation resets. Which those are is measured and is not the whole
// table — emulateoptions.go holds the partition and how it was taken.
//
// strict is the `-R` form, which widens the set from 81 names to 176 and is
// the only thing the letter does here.
func applyEmulation(r *interp.Runner, mode string, strict bool) {
	// The first of the axes with no option name over it. The five that used
	// to be swapped here — `shwordsplit`, `nomatch`, `ksharrays`,
	// `posixbuiltins` and the redirection rule `posixbuiltins` carries — are
	// ordinary rows of the option table now, because the table knows each
	// emulation's own default for them and the swap knew only sh-ness. See
	// emulationDefaults.
	//
	// `cd` with no operand and no `HOME` writes `HOME not set` and exits 1
	// in the three sh-family modes and is a silent 0 in zsh's own, which is
	// why a binary called `sh` has to reach it: nothing else in this shell
	// moves with the name.
	setAxis(r, func(s *interp.Semantics) *interp.Answer {
		return &s.CdWithoutHomeIsAnError
	}, answer(emulations[mode].cdNowhere))
	// The second, and the one that has to be in place before the shell reads
	// its `HOME` for the first time: a startup that seeds one is a startup,
	// and an emulation taken from argv[0] is applied before anything runs.
	// See interp/shellhome.go.
	setAxis(r, func(s *interp.Semantics) *interp.Answer {
		return &s.StartupFillsAnAbsentHome
	}, answer(emulations[mode].fillsHome))
	// The fourth, and the only one of them a script meets on an ordinary
	// line rather than at a boundary: whether a declaration with no value on
	// it gives the name one. See the table above for the measurement and for
	// the control that keeps it about the declaration and not about the name.
	setAxis(r, func(s *interp.Semantics) *interp.Answer {
		return &s.DeclaredNameWithoutValueIsEmpty
	}, answer(emulations[mode].declaredEmpty))
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
		// A name whose base state is not the table's default needs the
		// deviation *written* rather than dropped, and it is the one case
		// where dropping is wrong in both directions. The store is read
		// against the base, so a dropped `rcs` in a `zsh -f` shell reads off
		// — the invocation's answer, which is exactly what the emulation was
		// asked to undo.
		//
		// Measured on zsh 5.9.2, 2026-09-25, `zsh +Z -f -c`: the four
		// recordedOver names read `rcs` off, `hashdirs` off, `login` off and
		// `zle` off in that shell, and after `emulate -R zsh` — or `-R sh`,
		// `-R ksh`, `-R csh`, which agree — `rcs` and `hashdirs` read **on**
		// while `login` and `zle` stay off. The last two are the control:
		// both are in emulationNeverReset, so neither reaches this branch,
		// and a change that put every recordedOver name back would be wrong
		// about them. A bare `emulate sh` leaves all four, since the two
		// that move are in emulationStrictReset rather than in the 81.
		if o.over != nil {
			if emulationDefault(o, mode) != o.over(r) {
				kept = append(kept, o.base)
			}
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
	// And the grammar, which is the part of an emulation that reaches how the
	// *next line is read* rather than what a line already read means.
	//
	// After the option loop rather than before it, although the two cannot
	// collide: emulategrammar.go may not name a `syntax.Dialect` field an
	// option name already owns, since a field with two writers in one call
	// answers to whichever ran last. Last is still the right place for it —
	// this is the emulation's own answer, and a later addition that did
	// overlap would then be visible as an option that stopped taking effect
	// rather than as a grammar that intermittently did.
	setEmulationGrammar(r, mode)
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
			// Three names and not one, and not two. Measured on zsh 5.9.2
			// from a listing taken inside the scope — `f(){ emulate -L zsh;
			// setopt }; f` — `emulate -L zsh` leaves `localoptions`,
			// `localpatterns` and `localtraps` on and `localloops` off. So
			// the trap the letter scopes is the trap-side option working,
			// not the option table's rule reaching further than it does.
			//
			// After the emulation rather than before it, because a plain
			// emulation resets every option to that emulation's default and
			// both of these default off — which is exactly why a bare
			// `emulate sh` in a function does *not* localize, measured.
			setLocalOptions(r, true)
			setLocalPatterns(r, true)
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
		// Every function the code defines is **sticky**: the emulation is
		// entered again whenever that function is later called. The mark is
		// taken at the definition rather than from the set of names this
		// text left behind, because a redefinition adds no name and a
		// function redefined *outside* an emulation loses the mark — see
		// sticky.go, where both rows are measured.
		defer enterSticky(r, stickyWord(e.mode, e.strict, e.options))()
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
						if !knownEmulateOption(r, args[i]) {
							return e, 1
						}
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
					if !knownEmulateOption(r, args[i]) {
						return e, 1
					}
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

// knownEmulateOption refuses an `-o` or `+o` name no option answers to, at
// the point the command line is read rather than when the options are set.
//
// Measured on zsh 5.9.2, 2026-09-30 (#5144): a bad name is `no such option:
// NAME` at status 1, and it is the first thing said. It comes before an
// operand the mode has no room for (`emulate zsh -o bad 'print x'`), before
// an `-o` or `-c` left with no word after it, and before an unknown mode is
// passed over. And it stops the whole call: the emulation is not entered, no
// other option named beside it is set — `-o nullglob -o bad` leaves nullglob
// off — and a `-c` string does not run.
//
// Left to right, as the words are read: a bad name *after* a surplus operand
// is not reached, and the operand is what is reported.
func knownEmulateOption(r *interp.Runner, name string) bool {
	if _, _, ok := resolveOptionName(normalizeOption(name)); ok {
		return true
	}
	r.Diagnosef("no such option: %s\n", name)
	return false
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
