// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"sort"
	"strings"
)

// The set options as a variable, and as a startup input.
//
// One shell in the panel keeps the long option names in a variable, and it is
// not a variable that happens to be read at startup. It is bound to the option
// state in both directions: `set -x` changes what it says, and what it says at
// startup changes the options. It is readonly, and it is *produced* rather
// than stored — so it is never what was put in, and no assignment can make it
// lie.
//
// The core keeps it for the reason it keeps `$-`: what the options are is the
// runner's, and only the name is a dialect's. With no name from the dialect
// there is no variable and nothing is seeded, which is what the other three
// shells do with the same environment entry — measured, they leave it as an
// ordinary string and read nothing out of it.
//
// It is the sharpest startup input a shell takes. An inherited `xtrace`
// changes what every non-interactive shell below it writes to standard error,
// which is why the seeding is a deliberate call by the front end rather than
// something a Runner does to its embedder behind their back — and why it is
// the *last* thing startup does; see ApplyInheritedShellOptions.

// SetShellOptions exposes the `set -o` names that are on under a name, as a
// readonly produced parameter. A dialect calls it from Apply, the same seam
// that names the pipeline-status record and the regex captures.
//
// Produced rather than stored, and that is the whole design. A stored copy
// would be the options the shell started with, so `set -x; echo $SHELLOPTS`
// would tell a script something untrue — which is the one failure mode this
// variable has that plain absence does not.
//
// Readonly here rather than in the dialect, because the two halves are one
// fact: a name whose value is produced cannot be assigned to meaningfully, and
// a shell that accepted the assignment silently would be worse than one that
// refuses it. What the refusal *says*, and whether it ends the script, is the
// dialect's as it is for any other readonly name.
//
// An empty name is not guarded against, the same way the two records beside
// this one do not guard: a dialect that names nothing simply never calls this,
// and a guard here would be a line no test could distinguish from its absence.
// The one place the name is *used* checks it, which is where it matters.
func (r *Runner) SetShellOptions(name string) {
	r.shellOptsName = name
	r.SetOptionList(name, (*Runner).shellOptions, (*Runner).applyInheritedSetOptions)
}

// optionList is one produced, readonly variable bound to an option namespace:
// what it says, and what an inherited value of it does.
type optionList struct {
	name    string
	value   func(*Runner) string
	inherit func(*Runner, string)
}

// SetOptionList names a produced, readonly variable holding the names that
// are on in one option namespace, and says how a value inherited from the
// environment is read back into it. SetShellOptions is this for the namespace
// the core owns, and the general form exists because a dialect may have a
// second one the core knows nothing about: bash's `shopt` names are that
// shell's own table, and `$BASHOPTS` is bound to them in both directions
// exactly as `$SHELLOPTS` is bound to `set -o` (#2475).
//
// Registering rather than special-casing is what keeps the three things a
// bound variable needs in one place. It is produced, so a script never reads
// a stale copy; it is readonly, so nothing can make it lie; and the value a
// *child* is handed is recomputed at the moment of the exec rather than
// inherited — see Runner.environ, where a second namespace written as a
// second `if` would have been the one that quietly kept its startup string.
func (r *Runner) SetOptionList(name string, value func(*Runner) string, inherit func(*Runner, string)) {
	r.optionLists = append(r.optionLists, optionList{name: name, value: value, inherit: inherit})
	r.SetDynamic(name, value)
	if r.readonly == nil {
		r.readonly = map[string]bool{}
	}
	r.readonly[name] = true
}

// producedOptionList answers with the live value of a bound option variable,
// and whether the name is one. Read where a child's environment is built.
func (r *Runner) producedOptionList(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	for _, l := range r.optionLists {
		if l.name == name {
			return l.value(r), true
		}
	}
	return "", false
}

// exportedOptionLists is the environment entry a bound option record earns
// when the *script* exported it rather than inheriting it, and `done` names the
// ones the walk over the inherited environment has already written.
//
// A pass of its own for the reason exportedTables has one: the name is
// produced and readonly, so it is in no map any other pass walks — the walk
// over Vars cannot see it, and the walk over Env sees it only where it came in
// that way. Without this, `set -o noglob; export SHELLOPTS` records the
// attribute, reads back as `declare -rx`, and hands a child nothing at all —
// which is the opposite of what the script asked, at status 0, and `export
// SHELLOPTS` is the documented way to make it stick because assigning to the
// variable is refused (#4188).
//
// Sorted, for the reason zeroValuedTypeExports is sorted: a child's
// environment must not depend on the order a slice happened to be built in.
// The slice is already in registration order, which is stable, and this says
// so rather than relying on it.
func (r *Runner) exportedOptionLists(done map[string]bool) []string {
	var out []string
	for _, l := range r.optionLists {
		if done[l.name] || !r.isExported(l.name) {
			continue
		}
		out = append(out, l.name+"="+l.value(r))
	}
	sort.Strings(out)
	return out
}

