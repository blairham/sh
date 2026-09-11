// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "sort"

// The long `set -o` names, which shells have which, and what this one does
// about each.
//
// Three different questions, and conflating them is what made `set +o posix`
// — the thirteenth line of Homebrew's own `brew` script — stop the shell
// dead with `invalid option name`:
//
//   - Does this shell have the name at all? A dialect's answer. Fourteen are
//     unanimous and the rest belong to one, two or three of the panel, which
//     is why the extras are declared by each dialect rather than asked as
//     ninety separate axes.
//   - Do we implement it? Ours, and for most of them the answer is no.
//   - If we do not, which state are we already in? Also ours, and it is what
//     decides whether a request is a lie or a no-op.
//
// The last is the point. Turning off something this shell was never doing is
// a request that has been granted — `set +o posix` in a shell with no posix
// mode has left it exactly where it was asked to be. Turning *on* something
// we do not do would be a promise we cannot keep, so it is refused out loud
// rather than accepted quietly.

// setOption is what this shell does about one name.
type setOption struct {
	// apply sets it, where this shell has it for real. Nil otherwise, which
	// is most of them.
	apply func(r *Runner, on bool)

	// try is apply for the one request a dialect can refuse: `set -m` needs
	// a terminal in two of the panel. It is handed the spelling the script
	// used — `-m` or `monitor` — because one dialect's refusal echoes it
	// back, and reports whether the request was an error.
	try func(r *Runner, on bool, spelling string) bool

	// on is the state this shell is already in for a name it does not
	// implement, so that asking for that state can succeed honestly.
	//
	// Not a claim about what any *other* shell defaults to. bash has
	// `hashall` on and we do not hash at all, so ours is off and a script
	// turning it off gets what it asked for.
	on bool // get reads the live state, for the listing; nil means the static
	// `on` field is the whole answer.
	get func(*Runner) bool
}

// commonSetOptions are the names every shell in the panel has. They are the
// core's, and no dialect has to declare them.
var commonSetOptions = map[string]setOption{
	"errexit":   {apply: func(r *Runner, on bool) { r.errexit = on }, get: func(r *Runner) bool { return r.errexit }},
	"nounset":   {apply: func(r *Runner, on bool) { r.nounset = on }, get: func(r *Runner) bool { return r.nounset }},
	"xtrace":    {apply: func(r *Runner, on bool) { r.xtrace = on }, get: func(r *Runner) bool { return r.xtrace }},
	"noclobber": {apply: func(r *Runner, on bool) { r.noclobber = on }, get: func(r *Runner) bool { return r.noclobber }},
	"noglob":    {apply: func(r *Runner, on bool) { r.noglob = on }, get: func(r *Runner) bool { return r.noglob }},
	"allexport": {apply: func(r *Runner, on bool) { r.allexport = on }, get: func(r *Runner) bool { return r.allexport }},

	// noexec is one-way under both spellings: all four shells ignore turning
	// it back off, and with it on the command that would do so never runs
	// anyway.
	"noexec": {
		apply: func(r *Runner, on bool) {
			if on {
				r.noexec = true
			}
		},
		get: func(r *Runner) bool { return r.noexec },
	},
	// verbose writes input back as it is read; the echoing itself lives in
	// the front end, which holds the raw text.
	"verbose": {apply: func(r *Runner, on bool) { r.verbose = on }, get: func(r *Runner) bool { return r.verbose }},
	// monitor is the one request in the table a dialect can refuse, which
	// is why it is a try rather than an apply.
	"monitor": {try: (*Runner).setMonitor, get: func(r *Runner) bool { return r.monitor }},

	// The rest of the unanimous names, none of which this shell has yet.
	// Every one of them is off here: we do not defer a job notice and do
	// not hold the session open at end-of-file.
	"notify":    {},
	"ignoreeof": {},
	"nolog":     {},

	// The two editing modes, and they are one state under two names — see
	// Runner.editingMode and EditingMode. `emacs` starts on, because the
	// line editor really does read ^A, ^E and ^B, which is what that name
	// means; asking for `vi` selects the other keymap, which is as much as
	// this editor can promise and exactly what the zsh dialect's `bindkey
	// -v` already promises.
	//
	// Turning either *off* leaves neither on rather than swapping to the
	// other, measured in bash 5.3 and ksh93 both: `set -o vi; set +o vi`
	// reports `emacs off` and `vi off`.
	"vi": {
		apply: func(r *Runner, on bool) { r.setEditingMode(EditingModeVi, on) },
		get:   func(r *Runner) bool { return r.editingMode == EditingModeVi },
	},
	"emacs": {
		apply: func(r *Runner, on bool) { r.setEditingMode(EditingModeEmacs, on) },
		get:   func(r *Runner) bool { return r.editingMode == EditingModeEmacs },
	},
}

