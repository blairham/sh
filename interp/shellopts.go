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
	r.SetDynamic(name, (*Runner).shellOptions)
	if r.readonly == nil {
		r.readonly = map[string]bool{}
	}
	r.readonly[name] = true
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
// One default differs from the shell that has the variable, and it is already
// recorded in setoptions.go: `hashall` is off here because nothing is hashed.
// Reporting it the other way round to match a listing would be the lie this
// file exists to avoid.
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
	if r.shellOptsName == "" {
		// No dialect named it, so there is no such variable and nothing to
		// read out of the environment. This is the answer for three of the
		// four presets, and it is why the entry is left an ordinary string
		// for them exactly as it is in the shells they name.
		return
	}
	value, ok := r.inheritedValue(r.shellOptsName)
	if !ok || value == "" {
		return
	}
	// No script line has run, and a complaint about the environment names
	// line 0 the way an invocation option's does. The prelude may have moved
	// the counter, so it is put back rather than assumed.
	r.line = 0
	r.fromEnvironment = true
	defer func() { r.fromEnvironment = false }()
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