// shellOptions is the value: every long option name this shell has on, sorted
// and colon-separated.
//
// Sorted and long-named because that is what comes back out of the shell that
// has this — what went in is never what comes out, and a script comparing the
// whole string against what it exported would be comparing against a spelling
// no shell produces. Membership is the contract, exactly as it is for `$-`:
// `case ":$SHELLOPTS:" in *:xtrace:*)` is the question this variable answers.
//
// The names are this shell's own state and not a claim about anyone else's.
// Reporting one the other way round to match a listing would be the lie this
// file exists to avoid. `hashall` was the standing example of a default that
// differs, on the grounds that nothing was hashed; since #2554 something is,
// and in bash the name reads `on` here exactly as it does there — the state
// comes from the startup letter `h` rather than from a constant, and `set +h`
// really stops the table being filled.
//
// `emacs` was the second until #1858, and it was the wrong kind of honesty:
// the editor does read those keys, but a *script* has no line to edit, and no
// shell in the panel reports a keymap selected until it is interactive — the
// one that ever selects one on its own being bash. So the name is out of this
// value in a script now, and in it under `-i` in that dialect, which is what
// the real shell writes.
func (r *Runner) shellOptions() string {
	names := r.listedOptionNames()
	on := make([]string, 0, len(names))
	for _, n := range names {
		o, _ := r.lookupSetOption(n)
		if o.state(r) {
			on = append(on, n)
		}
	}
	// listedOptionNames is already sorted, so this is a re-assertion rather
	// than work — kept because the ordering is the contract and a later
	// change to how the names are gathered must not quietly drop it.
	sort.Strings(on)
	return strings.Join(on, ":")
}

// ApplyInheritedShellOptions turns on every option named in the value this
// shell inherited, which is the write direction of the binding.
//
// The front end calls it, and calls it **last**, which is measured rather than
// chosen: an inherited `xtrace` beats the invocation's own `+x`, so the
// environment is read after the argument vector rather than before it. It is a
// call and not something the Runner does for itself because it is a startup
// action, and a library Runner handed an environment is not entitled to change
// its embedder's options on the strength of a name in it.
//
// A name the shell does not have is refused in the dialect's own words and the
// shell carries on — measured, an unknown name in the value produces the
// invalid-option-name complaint at line 0 and every good name in the same
// value is still applied. Nothing is turned *off*: the value says what is on,
// and a shell whose defaults differ from what it was handed does not lose them.
//
// Only the *inherited* value is read, never one a script assigned. There can be
// no such value — the name is readonly and produced — but reading the live
// variable here would still be the wrong question: what is being asked is what
// this shell was launched with.
func (r *Runner) ApplyInheritedShellOptions() {
	// Every bound namespace, not only the core's. No dialect naming one means
	// there is nothing to read out of the environment, which is the answer for
	// three of the four presets and is why the entry is left an ordinary
	// string for them exactly as it is in the shells they name.
	for _, l := range r.optionLists {
		value, ok := r.inheritedValue(l.name)
		if !ok || value == "" {
			continue
		}
		// No script line has run, and a complaint about the environment names
		// line 0 the way an invocation option's does. The prelude may have
		// moved the counter, so it is put back rather than assumed.
		r.line = 0
		r.fromEnvironment = true
		l.inherit(r, value)
		r.fromEnvironment = false
	}
}

// applyInheritedSetOptions is the write direction for the namespace the core
// owns: every `set -o` name in the value is turned on.
func (r *Runner) applyInheritedSetOptions(value string) {
	for _, name := range strings.Split(value, ":") {
		// An empty piece is a name of nothing rather than a separator to
		// skip, which is measured: a value with a leading or trailing colon
		// draws the same complaint an unknown name does.
		//
		// ApplyNamedOption and not SetNamedOption, which is a measured
		// wording difference: the shell that has this variable names itself
		// twice when it refuses an invocation's `-o name` — once as the
		// prefix and once where the builtin's name would stand — and names
		// itself once here, with nothing where `set` would be. The flag above
		// is what tells the refusal which of the three it is.
		r.ApplyNamedOption(name, true)
	}
}