// EditingMode is which of `set -o emacs` and `set -o vi` is selected.
//
// Exported because a dialect's binding builtin needs it: which keymap `bind`
// or `bindkey` acts on without being told is this and nothing else, and the
// keymap *names* are the dialect's — `emacs` and `vi-insert` in one, `emacs`
// and `viins` in the other. So the core holds which mode, and each dialect
// spells it.
type EditingMode int

// The three states, and the zero value is the one every shell in the panel
// starts an interactive session in.
const (
	EditingModeEmacs EditingMode = iota
	EditingModeVi

	// EditingModeNone is what turning the selected mode off leaves behind,
	// and it is a real state rather than an absence: `set +o emacs` in bash
	// 5.3 reports `emacs off` *and* `vi off`, so a script can observe it.
	EditingModeNone
)

// EditingMode reports which editing mode is selected, for a dialect deciding
// which keymap its binding builtin acts on.
func (r *Runner) EditingMode() EditingMode { return r.editingMode }

// setEditingMode is the write half of the two option names.
//
// Turning a mode on selects it; turning one off leaves neither selected,
// which is the measurement and not a shortcut — see the table entries. Asking
// to turn off a mode that is not the selected one moves nothing, because the
// request has already been granted.
func (r *Runner) setEditingMode(mode EditingMode, on bool) {
	switch {
	case on:
		r.editingMode = mode
	case r.editingMode == mode:
		r.editingMode = EditingModeNone
	}
}

// extraSetOptions are names that belong to some shells and not others, with
// what this shell does about each. A dialect says which of them it has; this
// says what happens when it is asked for.
//
// Kept in one table rather than beside each dialect because what we do about
// a name is a fact about this implementation, not about the shell being
// imitated: `braceexpand` is on here for the same reason in all three shells
// that have the name.
var extraSetOptions = map[string]setOption{
	// We expand braces, so a script may turn that on and may not turn it off.
	"braceexpand": {on: true},
	// Comments are honored wherever they are written, which is what this
	// name asks for.
	"interactive-comments": {on: true},

	// pipefail is real — the pipeline code reads it — and its *existence* is
	// an axis older than this table (see setOption), so applying it never
	// reaches this entry. The entry is what puts it in the listings of the
	// dialects that declare the name.
	"pipefail": {get: func(r *Runner) bool { return r.pipefail }},

	// Command tracking under its two names: bash calls it hashall — and zsh
	// takes that name too — where ksh93 says trackall. One state behind
	// both, kept honestly because it is permission to cache rather than a
	// promise to; see the field.
	"hashall": {
		apply: func(r *Runner, on bool) { r.tracksCommands = on },
		get:   func(r *Runner) bool { return r.tracksCommands },
	},
	"trackall": {
		apply: func(r *Runner, on bool) { r.tracksCommands = on },
		get:   func(r *Runner) bool { return r.tracksCommands },
	},
	// zsh's histignoredups, which its `set -h` abbreviates. A script has no
	// history for it to govern; an interactive session does, and reads this
	// through the dialect's option namespace before recording a line.
	"histignoredups": {
		apply: func(r *Runner, on bool) { r.histIgnoreDups = on },
		get:   func(r *Runner) bool { return r.histIgnoreDups },
	},

	// posix is a mode rather than a single behavior, and it is the core's to
	// implement for the same reason PosixSemantics is the core's: the name
	// asks for the standard's answers, not for some shell's. A dialect still
	// says whether the shell *has* the name — one of the panel does — and
	// this says what happens when it is asked for.
	//
	// It moves the axes measured to move with it and no others, which is the
	// same partial honesty `emulate` keeps in the zsh dialect: real posix
	// modes fold in dozens of behaviors, and claiming those would be a
	// promise nothing here keeps. Today that is one axis, and the evidence
	// is direct — `set -o posix` makes a failed redirection on a special
	// builtin end bash 5.3 and bash 3.2, `set +o posix` makes bash invoked
	// as `sh` carry on, and the two states are exactly the panel's `bash`
	// and `bash-as-sh` columns.
	//
	// Turning it *off* puts back the answer the dialect started with rather
	// than writing the opposite of the standard's, which is not the same
	// thing: a shell POSIX already agrees with would lose its own answer
	// that way. A request for the state we are already in moves nothing,
	// which is what keeps `set +o posix` — the thirteenth line of Homebrew's
	// own script — the grant it has always been.
	"posix": {
		apply: func(r *Runner, on bool) { r.SetPosixMode(on) },
		get:   func(r *Runner) bool { return r.posixMode },
	},
	"errtrace":   {},
	"functrace":  {},
	"history":    {},
	"histexpand": {},
	"keyword":    {},
	// onecmd is `set -t`: the line it is set on finishes and the shell reads
	// no more. Implemented rather than recorded because the front end can
	// honestly stop — see Runner.OneCommand — and because bash takes the
	// name at 0 where we used to refuse it as not-implemented, which the
	// panel already records as neither answer
	// (opt/set-o-a-name-this-shell-has-and-will-not-move).
	//
	// zsh reaches `set -o` through its own table and refuses `onecmd` with
	// `can't change option`, its borrowed spelling for `singlecommand` being
	// one of the five it will not move. That answer is the dialect's and this
	// entry does not disturb it.
	"onecmd": {
		apply: func(r *Runner, on bool) { r.onecmd = on },
		get:   func(r *Runner) bool { return r.onecmd },
	},
	"physical":   {},
	"privileged": {},
}

