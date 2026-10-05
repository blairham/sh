// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
// Measured shapes: a bare `emulate` prints the current mode and 0; every
// mode word names an emulation, by its first letter (see emulationForWord);
// a second operand is
// `unknown argument`, 1; `-c code` runs the code under the emulation and
// then restores everything, options included, reporting the code's status.

// currentEmulation reads the mode, which lives in
// interp.Runner.DialectOptions beside the recorded options. It was a
// parameter under a name no script can reach until #6100, for the reason it
// is on the runner now: a subshell must keep its own, and the runner is
// copied by value into one. Empty until something emulates, which is zsh's
// own mode.
func currentEmulation(r *interp.Runner) string {
	if m := r.DialectOptions.Mode; m != "" {
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
//
// `lastPipeHere` is the fifth, and the one `B07emulate` ends on: whether the
// last element of a pipeline runs in this shell, which is
// interp.Semantics.LastPipelineElementInCurrentShell. zsh's own mode, ksh and
// csh run it here and **sh alone runs it in a subshell**, so `echo | x=2`
// leaves `x` where it was only under sh. Measured 2026-09-30 on zsh 5.9.2,
// `-f`, with an assignment, `read`, a function and a brace group as the last
// element — four for four in each mode — under `emulate MODE -c`, a bare
// `emulate sh`, a sticky sh function and argv[0] `sh`.
//
// **No option name moves it**, which is what makes it an axis of the mode
// rather than a row of the option table: every option `emulate sh` and
// `emulate zsh` disagree about was set to the other mode's value one at a
// time, inside each mode, and none moved the answer — the same probe, left
// alone, reads the subshell under sh and the current shell under zsh.
//
// `infNaN` is the sixth, and E03posix's (#5157): whether arithmetic reads the
// names `inf` and `nan` as the two floating constants, which is
// interp.Semantics.ArithInfAndNaNAreConstants. zsh's own mode, ksh and csh do
// and **sh alone reads them as parameters**, so `inf=42; echo $((inf))` is
// `42` only under sh and `Inf` in the other three. Measured 2026-10-02 on zsh
// 5.9.2, `-f`, under a bare `emulate MODE`, an argv[0] of `sh`, a sticky sh
// function, `emulate -L sh` and `emulate -l sh` — the last moving it exactly
// as it moves `lastPipeHere`. No option moves it: under `emulate sh`, every
// name `emulate -l zsh` lists set to zsh's value leaves `$((inf))` at `42`.
var emulations = map[string]struct {
	cdNowhere     bool
	fillsHome     bool
	declaredEmpty bool
	lastPipeHere  bool
	infNaN        bool
}{
	"zsh": {cdNowhere: false, fillsHome: true, declaredEmpty: true, lastPipeHere: true, infNaN: true},
	"sh":  {cdNowhere: true, fillsHome: false, declaredEmpty: false, lastPipeHere: false, infNaN: false},
	"ksh": {cdNowhere: true, fillsHome: false, declaredEmpty: false, lastPipeHere: true, infNaN: true},
	"csh": {cdNowhere: true, fillsHome: false, declaredEmpty: true, lastPipeHere: true, infNaN: true},
}

// applyEmulation switches the axes and puts back the options this form of
// the emulation resets. Which those are is measured and is not the whole
// table — emulateoptions.go holds the partition and how it was taken.
//
// strict is the `-R` form, which widens the set from 81 names to 176 and is
// the only thing the letter does here.
func applyEmulation(r *interp.Runner, mode string, strict bool) {
	em := emulations[mode]
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
	}, answer(em.cdNowhere))
	// The second, and the one that has to be in place before the shell reads
	// its `HOME` for the first time: a startup that seeds one is a startup,
	// and an emulation taken from argv[0] is applied before anything runs.
	// See interp/shellhome.go.
	setAxis(r, func(s *interp.Semantics) *interp.Answer {
		return &s.StartupFillsAnAbsentHome
	}, answer(em.fillsHome))
	// The fourth, and the only one of them a script meets on an ordinary
	// line rather than at a boundary: whether a declaration with no value on
	// it gives the name one. See the table above for the measurement and for
	// the control that keeps it about the declaration and not about the name.
	setAxis(r, func(s *interp.Semantics) *interp.Answer {
		return &s.DeclaredNameWithoutValueIsEmpty
	}, answer(em.declaredEmpty))
	// The fifth: where a pipeline's last element runs. See the table.
	setAxis(r, func(s *interp.Semantics) *interp.Answer {
		return &s.LastPipelineElementInCurrentShell
	}, answer(em.lastPipeHere))
	// The sixth: whether `inf` and `nan` are constants. See the table.
	setAxis(r, func(s *interp.Semantics) *interp.Answer {
		return &s.ArithInfAndNaNAreConstants
	}, answer(em.infNaN))
	// The option table, from a plan compiled once per mode — see
	// emulationplan.go, which says what each part of the plan is and why it
	// is the same answer the per-name walk gave.
	planFor(mode, strict).apply(r)
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
	r.DialectOptions.Mode = mode
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
	e.mode = emulationForWord(r, e.mode)
	if e.list {
		listEmulation(r, e.mode, e.strict, e.local)
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
	// Whether the string's trace keeps out of this command's redirection is
	// decided by tracing as it stood when the command was reached, before
	// the emulation and its option words move it (#5260) — see
	// interp.Runner.HoldTracingForBorrowedText.
	defer r.HoldTracingForBorrowedText()()
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

// listEmulation is `emulate -l`: it prints what the emulation would set rather
// than setting it (#5249). Measured on zsh 5.9.2, 2026-09-30:
//
//   - One name a line, sorted, each as **the value the emulation sets**, with
//     `no` in front where that is off. The shell's own state does not show:
//     with `setopt nullglob`, `emulate -l zsh` still prints `nonullglob`.
//   - The names are the ones the emulation resets — the 81 bare, and under
//     `-R` the 95 more and `exec`, 177 lines. `exec` is the one `-R` lists and
//     that emulateoptions.go keeps among the nine no emulation touches, since
//     no probe from inside the shell can see it moved.
//   - `-L` lists `localoptions`, `localpatterns` and `localtraps` on, which is
//     what `emulate -L` turns on after the emulation.
//
// And it **switches the mode word and nothing else**: after `emulate csh;
// emulate -l sh`, `emulate` prints `sh`, while no option has moved —
// `shwordsplit` still off, a `nullglob` set before it still set — and neither
// has `cd` with no HOME or a value-less `typeset`. What does move is what
// follows the mode word rather than an option: the last element of a pipeline
// runs in a subshell afterwards, and the bare `setopt` listing is taken
// against sh's defaults.
func listEmulation(r *interp.Runner, mode string, strict, local bool) {
	r.DialectOptions.Mode = mode
	setAxis(r, func(s *interp.Semantics) *interp.Answer {
		return &s.LastPipelineElementInCurrentShell
	}, answer(emulations[mode].lastPipeHere))
	setAxis(r, func(s *interp.Semantics) *interp.Answer {
		return &s.ArithInfAndNaNAreConstants
	}, answer(emulations[mode].infNaN))
	names := make([]string, 0, len(zshOptions))
	byName := make(map[string]zshOption, len(zshOptions))
	for _, o := range zshOptions {
		if resetByEmulation(o.base, strict) || (strict && o.base == "exec") {
			names = append(names, o.base)
			byName[o.base] = o
		}
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		on := emulationDefault(byName[n], mode)
		if local && (n == "localoptions" || n == "localpatterns" || n == "localtraps") {
			on = true
		}
		if !on {
			b.WriteString("no")
		}
		b.WriteString(n)
		b.WriteByte('\n')
	}
	_, _ = fmt.Fprint(r.Out(), b.String())
}

// emulationForWord reads a mode word the way zsh does: **every word names an
// emulation** (#5254). One leading `r` is dropped, and then the first letter
// of what is left decides — `s` and `b` are sh, `k` is ksh, `c` is csh, and
// anything else, the empty word included, is zsh. Case-sensitive, and not a
// path: no basename is taken.
//
// Measured on zsh 5.9.2, 2026-09-30, from **csh** — the probe that said "an
// unknown word leaves the mode unchanged" was run from zsh, where unchanged
// and zsh print the same thing. From csh, `emulate fish`, `emulate BASH`,
// `emulate ”`, `emulate r` and `emulate /bin/sh` report `zsh`; `b`, `bash`,
// `bfoo`, `rsh`, `'s h'` report `sh`; `k`, `ksh93`, `rk` report `ksh`; `cfoo`
// and `rcsh` report `csh`. And `emulate fish -c '…'` runs the code, under
// zsh.
//
// It is the rule argv[0] is read by, so the letters are read from the same
// table the front end reads it from, interp.EmulationOption, rather than
// written twice; the builtin's differences are that it takes the word whole
// and that no letter leaves it unnamed. `--emulate` hands its word here,
// which is what makes `zsh --emulate bash` an sh.
func emulationForWord(r *interp.Runner, word string) string {
	opt := r.Semantics.EmulationOption
	if word != "" && strings.IndexByte(opt.NameDropsInitial, word[0]) >= 0 {
		word = word[1:]
	}
	if word != "" {
		for _, pair := range strings.Fields(opt.NameInitials) {
			letter, mode, ok := strings.Cut(pair, "=")
			if ok && len(letter) == 1 && letter[0] == word[0] {
				if _, known := emulations[mode]; known {
					return mode
				}
			}
		}
	}
	return "zsh"
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
	// Whether the flags before the mode are over. `--` ends them, which this
	// builtin refused outright until the invocation option needed it:
	// `--emulate` takes its next word unconditionally, so the front end hands
	// the word over behind a `--` rather than letting the builtin read `-c`
	// or `--` as options of its own. Measured in the same run: `emulate --
	// sh` is sh, `emulate -- -L` and `emulate -- --` are mode words — zsh,
	// by their first letter (#5254) — and `emulate sh --` is sh.
	//
	// It ends the flags *before* the mode and nothing more: after the mode
	// the options are read afresh, measured 2026-09-30 — `emulate -- sh -c
	// 'print ran'` runs, and `emulate -- sh -o nullglob` sets it.
	endOfFlags := false
	// Whether the options after the mode are over, and whether a `-c` among
	// them is waiting for its string (#5250). zsh reads those options the way
	// `set` does: `-c` only *marks* the call, and the string it runs is the
	// first word left once the options are over, not the word after the
	// letter. Measured on zsh 5.9.2, 2026-09-30:
	//
	//   - `emulate zsh -c 'print ran' -o nullglob` is `unknown argument -o`:
	//     the string ended the options, and the next word has nowhere to go.
	//   - `emulate zsh -c -o nullglob 'print ran'` runs with nullglob on, and
	//     `-co nullglob 'print ran'` does too — the `o` took the word the `c`
	//     did not.
	//   - `emulate zsh -c -c 'print ran'` runs once.
	//   - `-`, `--` and a bare `+` each end the options and are consumed:
	//     `emulate zsh -c - 'print ran'` runs, `emulate zsh -c --` is
	//     `string expected after -c`, and `emulate zsh - -c 'x'` is `unknown
	//     argument -c`.
	optionsOver := false
	wantCode := false
	// Whether `-L` was written before the mode, which is the letter a `-c`
	// cannot take — see the end of this function. After the mode `-L` is
	// another letter's business (#5248).
	localBeforeMode := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if e.hasMode && e.list {
			// Under `-l` the mode is the last word, and anything after it is
			// refused before it is looked at — `emulate -l sh -o bad` is
			// this and not `no such option`, measured (#5249).
			r.Diagnosef("too many arguments for -l\n")
			return e, 1
		}
		if e.hasMode {
			if !optionsOver {
				switch {
				case a == "-" || a == "--" || a == "+":
					optionsOver = true
					continue
				case len(a) > 1 && (a[0] == '-' || a[0] == '+'):
					var ok, ends bool
					i, ok, ends = e.readOptionWord(r, args, i, &wantCode)
					if !ok {
						return e, 1
					}
					sawFlags = true
					if ends {
						optionsOver = true
					}
					continue
				}
				optionsOver = true
			}
			if wantCode && !e.hasCode {
				e.code, e.hasCode = a, true
				continue
			}
			r.Diagnosef("unknown argument %s\n", a)
			return e, 1
		}
		switch {
		case endOfFlags:
			// Every word after `--` is the mode, whatever it starts with.
		case a == "--":
			endOfFlags = true
			continue
		case a == "-":
			// An option word with nothing in it. Not a mode — `emulate -`
			// prints the current one and `emulate - sh` is sh — and not a
			// flag either, which is why it does not set sawFlags. `+` alone
			// is *not* this: measured, `emulate +` is silent at 0, which is a
			// mode word — zsh, by its first letter (#5254) — rather than a
			// flag word.
			continue
		case len(a) > 1 && a[0] == '-':
			for _, letter := range a[1:] {
				switch letter {
				case 'l':
					// The listing (#5249). Not a flag the count check below
					// counts: `emulate -l` alone prints the current mode like
					// a bare `emulate`, where `emulate -lR` and `emulate -lL`
					// are `not enough arguments`. Measured on zsh 5.9.2,
					// 2026-09-30.
					e.list = true
					continue
				}
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
					localBeforeMode = true
				default:
					// Everything else, `-o` and `-c` included (#5247): before
					// the mode word neither is a letter of this builtin, and
					// the letter is refused as it is read, before the word it
					// would take is looked at. Measured on zsh 5.9.2,
					// 2026-09-30: `emulate -o nullglob zsh`, `emulate -o`,
					// `emulate -Ro sh` and `-L -o nullglob sh` are each `bad
					// option: -o` at 1 with the mode unchanged, and `emulate
					// -c 'print ran' sh` is `bad option: -c` without running
					// anything.
					r.Diagnosef("bad option: -%c\n", letter)
					return e, 1
				}
			}
			continue
		}
		// The mode word. Before it a word starting with `+` is not a flag
		// word at all but this word itself (#5247). Measured on zsh 5.9.2,
		// 2026-09-30: `emulate +o nullglob zsh` is `unknown argument
		// nullglob` — the operand after the mode `+o`, which names no
		// emulation of its own — and a bare `emulate +o` is zsh, by its first
		// letter (#5254). `+R sh`, `+L sh` and `+c 'print ran' sh` answer the same way,
		// and `-L +L zsh` in a function is `unknown argument zsh`.
		//
		// Written and empty is a mode like any other, which is measured
		// rather than assumed: `emulate ""` is silent at 0 with the mode
		// unchanged, and `emulate "" sh` is `unknown argument sh` — so the
		// empty word occupied the operand a second one would have wanted.
		// A pair rather than a non-empty string, because "no mode" and "the
		// empty mode" are two states and only one of them prints.
		e.mode, e.hasMode = a, true
	}
	if !e.hasMode && sawFlags {
		// Flags with nothing to emulate, which is what real zsh says before
		// looking at the flags themselves.
		r.Diagnosef("not enough arguments\n")
		return e, 1
	}
	if wantCode && !e.hasCode {
		// Spelled with `-` whichever form marked it: `emulate zsh +c` is
		// `string expected after -c`, measured.
		r.Diagnosef("string expected after -c\n")
		return e, 1
	}
	if e.hasCode && localBeforeMode {
		// The last thing said, after every refusal above: `emulate -L sh -c`
		// is still `string expected after -c` and `emulate -L sh -c 'x'
		// extra` still `unknown argument extra`. Measured on zsh 5.9.2,
		// 2026-09-30, with `-LR` and with `+c` alike, and with nothing run.
		r.Diagnosef("option -L incompatible with -c\n")
		return e, 1
	}
	return e, -1
}

