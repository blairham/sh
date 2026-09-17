// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

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
// mode has left it exactly where it was asked to be.
//
// Turning *on* something we do not do is the question #3128 reopened, and the
// answer is a fourth kind rather than a refusal: the name is **recorded**,
// which means remembered and reported and acted on by nothing. A refusal was
// the honest answer while the listing was short; it stopped being one when
// #2925 made the listing carry the shell's whole roster, because a listing is
// a capture surface and a name it writes that `set` will not take is a line
// `eval "$(set +o)"` cannot feed back. Measured 2026-09-16 against every name
// in every panel shell's own listing, in both directions: bash and dash refuse
// none, and ksh93 and zsh refuse only the handful their dialects already
// declare. See recordedOption, and Runner.AddImmovableSetOptions and
// Runner.AddInertSetOptions for the two shapes a dialect can still say no in.

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
	// Not a claim about what any *other* shell defaults to: a name recorded
	// here is one this shell does not act on, so the state it reports is the
	// state it is already in and asking for that state succeeds honestly.
	// `hashall` used to be the example and is not one any more — the command
	// hash is real since #2554, the option gates it in the dialect that says
	// it should, and its entry below reads live state through a method.
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

	// The rest of the unanimous names, none of which this shell has yet:
	// we do not defer a job notice, do not hold the session open at
	// end-of-file, and keep no history for `nolog` to leave a function
	// definition out of. All three are recorded rather than refused, which
	// is measured — every shell in the panel that has the name takes it in
	// both directions at 0, and the three were `set: notify: not
	// implemented` here under five of our six dialects (#3128). See
	// recordedOption.
	"notify":    recordedOption("notify", false),
	"ignoreeof": recordedOption("ignoreeof", false),
	"nolog":     recordedOption("nolog", false),

	// The two editing modes, and they are one state under two names — see
	// Runner.editingMode and EditingMode. Neither starts selected, because a
	// shell with no line to edit has no keymap to be in; asking for `vi`
	// selects the other keymap, which is as much as this editor can promise
	// and exactly what the zsh dialect's `bindkey -v` already promises.
	//
	// Turning either *off* leaves neither on rather than swapping to the
	// other, measured in bash 5.3 and ksh93 both: `set -o vi; set +o vi`
	// reports `emacs off` and `vi off`.
	"vi": {
		apply: func(r *Runner, on bool) { r.setEditingMode(EditingModeVi, on) },
		get:   func(r *Runner) bool { return r.EditingMode() == EditingModeVi },
	},
	"emacs": {
		apply: func(r *Runner, on bool) { r.setEditingMode(EditingModeEmacs, on) },
		get:   func(r *Runner) bool { return r.EditingMode() == EditingModeEmacs },
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

// The three states a script can ask for, and a fourth the zero value holds:
// no mode chosen yet, which is not the same as one turned off.
const (
	// editingModeUnchosen is the zero value, and it is unexported because it
	// is not a state a script can select or observe directly: what it reads
	// as depends on whether this shell is interactive, which is a question
	// no `set` operand asks. A shell that has had a mode chosen — either way
	// round, including off — never reads it again.
	//
	// Separate from EditingModeNone because the two answer differently in
	// exactly one shell and exactly one place. Measured on bash 5.3.15,
	// 2026-09-11: `bash -i -c 'set -o'` reports `emacs on` and
	// `bash -i -c 'set +o emacs; set -o'` reports `emacs off`, so "never
	// chosen" and "chosen off" are both observable in the same shell.
	editingModeUnchosen EditingMode = iota

	EditingModeEmacs
	EditingModeVi

	// EditingModeNone is what turning the selected mode off leaves behind,
	// and it is a real state rather than an absence: `set +o emacs` in bash
	// 5.3 reports `emacs off` *and* `vi off`, so a script can observe it.
	EditingModeNone
)

// EditingMode reports which editing mode is selected, for a dialect deciding
// which keymap its binding builtin acts on.
//
// Nothing is selected until a script selects one or the shell becomes a shell
// with a line to edit, which is the whole of Semantics.InteractiveSelectsEmacs
// — one shell in the panel reads `emacs on` the moment it is interactive and
// the other three never select a mode on their own. Measured 2026-09-11, at a
// terminal as well as without one: this shell reported `emacs on` in a script,
// which is a keymap claimed by a shell that has no line to edit (#1858).
//
// The axis is read rather than asked, as the invocation's own facts are:
// reporting an option is not the place to refuse a script over a disagreement,
// and a dialect that answers nothing gets the majority's no.
func (r *Runner) EditingMode() EditingMode {
	if r.editingMode != editingModeUnchosen {
		return r.editingMode
	}
	if r.Interactive && r.sem().InteractiveSelectsEmacs == Yes {
		return EditingModeEmacs
	}
	return EditingModeNone
}

// setEditingMode is the write half of the two option names.
//
// Turning a mode on selects it; turning one off leaves neither selected,
// which is the measurement and not a shortcut — see the table entries. Asking
// to turn off a mode that is not the selected one moves nothing, because the
// request has already been granted.
//
// The read is EditingMode rather than the field, so that `set +o emacs` in a
// shell that has chosen nothing and *would* read emacs writes the refusal
// down: measured on bash 5.3.15, `bash -i -c 'set +o emacs; set -o'` reports
// both names off, and `bash -i -c 'set +o vi; set -o'` leaves emacs on.
func (r *Runner) setEditingMode(mode EditingMode, on bool) {
	switch {
	case on:
		r.editingMode = mode
	case r.EditingMode() == mode:
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
	// Brace expansion, which really moves: `set +o braceexpand` leaves
	// `{a,b}` the word it was written as, and setting it again puts the
	// expansion back — unlike `noexec`, which is one-way in every shell that
	// has it. The dialect still says whether the shell *has* the name, and
	// Semantics.BraceExpansion says whether it has braces at all; this is
	// only the switch beside them (#1856).
	"braceexpand": {
		apply: func(r *Runner, on bool) { r.noBraceExpand = !on },
		get:   func(r *Runner) bool { return !r.noBraceExpand },
	},
	// Comments are honored wherever they are written, which is what this
	// name asks for — so the state is on and nothing here reads it. Recorded
	// rather than fixed on, because bash takes `set +o interactive-comments`
	// at 0 and we refused it: measured 2026-09-16, it is the one name in
	// bash's own listing this shell refused in the *plus* direction, which
	// is the half of #3128 the roster there was never asked about.
	"interactive-comments": recordedOption("interactive-comments", true),

	// pipefail is real — the pipeline code reads it — and its *existence* is
	// an axis older than this table (see setOption), so applying it never
	// reaches this entry. The entry is what puts it in the listings of the
	// dialects that declare the name.
	"pipefail": {get: func(r *Runner) bool { return r.pipefail }},

	// Command tracking under its two names: bash calls it hashall — and zsh
	// takes that name too — where ksh93 says trackall. One state behind
	// both, and since #2554 it is load-bearing rather than merely honest:
	// where the dialect reads the option as a stop, turning it off empties
	// nothing and fills nothing. See the field, and
	// Semantics.HashObeysCommandTracking for the two shells that read it
	// that way and the one that does not.
	//
	// Through the pair of methods rather than the field, because the state
	// this reports before a script has moved it is the startup letters' and
	// not the zero value's: `set -o` said `hashall off` in a bash whose own
	// `$-` said `h` (#1951).
	"hashall": {
		apply: func(r *Runner, on bool) { r.setCommandTracking(on) },
		get:   func(r *Runner) bool { return r.commandTracking() },
	},
	"trackall": {
		apply: func(r *Runner, on bool) { r.setCommandTracking(on) },
		get:   func(r *Runner) bool { return r.commandTracking() },
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
	// promise nothing here keeps. Today that is eleven axes, and the evidence
	// is direct in every case — `set -o posix` makes a failed redirection on
	// a special builtin end bash 5.3 and bash 3.2, `set +o posix` makes bash
	// invoked as `sh` carry on, and the two states are exactly the panel's
	// `bash` and `bash-as-sh` columns. See SetPosixMode for the list and for
	// the measurement behind each.
	//
	// Eight of the eleven take the standard's own answer, and the other
	// three take the *dialect's* answer to what its mode makes of that axis.
	// See SetPosixMode: the mode is the core's, and what a given shell's
	// mode moves is not (#2583, #2641).
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
	// The two trap-carriage options under their long names. The letters
	// already wrote these fields — see the `E`, `T` case in the `set`
	// builtin — and the names sat here refused, so a script reaching the
	// state the long way was told `not implemented` and left with a DEBUG
	// trap that never fired inside the call it was set to watch (#2426).
	// One state behind the letter and the name, which is the rule `hashall`
	// and `set -h` already follow: two spellings of one question cannot be
	// allowed to answer differently.
	//
	// Whether the shell *has* the names stays the dialect's, and it is the
	// same shell whose `$-` shows `E` and `T` — the other three have neither
	// spelling. What the state then does is read at the firing sites in
	// pseudotrap.go, which is where the carriage is decided.
	"errtrace": {
		apply: func(r *Runner, on bool) { r.errtrace = on },
		get:   func(r *Runner) bool { return r.errtrace },
	},
	"functrace": {
		apply: func(r *Runner, on bool) { r.functrace = on },
		get:   func(r *Runner) bool { return r.functrace },
	},
	// The two history states, which are two states and not one: bash lists
	// `history` and `histexpand` separately in `set -o`, and measured on
	// bash 5.3.20 a script with `set -o history; set -H` expands where a
	// script with only the first does not. Both were listed here and refused
	// until #3093, so `set -H` — the line a person's rc file opens with —
	// was `set: -H is not implemented yet` and the shell had no expander at
	// all.
	//
	// Whether the shell *has* the names stays the dialect's; what the state
	// then does is read by the front end, which is the only part of this
	// tree holding a history list to index. See Semantics.HistoryExpansion.
	"history": {
		apply: func(r *Runner, on bool) { r.setHistoryRecording(on) },
		get:   func(r *Runner) bool { return r.histRecord },
	},
	"histexpand": {
		apply: func(r *Runner, on bool) { r.SetHistoryExpansion(on) },
		get:   func(r *Runner) bool { return r.histExpand },
	},
	// `set -o keyword`, the long spelling of `set -k`: every `name=value`
	// word of a simple command is a prefix assignment and not only the ones
	// in front of the command name. Listed here and refused until now, so a
	// script reaching the state either way was told `not implemented` and
	// then ran with the option off — the argument count wrong, `$1` the
	// assignment's own text, and the variable the command expected unset, all
	// at status 0 (#3095).
	//
	// One state behind the letter and the name, which is the rule `hashall`
	// and `set -h` already follow. Whether the shell *has* the name stays the
	// dialect's: bash and ksh93 list it, zsh answers `no such option`, and
	// dash and BusyBox ash refuse the letter and the name alike. See
	// Semantics.KeywordAssignments.
	"keyword": {
		apply: func(r *Runner, on bool) { r.keywordAssignments = on },
		get:   func(r *Runner) bool { return r.keywordAssignments },
	},
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
	// `physical` is `cd -P` as a mode, and `privileged` is `-p`. Both are
	// recorded: the resolver behind `cd` is real (see physicalpath.go) and
	// the option is not wired to it, and nothing here drops privilege,
	// so remembering is the whole of what either promises.
	//
	// The panel splits over `privileged` and only over whether it *moves*.
	// bash 5.3.20 and bash 3.2.57 take `set -o privileged` and then report
	// `privileged on`; ksh93u+ takes it at 0 and reports `privileged off`
	// afterwards, and reports it off under `ksh -p` as well — measured
	// 2026-09-16. So the name is taken everywhere and the state is bash's
	// alone, which is Runner.AddInertSetOptions and not a second entry here.
	"physical":   recordedOption("physical", false),
	"privileged": recordedOption("privileged", false),

	// The three names one shell in the panel has and the rest do not, and
	// all three describe **how the shell was started** rather than a
	// behavior a script chose. That is what makes them worth having: a
	// script reads `set -o` to find out which of them it is under, and the
	// listing was three rows short of being able to say (#2624).
	//
	// Measured on dash, 2026-09-13. Each is settable in both directions at
	// 0, and the first two carry the `$-` letter that names the same fact:
	//
	//	set -o interactive; echo $-             i
	//	set -o interactive; set +o interactive  the letter goes again
	//	set -o stdin; echo $-                   s      (under -c)
	//	set +o stdin; echo $-                   empty  (on the stdin route)
	//	set -o debug; echo $-                   empty, and `debug on` listed
	//
	// So two of them are a second spelling of a state this shell already
	// holds and the third is a recorded name. `interactive` writes the field
	// the front end filled in, which is the same state `$-`'s `i` and the
	// prompt decision read — one fact with two spellings, which is the rule
	// `hashall` and `set -h` already follow.
	"interactive": {
		apply: func(r *Runner, on bool) { r.Interactive = on },
		get:   func(r *Runner) bool { return r.Interactive },
	},
	"stdin": {
		apply: func(r *Runner, on bool) { r.stdinOptionMoved, r.stdinOption = true, on },
		get:   (*Runner).showsS,
	},
	// Listed, remembered, and acted on by nothing — which is what the shell
	// that has the name does with it in the build it ships.
	"debug": {
		apply: func(r *Runner, on bool) { r.debugOption = on },
		get:   func(r *Runner) bool { return r.debugOption },
	},

	// The names one shell in the panel has and the rest do not, beyond the
	// three above. Measured 2026-09-15 on ksh93u+ over `-c`, over `-i` with
	// no terminal, and over a login invocation by both `-l` and an `argv[0]`
	// of `-ksh`, which is what tells a state apart from a constant.
	//
	// Three of them are facts about the *invocation* and read live state
	// here rather than a default: `bgnice` and `rc` are off in a script and
	// on at a prompt, which is the same fact `interactive` above reports,
	// and `login_shell` is on under both login routes and off otherwise. A
	// constant written for any of the three would have been wrong on one of
	// the two runs that produced it.
	// `bgnice` is recorded *over* that live fact rather than fixed to it,
	// because ksh93 moves it and `rc` it will not: measured 2026-09-16,
	// `set -o bgnice; set -o` reports `bgnice on` in a script whose bare
	// listing said off, while `set -o rc` there is `bad option(s)` in both
	// directions — which is why only one of the two takes a request.
	"bgnice": recordedOverOption("bgnice", false, func(r *Runner) bool { return r.Interactive }),
	"rc":     {get: func(r *Runner) bool { return r.Interactive }},
	// login_shell reads the field the front end filled in, the same one
	// `$-`'s `l` is drawn from where a dialect shows the letter.
	"login_shell": {get: func(r *Runner) bool { return r.LoginShell }},

	// And the rest are states, listed with the state this shell is in. Two
	// of them are on, and both describe how a *line* is read and drawn
	// rather than how a script runs: this editor reads raw keystrokes and
	// redraws a line that outgrows the terminal in place rather than
	// scrolling it sideways, which is what the two names ask for. The shell
	// that has them reports both on in every route measured.
	//
	// Recorded rather than fixed on, because the plus direction is a request
	// too: `set +o multiline` and `set +o viraw` are 0 in ksh93u+ and were
	// `not implemented` here, which is the half of #3128 that only a sweep
	// of the listing in *both* directions finds.
	"multiline": recordedOption("multiline", true),
	"viraw":     recordedOption("viraw", true),
	// `**` crossing directory levels, under the name one shell in the panel
	// spells it with. bash has the same behavior behind a `shopt` and reaches
	// it through dialect/bash/shopt.go; this is the other door onto the same
	// option, and both move the state the walk reads so the two spellings
	// cannot disagree.
	//
	// The state rather than a record of the request, which is what took this
	// name out of the group below. It was recorded from #3128 until #3152
	// built it, and the rule that move follows is written at recordedOption:
	// **a name leaves the recorded set in the change that makes it real**,
	// because a listing saying `globstar on` over a `**` still reading as
	// `*` is the same lie the refusal was, one level quieter.
	//
	// The *other* three questions `**` raises — bare `**` crossing levels, a
	// link counting as one of them, and a run of `**` being one component —
	// are the dialect's and are set beside the walk rather than here: this
	// option is the only name either shell has for any of them, so a dialect
	// that has this name has already answered all four. See
	// interp.StarStarCrossesDirectories and the three that follow it.
	"globstar": {
		apply: func(r *Runner, on bool) { r.SetMatchOption(StarStarCrossesDirectories, on) },
		get:   func(r *Runner) bool { return r.MatchOption(StarStarCrossesDirectories) },
	},

	// The rest are off here and off there. None of them is acted on: there
	// is no third editing mode, `let` reads no octal, a glob marks no
	// directory, nothing is withheld for `showme`, and this shell has no
	// restricted mode. All five are recorded: ksh93u+ takes every one of
	// them in both directions at 0 and reports the state back afterwards —
	// measured 2026-09-16, `set -o markdirs; set -o` is `markdirs on` there
	// — so refusing them made the listing advertise names the shell then
	// declined (#3128). What each is recorded *for* is the follow-up work;
	// remembering it is not a claim to do it. See recordedOption.
	"gmacs":      recordedOption("gmacs", false),
	"letoctal":   recordedOption("letoctal", false),
	"markdirs":   recordedOption("markdirs", false),
	"restricted": recordedOption("restricted", false),
	"showme":     recordedOption("showme", false),
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
// **One axis it moves is one-way**, and it is the exception to everything the
// paragraph on the saved fields below says. `set -o posix` turns bash's
// `inherit_errexit` on, and `set +o posix` does *not* turn it off: measured
// 2026-09-15 on bash 5.3.20, `set -o posix; set +o posix; set -e; echo
// "end[$(false; echo no)]"` is `end[]`, where the same line without the mode
// is `end[no]`. What the mode entered was a shell option with a name of its
// own, and leaving the mode leaves the option where it put it — `shopt -u
// inherit_errexit` is the only way back. So
// ErrExitEntersACommandSubstitution is written on the way in and has no saved
// value, because there is nothing to put back. bash 3.2 does move back, and
// this preset carries bash 5's reading for the reason
// Semantics.UnsetReadonlyFatal gives.
//
// **Nine of the twelve axes it moves take the standard's own answer**, because
// that is what the name asks for and every shell with a POSIX mode was measured
// to take them. The other three it takes from the dialect, through a field of
// their own apiece, because a value written in here reaches every dialect
// invoked as `sh` and the panel does not agree about any of them:
//
//   - BadOptionToSpecialBuiltinFatal, through
//     BadOptionToSpecialBuiltinFatalInPosixMode, because bash's mode moves it
//     to fatal through either door and zsh's leaves it alone, while both
//     shells' mode moves the redirection axis beside it. A knob that wrote
//     bash's answer would have given zsh-as-`sh` a fatality zsh has not got
//     (#2583).
//   - BadSetOptionNameFatal and BadSetOptionLetterFatal, through
//     BadSetOptionNameFatalInPosixMode and BadSetOptionLetterFatalInPosixMode,
//     because `set`'s own refusal has never gone through the axis above — it
//     has one per spelling — and because BusyBox ash, which has no POSIX mode
//     at all, carries on past a refused *name* under every name it is called
//     by. The standard's answer written in here would end a script it does not
//     end (#2641).
//
// The saved fields are the whole of the care it needs, and the reason it is
// one function rather than a line at each call site. Entering records the
// answer the dialect held, and leaving puts *that* back rather than asserting
// the standard's opposite: a shell POSIX already agrees with would otherwise
// lose its own answer on the way out. A caller that wrote the axis directly
// instead would leave nothing to restore, and the mode could then never be
// left —
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
	reassignRO := r.posixSavedReassignReadonly
	specialRO := r.posixSavedSpecialReadonly
	targetPattern := r.posixSavedTargetPattern
	forName := r.posixSavedForName
	funcName := r.posixSavedFuncName
	exportListing := r.posixSavedExportListing
	readonlyListing := r.posixSavedReadonlyListing
	bareListing := r.posixSavedBareListing
	badOption := r.posixSavedBadOption
	badSetName, badSetLetter := r.posixSavedBadSetName, r.posixSavedBadSetLetter
	assignPrefix := r.posixSavedAssignPrefix
	aliasReserved := r.posixSavedAliasReserved
	quoteProtects := r.posixSavedQuoteProtects
	if on {
		r.posixSaved = r.sem().RedirectErrorOnSpecialBuiltinFatal
		r.posixSavedUnsetReadonly = r.sem().UnsetReadonlyFatal
		r.posixSavedReassignReadonly = r.sem().ReadonlyReassignmentFatal
		reassignRO = Yes
		r.posixSavedSpecialReadonly = r.sem().ReadonlyReassignmentBySpecialBuiltinFatal
		specialRO = Yes
		r.posixSavedForName = r.sem().ForNameWhenTheLoopRuns
		r.posixSavedFuncName = r.sem().FunctionNameWhenTheDefinitionRuns
		r.posixSavedExportListing = r.sem().ExportListing
		r.posixSavedReadonlyListing = r.sem().ReadonlyListing
		r.posixSavedBareListing = r.sem().BareDeclarationListing
		// Unanswered stays unanswered, exactly as the two forms above do
		// and for the same reason: the mode moves an answer and does not
		// invent one, so a dialect that never took a position on a listing
		// still refuses it by name rather than acquiring the standard's.
		exportListing = posixListing(r.posixSavedExportListing)
		readonlyListing = posixListing(r.posixSavedReadonlyListing)
		bareListing = posixListing(r.posixSavedBareListing)
		r.posixSavedBadOption = r.sem().BadOptionToSpecialBuiltinFatal
		badOption = r.sem().BadOptionToSpecialBuiltinFatalInPosixMode
		r.posixSavedBadSetName = r.sem().BadSetOptionNameFatal
		r.posixSavedBadSetLetter = r.sem().BadSetOptionLetterFatal
		badSetName = r.sem().BadSetOptionNameFatalInPosixMode
		badSetLetter = r.sem().BadSetOptionLetterFatalInPosixMode
		r.posixSavedAssignPrefix = r.sem().AssignmentPrefixPersistsOnSpecialBuiltin
		assignPrefix = Yes
		r.posixSavedAliasReserved = r.dialect().AliasesExpandReservedWords
		aliasReserved = false
		// The dialect's own answer on the way in, not the standard's: the
		// three shells with no POSIX mode do not agree with bash about this
		// axis and every dialect invoked as `sh` comes through here, so an
		// asserted reading would move a shell that has nothing to move. The
		// zero value of the move is what declines it.
		r.posixSavedQuoteProtects = r.dialect().QuoteProtectsTheClosingBrace
		quoteProtects = r.dialect().QuoteProtectsTheClosingBraceInPosixMode.
			Policy(r.posixSavedQuoteProtects)
		redir, unsetRO = Yes, Yes
		// The tenth, and it takes the standard's own answer like the seven
		// that do: POSIX says a non-interactive shell does not pathname-expand
		// a redirection's target, and both columns that match one otherwise
		// were measured to stop in the mode — bash 5.3.20 and 3.2.57 under
		// `set -o posix` and under the `sh` name, and zsh 5.9.2 invoked as
		// `sh`. Saved all the same, because leaving the mode has to reach the
		// dialect's own answer and not the standard's opposite (#3207).
		r.posixSavedTargetPattern = r.sem().RedirectTargetTakesPathnameExpansion
		targetPattern = No
		// The one-way axis, written here rather than through the swap
		// below's restore half: see the paragraph on it above. Set on the
		// way in and never read back out, which is the whole of the latch.
		r.swapSemantics(func(s *Semantics) {
			s.ErrExitEntersACommandSubstitution = Yes
		})
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
		s.RedirectTargetTakesPathnameExpansion = targetPattern
		// The second axis the mode moves, and measured the same way: `set -o
		// posix` makes bash 5.3 stop on `readonly x=1; unset x` and `set +o
		// posix` makes bash invoked as `sh` carry on past it. It needs a
		// saved answer of its own because the two axes do not agree — zsh
		// carries on past a failed redirection and stops here — so one
		// remembered value could not put both back.
		s.UnsetReadonlyFatal = unsetRO
		// And its twin for an assignment, which the mode had left behind:
		// measured 2026-09-16, `readonly v; v=2; echo after` carries on at
		// status 0 in bash 5.3.20 and ends the script at 1 under `set -o
		// posix`, under `bash -o posix` and under the `sh` name alike, and
		// `set +o posix` puts the carrying-on back. It takes the standard's
		// answer and not the dialect's, because every other column already
		// holds it under every name — ReadonlyReassignmentFatal is No in
		// bash alone — so a written-in Yes moves nothing but bash.
		s.ReadonlyReassignmentFatal = reassignRO
		// And the special builtins' half of the declaration question, which
		// the mode moves for the same reason and `declare` does not: see
		// ReadonlyReassignmentBySpecialBuiltinFatal for the measurement.
		s.ReadonlyReassignmentBySpecialBuiltinFatal = specialRO
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
		// The fifth, sixth and seventh, and the first the mode moves that
		// are about what a builtin *writes* rather than about what ends a
		// script. Measured 2026-09-12 on bash 5.3.15 and the 3.2.57 macOS
		// ships, over `V='a b'; export V; R=2; readonly R`: `export -p`,
		// `readonly -p` and the bare forms of both write the clustered
		// `declare -x V="a b"` under bash's own name and repeat the command
		// word — `export V="a b"` — under `set -o posix`, and `set +o posix`
		// puts the clustered form back. Both builds agree, so it is a mode
		// the shell enters and leaves rather than the build or the
		// invocation.
		//
		// `declare -p` is the control that makes this three axes and not
		// one: it writes `declare -x V="a b"` in both modes, so
		// DeclareListing is deliberately not moved here. Nor does the
		// quoting move — `export V="a b"` keeps the double quotes the
		// clustered form uses — so DeclareValueQuoting stays where the
		// dialect put it (#2154).
		//
		// Saved and restored one per axis for the reason the four above
		// are: the panel does not answer the three alike — zsh writes
		// `export V='a b'` for the first and `typeset -r R=2` for the
		// second — so a single remembered form would hand one axis
		// another's answer on the way out.
		s.ExportListing = exportListing
		s.ReadonlyListing = readonlyListing
		s.BareDeclarationListing = bareListing
		// The eighth, and the only one whose value comes from the dialect
		// rather than from the standard. The seven above are the standard's
		// own answers and every shell that has a POSIX mode was measured to
		// take them; this one the shells disagree about, and they disagree
		// about it *within* one mode rather than about the mode as a whole —
		// zsh invoked as `sh` stops on a failed redirection and still carries
		// on past `shift -x`, where bash under either door stops on both.
		//
		// So the mode asks the dialect what it makes of this axis and swaps
		// in the answer, rather than writing bash's into a knob that every
		// dialect invoked as `sh` goes through. Writing `Yes` here would have
		// given zsh-as-`sh` a fatality zsh does not have, which is the whole
		// of why this axis was not simply added to the others (#2583).
		s.BadOptionToSpecialBuiltinFatal = badOption
		// The ninth, and the second one that is about what a special builtin
		// leaves behind rather than about what ends a script. It takes the
		// standard's own answer, like the seven above and unlike the one
		// before it, because the panel was measured to be unanimous about it
		// in both halves — which is the whole of why it is here and not a
		// tenth `…InPosixMode` twin. Measured 2026-09-13 over
		// `FOO=1 : ; echo "[$FOO]"`:
		//
		//	          default   mode on           called sh
		//	bash 5.3    []      [1] -o posix        [1]
		//	bash 3.2    []      [1] -o posix        [1]
		//	zsh 5.9     []      [1] -o posixbuiltins [1]
		//	dash        [1]     no such mode        [1]
		//	ksh93       [1]     no such mode        [1]
		//	BusyBox ash [1]     no such mode        [1]
		//
		// Every shell with a POSIX mode moves to the standard's answer, and
		// every shell without one already holds it. That second half is the
		// measurement that matters here, because this knob is entered by
		// *every* dialect invoked as `sh`: writing `Yes` cannot impose
		// anything on dash, ksh93 or ash, since `Yes` is what all three
		// already say under every name. Contrast
		// BadOptionToSpecialBuiltinFatal directly above, where zsh's mode
		// does not move the axis and a written-in `Yes` would have given
		// zsh-as-`sh` a fatality zsh has not got (#2583, #2659).
		//
		// `FOO=1 true` is the control and it stays transient in both modes
		// in all six: the mode moves the special-builtin rule and not the
		// prefix rule as a whole, which is why this is the one field written
		// and not the site in Runner.execBuiltin.
		s.AssignmentPrefixPersistsOnSpecialBuiltin = assignPrefix
		// The tenth and eleventh, and the second pair the mode takes from
		// the dialect rather than from the standard. `set`'s own refusal
		// never went through BadOptionToSpecialBuiltinFatal — it has an axis
		// per spelling — so the axis above moved and these two did not,
		// which left `set -o posix; set -o zzznosuch` carrying on here and
		// ending the script in bash.
		//
		// From the dialect for the reason the bad-option axis is, and the
		// deciding row is BusyBox ash: it has no POSIX mode, its `sh` applet
		// is its `ash` applet, and it carries on at 1 past a refused *name*
		// under either name. Every dialect invoked as `sh` comes through
		// here, so a written-in `Yes` would end an ash script that really
		// goes on. See Semantics.BadSetOptionNameFatalInPosixMode for the
		// panel and for why the two spellings are two fields (#2641).
		s.BadSetOptionNameFatal = badSetName
		s.BadSetOptionLetterFatal = badSetLetter
	})
	// The twelfth, and the only one that is not on the vector at all: whether
	// an alias may stand in for a word the grammar reserves is decided while
	// a line is *read*, so it is a dialect field and the mode reaches it the
	// way a grammar-reaching option does — a replaced Dialect, never a write
	// through the shared pointer, since a subshell holds the same one and a
	// script must not change the grammar of the shell that spawned it. The
	// front end watches for the replacement and re-reads the rest of the
	// program with it; see driver's run loop and syntax.Parser.SetDialect.
	//
	// The standard's answer on the way in, like the seven above, because
	// both shells with a POSIX mode were measured to take it: `set -o posix`
	// protects the words in bash 5.3 and in bash 3.2, and so does invoking
	// either bash or zsh as `sh`. The saved answer on the way out, because
	// the two shells that have the mode both expand reserved-word aliases
	// without it and the three that do not have it never did.
	//
	// The thirteenth goes through the same door and is the same kind of
	// thing: where a `}` ends a `${ … }` is decided while a word is *read*.
	// It differs in what the run then owes the words it read *before* the
	// mode moved — see syntax.ParamExpr.RawTail and Runner.wordForRun, which
	// is the expansion-time half bash has and an alias does not.
	if d := r.dialect(); d.AliasesExpandReservedWords != aliasReserved ||
		d.QuoteProtectsTheClosingBrace != quoteProtects {
		d.AliasesExpandReservedWords = aliasReserved
		d.QuoteProtectsTheClosingBrace = quoteProtects
		r.Dialect = &d
	}
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

// posixListing is what POSIX mode makes of one listing axis, given the answer
// the dialect held. One function for all three, so a fourth axis cannot be
// added with the silence carve-out left off it.
func posixListing(saved DeclarationListingForm) DeclarationListingForm {
	if saved == DeclarationListingUnspecified {
		return DeclarationListingUnspecified
	}
	return DeclareListingCommandWord
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

// MonitorOn reports whether job control is running — `set -m`, `set -o
// monitor`, or the interactive shell's own decision.
//
// Exported for a dialect builtin that has to know, which today is `suspend`:
// the shell that refuses to stop without job control is refusing on this, and
// a dialect cannot reach the field. A reader and no writer, deliberately —
// turning the monitor on is a request a script or a front end makes through
// one of the two doors above, both of which have a refusal to word, and a
// third door that skipped them would be a monitor nothing could decline.
func (r *Runner) MonitorOn() bool { return r.monitor }

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
	// The shell that names the process group it could not hand the terminal
	// to says so first, above the line below. Its process group is read here
	// the way `$$` reads its process id — see the `$` case in expand.go: a
	// fact about this process, asked for at the moment it is printed, and
	// wanted by nothing else.
	r.errf("%s", r.diag().TerminalProcessGroupDiagnostic(name, syscall.Getpgrp()))
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
// Both interactive routes reach it and each reads its own axis: `-i script.sh`
// is Semantics.InteractiveScriptAnnouncesJobs and `-i -c` is
// Semantics.InteractiveCommandStringAnnouncesJobs. Two fields rather than one,
// because the columns disagree — bash is the only shell quiet on the first and
// dash and ash the only two quiet on the second.
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
	switch r.Route {
	case RouteScriptFile:
		if r.sem().InteractiveScriptAnnouncesJobs == Yes {
			r.JobControl = true
		}
	case RouteCommandString:
		if r.sem().InteractiveCommandStringAnnouncesJobs == Yes {
			r.JobControl = true
		}
	case RouteUnspecified, RouteStandardInput:
		// Neither is a route this was measured on. `-i` with the program on
		// standard input draws a prompt in every shell in the panel, so the
		// prompt sets JobControl outright and never reaches here, and an
		// embedder that has not said which route it is has not said it is
		// interactive either.
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
		// The one refusal that arrives under either spelling, so the axis it
		// asks is the one for the spelling that asked: `set -m` is the
		// letter's and `set -o monitor` is the name's. zsh is the only
		// dialect that refuses this and it answers the two alike — it stops
		// at 1 for both — so the two readings are indistinguishable here
		// today, and the point of choosing by the spelling rather than by
		// habit is that the spelling is already in hand.
		r.endOnSetRefusal(d.MonitorDeniedStatus, spellingRefused(spelling),
			"a refused `set -m` ending the script")
		return false
	}
	if r.unspecified {
		return false
	}
	r.monitor = true
	return true
}

// spellingRefused reads a spelling `set` echoed back and says which of its
// two refusals it is. `-m` and `+m` are letters; `monitor` is a name.
func spellingRefused(spelling string) setRefusalSpelling {
	if strings.HasPrefix(spelling, "-") || strings.HasPrefix(spelling, "+") {
		return refusedOptionLetter
	}
	return refusedOptionName
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

// SetShellOption moves one name in the dialect's *second* option namespace —
// bash's `shopt` table — from the words the shell was started with, and
// returns the status the invocation should end with. Exported for the front
// end, with SetNamedOption, and the namespace is installed by
// [Runner.SetShellOptionNamespace].
//
// The line is zeroed and the invocation flag set exactly as the two above do
// it, and for the same two reasons: no script line has run, and the mover has
// to know which route it is serving because bash words the two differently —
// `bash: line 0: nosuchopt: invalid shell option name` from the command line
// against `bash: line 1: shopt: nosuchopt: invalid shell option name` from
// the builtin. Measured on bash 5.3.20.
//
// A shell with no such namespace refuses the name, which is what a shell
// without the table would say about any name at all. It is not reachable from
// a dialect this repository ships — the front end reads the letter only where
// Semantics.ShellOptionInvocationLetter names one, and the one dialect that
// names it installs the table beside it — and refusing is the honest answer
// for a front end and a dialect that disagree.
func (r *Runner) SetShellOption(name string, on bool) int {
	r.line = 0
	r.atInvocation = true
	defer func() { r.atInvocation = false }()
	if r.shellOptionMover == nil {
		r.Diagnosef("%s: invalid shell option name\n", name)
		return 2
	}
	return r.shellOptionMover(r, name, on)
}

// ListShellOptions writes the whole of that second namespace, which is what
// the same invocation letter does with no name after it: `bash -O` is `shopt`
// and `bash +O` is `shopt -p`, both byte-for-byte and both at status 0 with
// the shell going on to run whatever it was given. Measured on bash 5.3.20.
//
// Silent in a shell with no such namespace, which is the other half of
// SetShellOption's fallback: there are no rows to write.
func (r *Runner) ListShellOptions(reissuable bool) {
	if r.shellOptionListing == nil {
		return
	}
	r.shellOptionListing(r, reissuable)
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
	return r.setNamedOptionSpelled(name, name, on)
}

// setNamedOptionSpelled is setNamedOption for a caller whose script wrote the
// option some other way — a single letter this dialect gives it.
//
// The two spellings are one request with one answer, which is the whole
// reason the letters go through here rather than through a switch of their
// own. What differs is only what a refusal echoes back: measured 2026-09-13
// on zsh 5.9.2, an option the shell has and will not move is `can't change
// option: -i` when a letter asked and `can't change option: interactive` when
// the name did, so the sentence is one wording with the caller's own spelling
// in it.
func (r *Runner) setNamedOptionSpelled(name, spelled string, on bool) bool {
	if r.optionMover == nil {
		return r.setOption(name, on)
	}
	moved, known := r.optionMover(r, name, on)
	switch {
	case !known:
		return r.badSetOptionName(spelled)
	case moved:
		return true
	}
	// A name this shell has and will not move. Spoken here rather than by the
	// dialect so that the location, the status and whether it ends the script
	// are the same three answers every other refused `set` option gets.
	d := r.diag()
	r.saySetRefusal(Wording(d.SetImmovableOptionName, "set: %[1]s: not implemented", spelled),
		d.SetInvalidOptionNameUsage, true)
	return r.setRefusalStatus(refusedOptionName,
		"a `set -o` name this shell will not move ending the script")
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

// commandTracking reports whether command tracking is on — the option bash
// lists as `hashall`, ksh93 as `trackall`, and both abbreviate `-h`.
//
// It is a method and not a field read because the state has no constant
// default. What a shell starts it in is the shell's own fact, and the dialect
// has already stated it: a startup letter *is* the claim that the option
// behind it is on before the script's first line, so `h` among the letters is
// that declaration and nothing further needs saying. Deriving it here rather
// than taking a second declaration is what keeps the two from disagreeing —
// which is exactly the bug, in both directions at once: bash's letters are
// `hB` and the zero value is off, so `$-` said the option was on while
// `set -o` said it was off in a shell that had run nothing, and `set +h`
// could not take the letter back out because no state stood behind it
// (#1951).
//
// It also gets the case a constant default cannot reach. ksh93's letters are
// `hB` for a script and `imBE` when it is interactive — the `h` is *gone* —
// and `set -o` there reports `trackall on` for the script and off at a
// prompt, measured 2026-09-12. A default written down once, wherever it was
// written, would be wrong for one of those two.
func (r *Runner) commandTracking() bool {
	if r.tracksCommandsMoved {
		return r.tracksCommands
	}
	// Only where the letter means tracking at all. zsh spells a history
	// option with `h` and has no startup letters to read anyway, and a
	// shell whose dialect has not answered the axis has said nothing this
	// could stand on — see Semantics.SetHLetterTracksCommands.
	return r.sem().SetHLetterTracksCommands == Yes && r.startsWithOptionLetter('h')
}

// setCommandTracking is what `set -h`, `set -o hashall` and `set -o trackall`
// all write, and it records that the state has been spoken for so that the
// startup letters stop answering for it.
func (r *Runner) setCommandTracking(on bool) {
	r.tracksCommands, r.tracksCommandsMoved = on, true
}

// lookupSetOption finds a name this shell has, if it has it.
func (r *Runner) lookupSetOption(name string) (setOption, bool) {
	if base, ok := r.negatedOptions[name]; ok {
		// A name this dialect spells as the opposite of one the substrate
		// holds. The state behind it is the substrate's, read and written
		// upside down; see AddNegatedSetOptions. The base is never itself a
		// negated name, so this does not recur.
		o, known := r.lookupSetOption(base)
		if !known {
			return setOption{}, false
		}
		return negatedOption(o), true
	}
	if base, ok := r.negativeInteractiveSpelling(name); ok {
		// The `no`-prefixed spelling of this shell's own name for being
		// interactive, which is the same state read and written upside
		// down. Resolved here rather than declared as a row, because the
		// shell does not *list* it: measured 2026-09-16, ksh93u+ 2012-08-01
		// takes `ksh -o nointeractive` and `ksh +o nointeractive` at an
		// invocation and its `set -o` listing names only `interactive`.
		o, known := r.lookupSetOption(base)
		if !known {
			return setOption{}, false
		}
		return negatedOption(o), true
	}
	o, ok := commonSetOptions[name]
	if !ok {
		if !r.extraOptions[name] {
			return setOption{}, false
		}
		// A name the dialect declared. Anything not described above is
		// something we do not do, which is the safe reading: it can be
		// turned off and not on.
		o = extraSetOptions[name]
	}
	return o, true
}

// negativeInteractiveSpelling reports the name a dialect's
// Semantics.NonInteractiveOptionName stands for, which is always its
// Semantics.InteractiveOptionName read upside down.
//
// The pair is declared for the front end — an invocation says whether the
// shell prompts by either spelling, and which words mean it is the dialect's
// to say (#3195). This is the other half of the same declaration: a shell
// that grants `+o nointeractive` on its command line has to *move* something
// when it does, and the state is the one `interactive` already names.
//
// Only reached by a dialect whose option namespace is the substrate's own
// table. The one shell with a namespace of its own installs a mover
// (Runner.SetOptionTable) and answers both spellings there, so this asks a
// question that namespace has already answered.
func (r *Runner) negativeInteractiveSpelling(name string) (string, bool) {
	s := r.sem()
	if s.NonInteractiveOptionName == "" || s.InteractiveOptionName == "" {
		return "", false
	}
	if name != s.NonInteractiveOptionName {
		return "", false
	}
	return s.InteractiveOptionName, true
}

// immovableName reports whether `set` will refuse this name outright, in
// either direction — [Runner.AddImmovableSetOptions] and, with it, the
// negative spelling of an immovable name for being interactive.
//
// The two spellings are one fact and are refused together, which is measured
// rather than inferred: ksh93u+ 2012-08-01 answers `set: interactive: bad
// option(s)` and `set: nointeractive: bad option(s)` in a script, word for
// word, and takes both on its own command line.
func (r *Runner) immovableName(name string) bool {
	if r.immovableOptions[name] {
		return true
	}
	base, ok := r.negativeInteractiveSpelling(name)
	return ok && r.immovableOptions[base]
}

// negatedOption is one option read and written upside down, which is the whole
// of what a negated spelling is: `clobber` is `noclobber` inverted, and
// nothing else about the state moves.
func negatedOption(o setOption) setOption {
	n := setOption{on: !o.on}
	if o.get != nil {
		n.get = func(r *Runner) bool { return !o.get(r) }
	}
	if o.apply != nil {
		n.apply = func(r *Runner, on bool) { o.apply(r, !on) }
	}
	if o.try != nil {
		n.try = func(r *Runner, on bool, spelling string) bool { return o.try(r, !on, spelling) }
	}
	return n
}

// recordedOption is a name this shell lists, remembers, and does not act on.
//
// The third answer beside "we do it" and "we refuse it", and it is the one the
// panel says most of these names want. A shell's `set -o` listing is a capture
// surface — `eval "$(set +o)"` is the save/restore idiom — so a name in the
// listing that `set` will not take is a line the listing writes and the shell
// then rejects, which is worse than a short listing (#3128). Measured
// 2026-09-16 by asking every shell in the panel for every name in its *own*
// listing, in both directions: bash 5.3.20, bash 3.2.57 and dash 0.5.12 refuse
// nothing at all, ksh93u+ refuses exactly the three
// [Runner.AddImmovableSetOptions] already declares, and zsh 5.9.2 refuses the
// five about being interactive that dialect/zsh/setopt.go already names. Every
// other name every one of them lists, it takes.
//
// Recording is not implementing and does not claim to be — dialect/zsh's
// setopt table has made the same bargain for 140 names since it was written,
// and docs/spec/semantics.md says so in the same words. What it promises is
// that the request is remembered and reported back faithfully: `set -o
// markdirs` succeeds, `set -o` then says `markdirs on`, and a glob still marks
// no directory. What it replaces is a complaint on stderr and a status 2 about
// a name the shell had just finished advertising.
//
// A name moves *out* of here the moment something reads it, which is the
// distinction being written down rather than assumed: `keyword` was in this
// class until #3126 built `set -k`, `history` and `histexpand` until #3093,
// and `globstar` until #3152 wired it to the walk. The entries below are the
// ones nothing reads yet.
func recordedOption(name string, def bool) setOption {
	return recordedOverOption(name, def, nil)
}

// recordedOverOption is recordedOption for a name whose state in a shell that
// has not moved it is not a constant.
//
// `bgnice` is the one: ksh93 reports it on at a prompt and off in a script,
// which is the same live fact `interactive` reports, so a constant written
// here would have been wrong for one of the two runs that produced it. The
// base is read on every request rather than captured, and the recorded answer
// replaces it only once a script has asked.
func recordedOverOption(name string, def bool, base func(*Runner) bool) setOption {
	state := func(r *Runner) bool {
		if base != nil {
			return base(r)
		}
		return def
	}
	return setOption{
		on: def,
		apply: func(r *Runner, on bool) {
			if r.recordedOptions == nil {
				r.recordedOptions = make(map[string]bool, 1)
			}
			r.recordedOptions[name] = on
		},
		get: func(r *Runner) bool {
			if on, ok := r.recordedOptions[name]; ok {
				return on
			}
			return state(r)
		},
	}
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
		rows = append(rows, ListedOption{Name: n, On: o.state(r), NegatedName: r.negatedOptions[n]})
	}
	return rows
}

// listedOptionNames is every name this shell answers `set -o` with, in the
// order the dialect publishes them — sorted where it publishes none.
//
// Sorting is the majority and not the rule: bash and ksh93 sort, and dash
// writes its own table's order, which is neither sorted nor the order a
// script set anything in. Measured 2026-09-13 — dash opens `errexit`,
// `noglob`, `ignoreeof` where a sort would open `allexport`, `debug`,
// `emacs` — so `set -o | head` in a dash script reads a different line, and
// a listing is a table with an order rather than a set (#2624).
//
// A name the order does not mention keeps its sorted place after the ones it
// does, which is the same rule orderedOptionLetters follows next door and for
// the same reason: the order is a measurement of the names that were there
// when somebody looked, and a name added later must still come out somewhere.
func (r *Runner) listedOptionNames() []string {
	names := make([]string, 0, len(commonSetOptions)+len(r.extraOptions))
	// A name this dialect lists under a negated spelling is not listed under
	// the substrate's own: ksh93 writes `clobber on` and never `noclobber`,
	// while still *taking* both spellings. So the set is dropped from the
	// roster and the negated names take its place.
	negated := make(map[string]bool, len(r.negatedOptions))
	for _, base := range r.negatedOptions {
		negated[base] = true
	}
	for n := range commonSetOptions {
		if !negated[n] {
			names = append(names, n)
		}
	}
	for n := range r.extraOptions {
		if !negated[n] {
			names = append(names, n)
		}
	}
	for n := range r.negatedOptions {
		names = append(names, n)
	}
	sort.Strings(names)
	order := r.diag().OptionListingOrder
	if len(order) == 0 {
		return names
	}
	out := make([]string, 0, len(names))
	placed := make(map[string]bool, len(order))
	for _, n := range order {
		if slices.Contains(names, n) {
			out = append(out, n)
			placed[n] = true
		}
	}
	for _, n := range names {
		if !placed[n] {
			out = append(out, n)
		}
	}
	return out
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
				switch {
				case r.immovableOptions[row.Name]:
					// This line is a *command*, so a name `set` will not
					// take has no business in it. Measured on ksh93u+: an
					// interactive login shell reports `interactive`,
					// `login_shell` and `rc` in its listing and names none
					// of the three here, while `monitor` — on for the same
					// reason and movable — is on the line. See
					// AddImmovableSetOptions.
				case row.NegatedName != "":
					// A row the shell lists as the opposite of a state it
					// stores. This line names the *stored* states, so the
					// negated spelling appears when the row is off and the
					// row being on says nothing: measured on ksh93u+,
					// `set +o clobber` puts `--noclobber` on the line and a
					// stock shell names neither. See AddNegatedSetOptions.
					if !row.On {
						line += " --" + row.NegatedName
					}
				case row.On:
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

// setControlWords are the `--name` words the `set` builtin takes that are not
// option names, in the one shell that has any. See
// Semantics.SetHasTheStateAndDefaultWords for the measurements and for the
// usage line that advertises them.
var setControlWords = []string{"default", "state"}

// setControlWord reads a `--name` word — dashes already off — as one of those,
// by **unique prefix**, which is measured rather than assumed: `--d`, `--de`
// and `--defa` are all `--default` in ksh93u+ and `--s`, `--st` and `--stat`
// are all `--state`, while an *option* name abbreviates nowhere — `--g`,
// `--gl` and `--glo` are each `bad option(s)` in the same shell, so the
// abbreviation belongs to these two words and not to the namespace beside
// them.
//
// Case is not folded, which is the same shell's answer for an option name and
// is measured on both: `--DEFAULT` and `--GLOBSTAR` are refused alike. An
// empty word is not a prefix of anything here — `set --` is the terminator
// and never reaches this.
//
// A `no` prefix is **not** read off these two. Measured and deliberately not
// modeled: `set --nostate` writes the state line in that shell and
// `set --nodefault` is a silent 0, which is neither the option the word names
// nor its negation, and reproducing it would be copying a surface nothing
// documents. A script writing either gets this shell's `bad option(s)`.
func setControlWord(word string) (string, bool) {
	if word == "" {
		return "", false
	}
	found := ""
	for _, w := range setControlWords {
		if !strings.HasPrefix(w, word) {
			continue
		}
		if found != "" {
			// Two words share the prefix, so it names neither. Unreachable
			// with today's pair — `default` and `state` share no first
			// letter — and written because the ambiguity is the rule a
			// prefix match lives or dies by, not because a row needs it.
			return "", false
		}
		found = w
	}
	return found, found != ""
}

// applySetControlWord answers one of those words, reporting whether it was one
// at all.
//
// The two are answered at different **times**, and that is measured rather
// than chosen. `--default` moves the options where it stands, so a `-o` after
// it survives and one before it does not: `set --default -o errexit` leaves
// errexit on and `set -o errexit; set --default` takes it off. `--state`
// writes nothing here — it asks for the listing this builtin already defers
// to the end of the option parse, which is what makes `set --state -o errexit`
// name errexit on the line, `set --state --default` write the state the reset
// left, and `set --state --state` write one line rather than two. All four
// measured 2026-09-16 on ksh93u+.
//
// The form it asks for is the `+o` one, so the line `--state` writes and the
// line `set +o` writes are the same line by construction rather than by
// agreement — and measurement says they agree in every state probed: a moved
// row, a moved negated row, a recorded row, and after `--default` itself. See
// Semantics.SetListsOptionsOnceAtTheEnd for the deferral this borrows.
func (r *Runner) applySetControlWord(word string) bool {
	name, ok := setControlWord(word)
	if !ok || !r.ask(r.sem().SetHasTheStateAndDefaultWords,
		"`set --state` and `set --default`") {
		return false
	}
	if name == "state" {
		r.pendingOptionListing = listingAsInput
		return true
	}
	r.setOptionsToTheirDefaults()
	return true
}

// setOptionsToTheirDefaults is `set --default`: every option back to the state
// this shell was compiled with.
//
// The dialect's declared rows are the on ones and everything else its listing
// carries goes off — see [Runner.AddDefaultOnSetOptions] for why that is a
// table and not a re-read of the option entries. Written through the same
// seam `set -o NAME` goes through, so a negated spelling is inverted once and
// a recorded name is recorded rather than needing a second path.
//
// It does not touch the positional parameters, which is measured: `set 1 2 3;
// set --default` leaves `$*` as `1 2 3` and `set --default a b` sets them to
// `a b`, so the word is an option and the operands behind it are operands.
// Nothing here has to arrange that — the option loop reaches this and the
// operands are read after it — and it is written down because the opposite is
// the obvious guess about a word named "default".
func (r *Runner) setOptionsToTheirDefaults() {
	for _, name := range r.listedOptionNames() {
		if r.immovableOptions[name] || r.inertOptions[name] {
			// A reset is a request like any other, and these two refuse or
			// swallow one. The three immovable rows in the shell that has
			// this word are facts about the invocation — they move with how
			// the shell was started and a script cannot write them — so a
			// reset that reported them off would be lying about the shell it
			// is running in.
			continue
		}
		r.setNamedOption(name, r.defaultOnOptions[name])
	}
}

// SetLongOption applies one invocation option written as `--name`, the
// spelling ksh93 gives every `set -o` name on its command line. Exported for
// the front end beside SetOptionLetters and SetNamedOption, and returning the
// same thing they do: 0, or the status the dialect gives a name it refuses.
//
// The word arrives without its dashes, and what it holds is decided by
// longSetOptionName. See Semantics.LongOptionNamesASetOption for the rule and
// for why the fallback is tried second.
func (r *Runner) SetLongOption(word string) int {
	r.line = 0
	r.atInvocation = true
	r.longSetOptionSpelling = true
	defer func() {
		r.atInvocation = false
		r.longSetOptionSpelling = false
	}()
	name, on := r.longSetOptionName(word)
	// The word as it was written is what a refusal echoes, not the name the
	// reading arrived at: measured, `ksh --no_profile` is `no_profile: bad
	// option(s)` and `zsh --Zzz_No` is `no such option: Zzz_No`. The reader
	// is shown what they typed rather than what the shell was left holding.
	return r.namedOptionAnswer(r.setNamedOptionSpelled(name, word, on))
}

// longSetOptionName reads a `--name` word — already stripped of its dashes —
// into the option name it asks for and the direction it asks for it in.
//
// Three readings, in the order that keeps them from eating each other:
//
//  1. an `=value` suffix, where the dialect reads one. It is a number rather
//     than a word, so `=on` is the option **off**; see
//     Semantics.LongOptionValueIsANumber. Read first, because the name in
//     front of it still goes through the two readings below.
//  2. the whole word as a name. `notify` is an option in its own right and so
//     is `noglob`, so this has to come before the strip or `--notify` would
//     turn notify off.
//  3. the word with a leading `no` taken off, for the option off. Only when
//     the whole word is not a name, and only when something is left after the
//     `no`: `--no` is a refusal and not `+o ""`.
//
// A word none of the three resolves comes back unchanged and in the `on`
// direction, so the refusal downstream names the word as it was written. That
// is measured: ksh93 answers `--noprofile` with `noprofile: bad option(s)`
// and not with `profile:`.
func (r *Runner) longSetOptionName(word string) (name string, on bool) {
	name, on = word, true
	if before, value, ok := strings.Cut(name, "="); ok &&
		r.sem().LongOptionValueIsANumber == Yes {
		name = before
		// A number, and nonzero is on. Anything that is not one — a word, an
		// empty value — is the option off, which is what strtol leaves
		// behind when it reads nothing.
		n, err := strconv.Atoi(value)
		on = err == nil && n != 0
	}
	// The word with its hyphens taken out, where the dialect reads one that
	// way — a rule of this spelling and of no other route to the same
	// namespace. See Semantics.LongOptionNameIgnoresHyphens.
	//
	// Tried before the word as written, so that a name holding a hyphen
	// still wins where the folded form is not a name at all. No dialect that
	// answers the axis has such a name today, and the order is what keeps
	// that from being a rule nobody wrote down.
	candidates := [...]string{name, name}
	if r.sem().LongOptionNameIgnoresHyphens == Yes {
		candidates[0] = strings.ReplaceAll(name, "-", "")
	}
	for _, candidate := range candidates {
		if r.hasSetOptionName(candidate) {
			return candidate, on
		}
		if rest, ok := strings.CutPrefix(candidate, "no"); ok && rest != "" &&
			r.hasSetOptionName(rest) {
			return rest, !on
		}
	}
	return name, on
}