// SetPosixMode enters or leaves POSIX mode, which is what the `posix` entry
// above does when a script asks for it by name and what a front end does when
// the shell was invoked under the standard's own name.
//
// Exported because the second of those has no other way in. Whether a shell
// *has* the name is a dialect's answer and only one of the panel declares it,
// so a front end reaching this through SetNamedOption would be refused by
// every dialect that spells it differently or not at all — and the shell that
// most needs the startup override is one of those. The name and the mode are
// two questions; this is the mode, and it is the core's for the same reason
// PosixSemantics is.
//
// The two fields are the whole of the care it needs, and the reason it is one
// function rather than a line at each call site. Entering records the answer
// the dialect held, and leaving puts *that* back rather than asserting the
// standard's opposite: a shell POSIX already agrees with would otherwise lose
// its own answer on the way out. A caller that wrote the axis directly instead
// would leave nothing to restore, and the mode could then never be left —
// measured, `set +o posix` in a shell invoked as `sh` carries on past a failed
// redirection on a special builtin, so leaving it has to reach the dialect's
// own answer.
//
// A request for the state we are already in moves nothing, which is what keeps
// an invocation's own `+o posix` from recording a saved answer that was never
// entered.
func (r *Runner) SetPosixMode(on bool) {
	if on == r.posixMode {
		return
	}
	redir, unsetRO := r.posixSaved, r.posixSavedUnsetReadonly
	forName := r.posixSavedForName
	funcName := r.posixSavedFuncName
	if on {
		r.posixSaved = r.sem().RedirectErrorOnSpecialBuiltinFatal
		r.posixSavedUnsetReadonly = r.sem().UnsetReadonlyFatal
		r.posixSavedForName = r.sem().ForNameWhenTheLoopRuns
		r.posixSavedFuncName = r.sem().FunctionNameWhenTheDefinitionRuns
		redir, unsetRO = Yes, Yes
		forName = ForNameEndsTheScriptAsASyntaxError
		funcName = FuncNameEndsTheScriptAsASyntaxError
		if r.posixSavedFuncName == FuncNameRunUnspecified {
			// The same silence the loop's axis keeps, for the same reason.
			funcName = FuncNameRunUnspecified
		}
		if r.posixSavedForName == ForNameRunUnspecified {
			// A dialect that never answered keeps its silence: the mode
			// moves an answer and does not invent one, so a script that
			// depends on the axis is still refused by name rather than
			// getting POSIX's answer to a question its shell never took a
			// position on. See ForNameRunForm.
			forName = ForNameRunUnspecified
		}
	}
	r.swapSemantics(func(s *Semantics) {
		s.RedirectErrorOnSpecialBuiltinFatal = redir
		// The second axis the mode moves, and measured the same way: `set -o
		// posix` makes bash 5.3 stop on `readonly x=1; unset x` and `set +o
		// posix` makes bash invoked as `sh` carry on past it. It needs a
		// saved answer of its own because the two axes do not agree — zsh
		// carries on past a failed redirection and stops here — so one
		// remembered value could not put both back.
		s.UnsetReadonlyFatal = unsetRO
		// The third axis the mode moves, and the one that needs a saved
		// value most: the dialect answers with a *form* rather than a bool,
		// and only one of that form's three values belongs to the mode. A
		// loop variable that is not a name fails the loop and lets the
		// script carry on under bash's own name, and ends the script at the
		// syntax-error status under `sh` — measured, and `set -o posix` in
		// bash 5.3 moves it exactly as the name does, which is what makes it
		// a mode and not a build. Unanswered stays unanswered in both
		// directions — the entering half is above, and this half is what
		// puts the dialect's own answer back rather than the standard's
		// opposite (#1110).
		s.ForNameWhenTheLoopRuns = forName
		// And the fourth, which is the same three-valued question asked of a
		// function definition's name: bash reports it and carries on under
		// its own name and stops at the syntax-error status under `sh`, and
		// `set -o posix` moves it exactly as the name does. Measured
		// alongside the loop's, and saved the same way — the two axes do not
		// have to agree, and ksh93 is why: it stops on both at 1, so a mode
		// that asserted the standard's answer on the way out would hand it
		// bash's (#1296).
		s.FunctionNameWhenTheDefinitionRuns = funcName
	})
	r.posixMode = on
	// The standard has aliases expand in a script, so the mode turns the
	// switch on and leaving it puts back the answer the *route* gave rather
	// than whatever was set before entering — measured in bash 5.3, where
	// `shopt -s expand_aliases; set -o posix; set +o posix` leaves
	// `expand_aliases` off. See Runner.aliasExpansion.
	if on {
		r.aliasExpansion = true
	} else {
		r.aliasExpansion = r.aliasExpansionBase
	}
}