// readOptionWord reads one option word after the mode, args[i], the way
// `set` reads one, and returns the index of the last word it used and whether
// the word ended the options.
//
// `-o` and `+o` take the rest of their word as the name when there is one
// and the next word when there is not — measured, `emulate zsh -onullglob
// -c '…'` sets nullglob, `-oc` is `no such option: c`, and `-o -c` is `no
// such option: -c`. A missing name is `string expected after -o` in either
// form. `c` marks the call for a string (see emulateArguments).
//
// Every other letter is an **option letter**, the way `set` reads one
// (#5248), and it is set after the emulation exactly as an `-o` name is —
// see emulateLetter for which letters, and what they name.
func (e *emulateCall) readOptionWord(r *interp.Runner, args []string, i int, wantCode *bool) (int, bool, bool) {
	a := args[i]
	on := a[0] == '-'
	ends := false
	for j := 1; j < len(a); j++ {
		letter := rune(a[j])
		switch letter {
		case 'o':
			name := a[j+1:]
			if name == "" {
				if i+1 >= len(args) {
					r.Diagnosef("string expected after -o\n")
					return i, false, false
				}
				i++
				name = args[i]
			}
			if !knownEmulateOption(r, name) {
				return i, false, false
			}
			e.options = append(e.options, emulateOption{name: name, on: on})
			return i, true, ends
		case 'c':
			*wantCode = true
			continue
		}
		name, kind := emulateLetter(r, letter)
		switch kind {
		case letterEndsOptions:
			ends = true
		case letterBad:
			r.Diagnosef("bad option: -%c\n", letter)
			return i, false, false
		case letterFixed:
			if on {
				r.Diagnosef("can't change option: -%c\n", letter)
			}
		default:
			e.options = append(e.options, emulateOption{name: name, on: on})
		}
	}
	return i, true, ends
}

