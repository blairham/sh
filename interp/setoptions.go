// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

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

	// on is the state this shell is already in for a name it does not
	// implement, so that asking for that state can succeed honestly.
	//
	// Not a claim about what any *other* shell defaults to. bash has
	// `hashall` on and we do not hash at all, so ours is off and a script
	// turning it off gets what it asked for.
	on bool
}

// commonSetOptions are the names every shell in the panel has. They are the
// core's, and no dialect has to declare them.
var commonSetOptions = map[string]setOption{
	"errexit":   {apply: func(r *Runner, on bool) { r.errexit = on }},
	"nounset":   {apply: func(r *Runner, on bool) { r.nounset = on }},
	"xtrace":    {apply: func(r *Runner, on bool) { r.xtrace = on }},
	"noclobber": {apply: func(r *Runner, on bool) { r.noclobber = on }},
	"noglob":    {apply: func(r *Runner, on bool) { r.noglob = on }},
	"allexport": {apply: func(r *Runner, on bool) { r.allexport = on }},

	// The rest of the unanimous names, none of which this shell has yet.
	// Every one of them is off here: we do not stop before running, do not
	// echo what we read, do not defer a job notice, do not hold the session
	// open at end-of-file, and have no vi mode.
	"noexec":    {},
	"verbose":   {},
	"notify":    {},
	"ignoreeof": {},
	"monitor":   {},
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

	"posix":      {},
	"errtrace":   {},
	"functrace":  {},
	"history":    {},
	"histexpand": {},
	"hashall":    {},
	"keyword":    {},
	"onecmd":     {},
	"physical":   {},
	"privileged": {},
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