// PosixMode reports whether the shell is in POSIX mode, which is the read
// side of SetPosixMode.
//
// Exported for the front end, which has a startup question that turns on it:
// the file a non-interactive shell sources out of a named variable is not
// sourced in POSIX mode. Measured — the shell that reads such a file reads it
// called by its own name, and reads nothing when started with the standard's
// posix option or invoked as `sh`. See Semantics.NonInteractiveStartupVariable.
func (r *Runner) PosixMode() bool { return r.posixMode }

// SetInteractiveMonitor turns the monitor on because this shell is an
// interactive one, which is what every shell in the panel does for itself.
//
// Exported for the front end, and separate from setMonitor for the reason
// SetPosixMode is separate from `set -o posix`: this is not a request a script
// made, so there is nobody to refuse and nothing to word. Measured, and the
// two really are different questions — `bash -c 'set -m'` with no terminal
// turns the monitor on, and `bash -i script.sh` with no terminal leaves it
// off, so a shell that routed one through the other would answer the second
// with the first's answer.
//
// Whether a terminal is needed is the dialect's —
// Semantics.InteractiveMonitorNeedsATerminal — and one member of the panel
// says it is not; whether there *is* one is Runner.Terminal, which the front
// end established. Read off the field rather than handed in, so that this and
// `set -m` cannot be told different things about the same shell.
//
// Only ever turns it on. A shell that has decided it is not running a monitor
// leaves the state where it was, so an inherited `set -m` is not undone by
// this.
func (r *Runner) SetInteractiveMonitor() {
	// Read rather than `ask`ed, for the reason InteractiveOptionLetters is
	// read: this runs once at startup, before the program has done anything,
	// so an unanswered axis would put "the shells disagree here" on the
	// screen ahead of every `-i script.sh` under a preset that has not
	// chosen — including scripts that never mention a job. An unanswered
	// field therefore reads as the majority and the quiet answer, which is
	// that a terminal is needed and the monitor stays off.
	if r.Terminal || r.sem().InteractiveMonitorNeedsATerminal == No {
		r.monitor = true
		return
	}
	// And an interactive shell that wanted the monitor and has no terminal
	// says so, in two of the four. Here rather than in the front end because
	// this is where the decision is, and because the two front-end routes
	// that reach it — a prompt and `-i` with something to run — would
	// otherwise each have to remember: measured, the same line comes out of
	// `-i script.sh`, `-i -c` and `-i -s` alike, and out of no
	// non-interactive route at all.
	// Who is named is the dialect's: one of the two that speak names the
	// script it was handed and the other names itself, and the Runner is
	// holding both — `Name` is `$0` and `Invocation` is argv[0].
	name := r.Invocation
	if name == "" || r.diag().NoJobControlAtStartupNamesTheScript {
		name = r.name()
	}
	r.errf("%s", r.diag().JobControlDiagnostic(name))
}

