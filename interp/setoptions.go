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
	// Every one of them is off here: we do not defer a job notice, do not
	// hold the session open at end-of-file, and have no vi mode.
	"notify":    {},
	"ignoreeof": {},
	"nolog":     {},
	"vi":        {},
	// The exception, and it is on: the line editor reads ^A, ^E and ^B,
	// which is what this name means.
	"emacs": {on: true},
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
	// zsh's histignoredups, which its `set -h` abbreviates. It governs a
	// history this shell does not keep, so either state is kept truthfully.
	"histignoredups": {
		apply: func(r *Runner, on bool) { r.histIgnoreDups = on },
		get:   func(r *Runner) bool { return r.histIgnoreDups },
	},

	"posix":      {},
	"errtrace":   {},
	"functrace":  {},
	"history":    {},
	"histexpand": {},
	"keyword":    {},
	"onecmd":     {},
	"physical":   {},
	"privileged": {},
}

// setMonitor is `set -m`, the one request in the table a dialect can refuse:
// two of the panel tie job control to the terminal, and this runner only has
// one when a front end said so (JobControl).
//
// Measured with no terminal, which is what a script has: bash and ksh93
// grant it silently — background jobs already run in process groups of their
// own here, so there is nothing further to promise — dash remarks `can't
// access tty; job control turned off` and reports success with the option
// left off, and zsh refuses at 1, fatally, echoing the spelling that asked.
// Turning it *off* is granted everywhere.
func (r *Runner) setMonitor(on bool, spelling string) bool {
	if !on {
		r.monitor = false
		return true
	}
	if !r.JobControl && r.ask(r.sem().MonitorNeedsATerminal, "`set -m` in a shell with no terminal") {
		d := r.diag()
		r.diagf("%s\n", Wording(d.MonitorDenied, "set: cannot turn on job control without a terminal", spelling))
		if d.MonitorDeniedStatus == 0 {
			// A remark rather than a failure: the option is left off and
			// `set` still reports success, which is dash's shape.
			return true
		}
		r.setOptionStatus = d.MonitorDeniedStatus
		if r.ask(r.sem().BadSetOptionNameFatal, "a refused `set -m` ending the script") {
			r.status = d.MonitorDeniedStatus
			r.fatalQuiet()
		}
		return false
	}
	if r.unspecified {
		return false
	}
	r.monitor = true
	return true
}

// SetOptionLetters applies a run of single-letter options — `e` and `ux`
// from `-e` and `+ux` — exactly as `set` reads them, reporting whether every
// letter was one this shell has. A refusal has already been said on the
// runner's error stream.
//
// Exported for the front end: every shell in the panel accepts its `set`
// options at invocation too — `sh -e script.sh` is an everyday spelling —
// and routing them through the same machinery is what keeps `sh -f` and
// `set -f` the same question with the same dialect answer.
func (r *Runner) SetOptionLetters(letters string, on bool) bool {
	// No script line has run yet; a dialect prelude may have. bash reports
	// an invocation option's failure at "line 0", and a diagnostic naming
	// the prelude's last line would point somewhere nobody wrote.
	r.line = 0
	ok := r.setLetters(letters, on)
	if r.unspecified {
		// A letter that hangs on an axis no dialect answered — `-f` under
		// core — has been refused out loud. Inside a script the run loop
		// reads this flag; at invocation the caller only gets the bool, so
		// it is folded in here and cleared the way the run loop clears it.
		r.unspecified = false
		return false
	}
	return ok
}

// SetNamedOption applies one long option — `-o pipefail`, `+o allexport` —
// exactly as `set -o` reads it, returning 0 or the status the dialect gives
// a name it refuses. Exported for the front end, with SetOptionLetters.
func (r *Runner) SetNamedOption(name string, on bool) int {
	r.line = 0
	return r.ApplyNamedOption(name, on)
}

// ApplyNamedOption is SetNamedOption from inside a running script: a
// registered builtin presenting the same options under its own names routes
// through here, where the line a complaint would name is the one being run
// rather than the zero an invocation reports.
func (r *Runner) ApplyNamedOption(name string, on bool) int {
	ok := r.setOption(name, on)
	if r.unspecified {
		r.unspecified = false
		return 2
	}
	if !ok {
		return r.setOptionFailure()
	}
	return 0
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
	names := r.listedOptionNames()
	if plus {
		if r.diag().PlusOListsActive {
			line := "set --default"
			for _, n := range names {
				o, _ := r.lookupSetOption(n)
				if o.state(r) {
					line += " --" + n
				}
			}
			r.printf("%s\n", line)
			return 0
		}
		for _, n := range names {
			o, _ := r.lookupSetOption(n)
			sign := "+"
			if o.state(r) {
				sign = "-"
			}
			r.printf("set %so %s\n", sign, n)
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
	for _, n := range names {
		o, _ := r.lookupSetOption(n)
		state := "off"
		if o.state(r) {
			state = "on"
		}
		r.printf("%-*s%s%s\n", width, n, sep, state)
	}
	return 0
}