// letterKind is what one option letter after the mode word is.
type letterKind uint8

const (
	letterOption      letterKind = iota // names an option, set like `-o name`
	letterBad                           // `bad option: -X`, and the call stops
	letterFixed                         // `can't change option: -X`, and nothing moves
	letterEndsOptions                   // `-b`: the options end with this word
)

// emulateLetter reads one option letter after the mode word (#5248).
//
// They are `set`'s letters, **from the table the caller's shell reads `set`
// by**: zsh's own, or sh's while `shoptionletters` is on. Measured on zsh
// 5.9.2, 2026-09-30, one letter at a time in both directions against a plain
// emulation, under both tables:
//
//   - zsh's table: every letter of setLetterOptions and the eight every shell
//     shares (`a e m n u v x C`) — so `-L` is `sunkeyboardhack`, `-R`
//     `longlistjobs`, `-N` `autopushd`, `-G` `nullglob`, `-X` `listtypes`,
//     and the digits theirs. `-b` is not a letter here: it **ends the
//     options at the end of its word**, so `-Gb` sets nullglob and `-b -G` is
//     `unknown argument -G`. `-j`, `-q`, `-z` and `-A` are `bad option`.
//   - sh's: shLetterOptions and the same eight, and `-b` is `notify` — `+b`
//     turns it off. Everything else, the digits and `-Z` included, is `bad
//     option`: `emulate sh; emulate zsh -G` is refused, because the table is
//     the caller's and not the mode being entered.
//   - `-i`, `-m`, `-s`, `-t` and (zsh's table) `-Z` are `can't change
//     option: -X` at status 0: said, nothing moves, and the call goes on —
//     `emulate zsh -Z -c 'print ran'` runs. The `+` form of each is silent.
//     `-m` and `-Z` were measured in a shell with no terminal, where the
//     options behind them cannot move; in one with a terminal they read as
//     the options do.
//
// A letter joins the sticky identity as the name it abbreviates: measured,
// `-G` and `-o nullglob` are one emulation, and `-F` is `+o glob`.
func emulateLetter(r *interp.Runner, letter rune) (string, letterKind) {
	shLetters := zshOptions[shOptionLettersIndex].get(r)
	switch letter {
	case 'b':
		if shLetters {
			return "notify", letterOption
		}
		return "", letterEndsOptions
	case 'i', 's', 't':
		return "", letterFixed
	case 'm':
		if !r.Interactive {
			return "", letterFixed
		}
		return "monitor", letterOption
	case 'Z':
		if shLetters {
			return "", letterBad
		}
		if !r.Interactive {
			return "", letterFixed
		}
		return "zle", letterOption
	}
	if name, ok := emulateSharedLetters[letter]; ok {
		return name, letterOption
	}
	table := setLetterOptions
	if shLetters {
		table = shLetterOptions
	}
	if name, ok := table[letter]; ok && name != "" {
		return name, letterOption
	}
	return "", letterBad
}

// emulateSharedLetters are the letters every shell spells alike, which the
// substrate's own table answers for `set` and this builtin has to answer
// itself.
var emulateSharedLetters = map[rune]string{
	'a': "allexport",
	'e': "errexit",
	'n': "noexec",
	'u': "nounset",
	'v': "verbose",
	'x': "xtrace",
	'C': "noclobber",
}

// knownEmulateOption refuses an `-o` or `+o` name no option answers to, at
// the point the command line is read rather than when the options are set.
//
// Measured on zsh 5.9.2, 2026-09-30 (#5144): a bad name is `no such option:
// NAME` at status 1, and it is the first thing said. It comes before an
// operand the mode has no room for (`emulate zsh -o bad 'print x'`), before
// an `-o` or `-c` left with no word after it, and before the mode word is
// read. And it stops the whole call: the emulation is not entered, no
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
	// list is `-l`: print the options this emulation would set instead of
	// setting them. See listEmulation.
	list bool
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