// SetInteractiveJobNotices gives this shell somebody to tell about its jobs
// because it is an interactive one, which is what a front end that has read
// `-i` says on a route that draws no prompt.
//
// Exported for the front end and separate from JobControl for the reason
// SetInteractiveMonitor is separate from setMonitor: the field is a fact the
// front end states, and this is a question the dialect answers. A prompt
// still sets JobControl outright — every shell in the panel announces a job
// to a person typing at it, so there is nothing there to ask.
//
// Only ever turns it on, and only on the one route it was measured for, and
// only where the monitor is already running. The route and the monitor are
// both the Runner's own, so nothing has to be handed in: the front end has
// already said where the program came from and already asked for the monitor.
//
// Call it after SetInteractiveMonitor, which is what settles the gate.
//
// Whether `-i -c` announces is a separate question with a different split and
// is not decided here — see Semantics.InteractiveScriptAnnouncesJobs.
func (r *Runner) SetInteractiveJobNotices() {
	// Read rather than `ask`ed, for the reason the monitor's answer is read:
	// this runs once at startup, so an unanswered axis would complain ahead
	// of every `-i script.sh` under a preset that has not chosen, including
	// the scripts that never start a job. An unanswered field reads as the
	// quiet answer, which is the intersection here.
	// The monitor first, and it is a gate rather than a coincidence. Measured
	// on `-i script.sh` with no terminal anywhere: dash and zsh, which leave
	// the monitor off there, say nothing about the job either, and ksh93,
	// which runs it without one, still announces both ends. So the notice
	// rides on the monitor, and the axis is what the one dialect that runs a
	// monitor and stays quiet anyway is for.
	if !r.monitor {
		return
	}
	if r.Route == RouteScriptFile && r.sem().InteractiveScriptAnnouncesJobs == Yes {
		r.JobControl = true
	}
}

// setMonitor is `set -m`, the one request in the table a dialect can refuse:
// two of the panel tie job control to the terminal, and this runner only has
// one when a front end said so (Runner.Terminal).
//
// Measured with no terminal, which is what a script on a pipe has: bash and
// ksh93 grant it silently — background jobs already run in process groups of
// their own here, so there is nothing further to promise — dash remarks
// `can't access tty; job control turned off` and reports success with the
// option left off, and zsh refuses at 1, fatally, echoing the spelling that
// asked. Turning it *off* is granted everywhere.
//
// The terminal and not JobControl, which is what this asked until #1720:
// having somebody to announce a job to is a prompt, and having a terminal is
// any route started from one. Measured on a pseudo-terminal, all five of
// bash 5.3.15, bash 3.2.57, ksh93u+, dash and zsh 5.9.2 grant `set -m` inside
// a plain `-c` string and put `m` in `$-`; the JobControl reading refused
// every one of those, because no `-c` has a person to tell.
func (r *Runner) setMonitor(on bool, spelling string) bool {
	if !on {
		r.monitor = false
		return true
	}
	if !r.Terminal && r.ask(r.sem().MonitorNeedsATerminal, "`set -m` in a shell with no terminal") {
		d := r.diag()
		r.diagf("%s\n", Wording(d.MonitorDenied, "set: cannot turn on job control without a terminal", spelling))
		if d.MonitorDeniedStatus == 0 {
			// A remark rather than a failure: the option is left off and
			// `set` still reports success, which is dash's shape.
			return true
		}
		r.setOptionStatus = d.MonitorDeniedStatus
		r.endOnSetRefusal(d.MonitorDeniedStatus, "a refused `set -m` ending the script")
		return false
	}
	if r.unspecified {
		return false
	}
	r.monitor = true
	return true
}

// SetOptionLetters applies a run of single-letter options — `e` and `ux`
// from `-e` and `+ux` — exactly as `set` reads them, returning 0 or the
// status the dialect gives a letter it refuses. A refusal has already been
// said on the runner's error stream.
//
// Exported for the front end, with SetNamedOption, and the same shape as it:
// every shell in the panel accepts its `set` options at invocation too —
// `sh -e script.sh` is an everyday spelling — and routing them through the
// same machinery is what keeps `sh -f` and `set -f` the same question with
// the same dialect answer.
//
// It returned a bool once, which the front end could only turn into the one
// status it had: `zsh -q` exited 2 where zsh exits 1, because the answer the
// dialect held had nowhere to travel (#483).
func (r *Runner) SetOptionLetters(letters string, on bool) int {
	// No script line has run yet; a dialect prelude may have. bash reports
	// an invocation option's failure at "line 0", and a diagnostic naming
	// the prelude's last line would point somewhere nobody wrote.
	r.line = 0
	r.atInvocation = true
	defer func() { r.atInvocation = false }()
	ok := r.setLetters(letters, on)
	if r.unspecified {
		// A letter that hangs on an axis no dialect answered — `-f` under
		// core — has been refused out loud. Inside a script the run loop
		// reads this flag; at invocation the caller only gets the status, so
		// it is folded in here and cleared the way the run loop clears it.
		// 2, since a dialect that answered nothing has no status either.
		r.unspecified = false
		return 2
	}
	if !ok {
		return r.setOptionFailure()
	}
	return 0
}

// AtInvocation reports whether the option request being served came from the
// words the shell was started with rather than from a line of script.
//
// Exported for a dialect's own option table, which is the one place the
// *route* can change the answer rather than only the wording: one shell in
// the panel refuses an option to a running script and takes the same option
// on its command line (Semantics.ImmovableOptionsSetAtInvocation). The core
// cannot answer that on the dialect's behalf, because only the dialect knows
// what such a name would move.
//
// Set for the length of one call in SetOptionLetters and SetNamedOption,
// which are the front end's only way in, so a `setopt` written in a script
// always reads false.
func (r *Runner) AtInvocation() bool { return r.atInvocation }

// SetNamedOption applies one long option — `-o pipefail`, `+o allexport` —
// exactly as `set -o` reads it, returning 0 or the status the dialect gives
// a name it refuses. Exported for the front end, with SetOptionLetters.
func (r *Runner) SetNamedOption(name string, on bool) int {
	r.line = 0
	r.atInvocation = true
	defer func() { r.atInvocation = false }()
	return r.namedOptionAnswer(r.setNamedOption(name, on))
}

// ApplyNamedOption is SetNamedOption from inside a running script: a
// registered builtin presenting the same options under its own names routes
// through here, where the line a complaint would name is the one being run
// rather than the zero an invocation reports.
//
// And where a refusal is not fatal, whatever the dialect answers for `set`:
// whoever arrives here is not `set`, which is the special builtin the
// fatality belongs to. See endOnSetRefusal for the measurement.
func (r *Runner) ApplyNamedOption(name string, on bool) int {
	r.outsideSetBuiltin = true
	defer func() { r.outsideSetBuiltin = false }()
	return r.namedOptionAnswer(r.setOption(name, on))
}

// namedOptionAnswer turns "did it work" into the status the two exported
// entry points hand back.
func (r *Runner) namedOptionAnswer(ok bool) int {
	if r.unspecified {
		r.unspecified = false
		return 2
	}
	if !ok {
		return r.setOptionFailure()
	}
	return 0
}

// setNamedOption reads a `set -o` operand in the namespace the *script* means
// by one: the dialect's own where it installed one with SetOptionTable, and
// the substrate's otherwise.
//
// Separate from setOption, which stays the substrate's own table, because the
// dialect's entries are written in terms of it — zsh's `err_exit` moves the
// substrate's `errexit` — so one function serving both would call itself.
// This is the seam a `set -o` operand and an invocation's `-o` arrive at, and
// setOption is the seam a dialect's own option builtin arrives at.
func (r *Runner) setNamedOption(name string, on bool) bool {
	if r.optionMover == nil {
		return r.setOption(name, on)
	}
	moved, known := r.optionMover(r, name, on)
	switch {
	case !known:
		return r.badSetOptionName(name)
	case moved:
		return true
	}
	// A name this shell has and will not move. Spoken here rather than by the
	// dialect so that the location, the status and whether it ends the script
	// are the same three answers every other refused `set` option gets.
	d := r.diag()
	r.saySetRefusal(Wording(d.SetImmovableOptionName, "set: %[1]s: not implemented", name),
		d.SetInvalidOptionNameUsage, true)
	return r.setRefusalStatus("a `set -o` name this shell will not move ending the script")
}

// AddSetOptions declares the `set -o` names this shell has beyond the ones
// they all share.
//
// Here rather than as axes for the reason Apply gives about variables and
// builtins: which names a shell has is the same kind of question, and one
// Answer per name would be ninety of them saying nothing but "yes" or "no".
func (r *Runner) AddSetOptions(names ...string) {
	if r.extraOptions == nil {
		r.extraOptions = make(map[string]bool, len(names))
	}
	for _, n := range names {
		r.extraOptions[n] = true
	}
}

// lookupSetOption finds a name this shell has, if it has it.
func (r *Runner) lookupSetOption(name string) (setOption, bool) {
	if o, ok := commonSetOptions[name]; ok {
		return o, true
	}
	if !r.extraOptions[name] {
		return setOption{}, false
	}
	// A name the dialect declared. Anything not described above is something
	// we do not do, which is the safe reading: it can be turned off and not
	// on.
	return extraSetOptions[name], true
}

// setOptionFailure is what a refused `set -o` reports, and resets it.
//
// A field rather than a return value because the refusal happens two calls
// down from the builtin, past a bool that says only whether it worked. Reset
// on the way out so a later refusal cannot inherit an earlier answer.
func (r *Runner) setOptionFailure() int {
	code := r.setOptionStatus
	r.setOptionStatus = 0
	if code == 0 {
		// Refused for a reason with no dialect answer of its own — a name
		// this shell has and does not do.
		return 2
	}
	return code
}

// optionState reads one option's current answer for the listings.
func (o setOption) state(r *Runner) bool {
	if o.get != nil {
		return o.get(r)
	}
	return o.on
}

// listedOptions is every row a `set -o` listing writes: the dialect's own
// table where it installed one with SetOptionTable, and the substrate's names
// with their states otherwise.
//
// One shape for both, because the four listing layouts are the same four
// whichever namespace supplies the rows — a name and whether it is on is all
// any of them writes.
func (r *Runner) listedOptions() []ListedOption {
	if r.optionListing != nil {
		return r.optionListing(r)
	}
	names := r.listedOptionNames()
	rows := make([]ListedOption, 0, len(names))
	for _, n := range names {
		o, _ := r.lookupSetOption(n)
		rows = append(rows, ListedOption{Name: n, On: o.state(r)})
	}
	return rows
}

// listedOptionNames is every name this shell answers `set -o` with, sorted.
func (r *Runner) listedOptionNames() []string {
	names := make([]string, 0, len(commonSetOptions)+len(r.extraOptions))
	for n := range commonSetOptions {
		names = append(names, n)
	}
	for n := range r.extraOptions {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// listOptions is `set -o` with nothing after it: every option and its state,
// in the dialect's columns — bash pads to fifteen and tabs, dash and ksh93
// open with a header, and the widths are theirs. `set +o` instead writes
// re-inputtable commands, except the dialect whose one line names only what
// is on.
func (r *Runner) listOptions(plus bool) int {
	rows := r.listedOptions()
	if plus {
		if r.diag().PlusOListsActive {
			line := "set --default"
			for _, row := range rows {
				if row.On {
					line += " --" + row.Name
				}
			}
			r.printf("%s\n", line)
			return 0
		}
		for _, row := range rows {
			sign := "+"
			if row.On {
				sign = "-"
			}
			r.printf("set %so %s\n", sign, row.Name)
		}
		return 0
	}
	if h := r.diag().OptionListingHeader; h != "" {
		r.printf("%s\n", h)
	}
	width := r.diag().OptionListingWidth
	if width == 0 {
		width = 15
	}
	sep := ""
	if r.diag().OptionListingTabbed {
		sep = "\t"
	}
	for _, row := range rows {
		state := "off"
		if row.On {
			state = "on"
		}
		r.printf("%-*s%s%s\n", width, row.Name, sep, state)
	}
	return 0
}
