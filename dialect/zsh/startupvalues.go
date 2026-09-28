// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strconv"

	"github.com/blairham/sh/interp"
)

// The names this shell starts with a value in and this one had no parameter
// for at all.
//
// A parameter that is *missing* is a different fault from a parameter
// described with the wrong word — `${+NAME}` is 0 rather than 1, and no
// attribute table can be wrong about a name that is not there. #4866 is the
// ledger of the first kind: 37 of the 123 names a fresh `zsh -f` lists under
// `typeset +` had no parameter here, and this file is the group of them whose
// value is a **constant the shell being modeled starts with**, so that
// supplying it is a measurement rather than a feature.
//
// Swept 2026-09-27 against `/opt/homebrew/bin/zsh`, `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)` — `go version -m` says *not a Go executable*
// for it and `github.com/blairham/sh/cmd/zsh` for ours, so the two columns
// are two programs — one script listing every name, run `-f` from a script
// file under `env -i PATH=/usr/bin:/bin` with a scratch `HOME`, both shells
// in the same run:
//
//	name            ${(t)}          typeset -p                     here, before
//	FUNCNEST        integer-special typeset -i10 FUNCNEST=500      no such name
//	KEYTIMEOUT      integer         typeset -i KEYTIMEOUT=40       no such name
//	LISTMAX         integer         typeset -i LISTMAX=100         no such name
//	MAILCHECK       integer         typeset -i MAILCHECK=60        no such name
//	SAVEHIST        integer-special typeset -i10 SAVEHIST=0        no such name
//	TIMEFMT         scalar          typeset TIMEFMT='%J  %U …'     no such name
//	TMPPREFIX       scalar          typeset TMPPREFIX=/tmp/zsh     no such name
//	KEYBOARD_HACK   scalar-special  typeset KEYBOARD_HACK=''       no such name
//	OPTARG          scalar-special  typeset OPTARG=''              no such name
//	PS3             scalar-special  typeset PS3='?# '              no such name
//	SPROMPT         scalar-special  typeset SPROMPT='zsh: corr…'   no such name
//
// **The base is part of the answer and it is not the same for all five
// integers**: the reference writes `-i` with no base for `KEYTIMEOUT`,
// `LISTMAX` and `MAILCHECK` and `-i10` for `SAVEHIST`, which is the same
// split [interp.Runner.SetIntegerParameter] states — base ten written down is
// what separates one of the shell's *own* integers from one a script
// declared, and the three that are not `special` are not the shell's in that
// sense either. The `special` column and the base column agree on every row
// above, and that agreement is a finding rather than a rule to derive one
// from: `KEYTIMEOUT` is an ordinary parameter the shell happens to ship a
// value in.
//
// # The environment supplies nine of the ten, and the tenth is the control
//
// Measured the same day, one shell per cell, `env -i PATH=/usr/bin:/bin`
// plus the one name:
//
//	env KEYTIMEOUT=7     export -i KEYTIMEOUT=7       integer-export
//	env SAVEHIST=7       export -i10 SAVEHIST=7       integer-export-special
//	env TIMEFMT=7        export TIMEFMT=7             scalar-export
//	env PS3=7            export PS3=7                 scalar-export-special
//	env KEYBOARD_HACK=7  typeset KEYBOARD_HACK=''     scalar-special
//
// So the value a shell is handed wins for nine of them — the attributes stay
// and `export` joins them — and `KEYBOARD_HACK` is a name the environment may
// not supply, which is the row `$UID` and `$IFS` are on and the reason
// [interp.Runner.SetSpecial] exists. Without that row this would have been
// written as "the environment always wins", which is true of every name here
// but one.
//
// **An inherited integer is scanned rather than evaluated**, which is the
// same reading `HISTSIZE` takes and is why importedInteger is shared
// with it rather than written twice. Measured over `KEYTIMEOUT`:
//
//	2x → 2     1+1 → 1    " 2 " → 2   0x2 → 2   -1 → -1
//	08 → 0     abc → 0    "" → 0      3.9 → 3
//
// A leading integer, base prefix and all, and the scan stops at the first
// character that is not part of one. `-1` is the row that says this is not
// the history size's reading: that one floors at one, and these do not.
//
// # What is here is the value and not always the behavior
//
// Said in place rather than left to be found, because a parameter that exists
// and is ignored is its own kind of wrong answer:
//
//   - `SAVEHIST` is read — see fcSaveHistOn — and zero is what unset already
//     meant, so this changes no save.
//   - `TIMEFMT` **is** read back since #4910 — the per-command line is
//     rendered through it, in the vocabulary of its own that
//     interp/timeformatunits.go implements.
//   - `TMPPREFIX` is a value only, deliberately: `=(cmd)` resolves its scratch
//     through `TMPDIR` and interp/procsubst.go argues that choice in place.
//   - `KEYTIMEOUT`, `LISTMAX` and `MAILCHECK` are line-editor and mail knobs
//     this shell has no reader for yet, and each is a **feature** rather than
//     a read-back: the key-sequence wait this shell deliberately does not
//     make is #1427, nothing asks before listing a completion, and there is
//     no mail check at all. Measured 2026-09-27, none of the three has a
//     scripted surface in the reference either — a non-interactive shell
//     that sets all three behaves identically with and without them — so
//     none can be graded by a corpus case.
//
// Those three readers are the rest of #4910. `FUNCNEST` is here with
// its reader rather than without one, which is what #4905 asked for: the
// reference's bound *is* that parameter, and a name that looks like a bound a
// script can move while moving nothing is worse than an absent one. See
// funcnesting.go for the bound, the sentence and what the refusal costs.
//
// The other twenty-five names of #4866's ledger are filed as the jobs they
// are rather than carried here: the nine identity values (#4903), the three
// counters (#4904), `$signals` (#4906), the `WATCH`/`watch` pair whose type
// word does not say `tied` (#4907), the evaluation-context pair and the stack
// under it (#4908), and the seven `zsh/parameter` tables (#4909).
func registerTheStartupValues(r *interp.Runner) {
	for _, p := range startupScalars {
		if _, inherited := r.GetVar(p.name); !inherited || !p.environmentMaySupply {
			r.SetSpecial(p.name, p.value)
		}
		if p.shellsOwn {
			r.MarkShellOwnParameter(p.name)
		}
	}
	for _, p := range startupIntegers {
		value := p.value
		if inherited, ok := r.GetVar(p.name); ok {
			value = importedInteger(inherited)
		}
		// The value first and the attribute after it, which is the order
		// fcStartSize states and for the same reason: with the attribute
		// already on, laying the parameter down is an *evaluation*, and this
		// shell's own startup becomes the last sum a script did not do.
		r.SetSpecial(p.name, strconv.Itoa(value))
		r.SetIntegerParameter(p.name, p.base)
		if p.shellsOwn {
			r.MarkShellOwnParameter(p.name)
		}
	}
	registerTheLastStatus(r)
}

// startupScalar is one name this shell starts with a string in.
type startupScalar struct {
	name  string
	value string
	// shellsOwn is the `special` word in `${(t)NAME}`, which is the shell
	// maintaining the parameter rather than a script having made it. Measured
	// per name: `PS3` has it and `TIMEFMT`, sitting beside it in the same
	// listing, does not.
	shellsOwn bool
	// environmentMaySupply is whether a value the shell was *handed* wins.
	// True for all but `KEYBOARD_HACK`, and it is a fact about each name
	// rather than a property of the group — see the table above.
	environmentMaySupply bool
}

var startupScalars = [...]startupScalar{
	// The selection prompt, and the half of a pair that was missing rather
	// than the whole parameter: `PROMPT3` has been produced over `PS3` since
	// promptnames.go, so a shell with no `PS3` answered `$PROMPT3` with
	// nothing where the reference writes `?# `.
	{name: "PS3", value: "?# ", shellsOwn: true, environmentMaySupply: true},
	// The spelling-correction prompt. Not tied to any `SPS1` — measured in
	// promptnames.go, which has the three pairs whose names say they should
	// be and are not.
	{name: "SPROMPT", value: "zsh: correct '%R' to '%r' [nyae]? ", shellsOwn: true, environmentMaySupply: true},
	// `getopts` writes this name and the reference has it before any `getopts`
	// has run — an empty scalar rather than an absent one, which is what a
	// script testing `${OPTARG:-…}` sees.
	{name: "OPTARG", value: "", shellsOwn: true, environmentMaySupply: true},
	// The one the environment may not supply.
	{name: "KEYBOARD_HACK", value: "", shellsOwn: true, environmentMaySupply: false},
	{name: "TIMEFMT", value: "%J  %U user %S system %P cpu %*E total", shellsOwn: false, environmentMaySupply: true},
	{name: "TMPPREFIX", value: "/tmp/zsh", shellsOwn: false, environmentMaySupply: true},
}

// startupInteger is one name this shell starts with a number in, and the base
// its listing writes beside the letter.
type startupInteger struct {
	name  string
	value int
	// base is ten for the shell's own integers and zero — no base at all —
	// for the three that list as a bare `-i`. See the table above.
	base      int
	shellsOwn bool
}

var startupIntegers = [...]startupInteger{
	// The bound on function nesting, and the one name in this table that is
	// a **bound** rather than a knob: the parameter, the bound being it, the
	// sentence that names it and the refusal ending the script arrive
	// together in funcnesting.go, which is what #4905 asked for and why this
	// row was held back from #4912.
	{name: "FUNCNEST", value: 500, base: 10, shellsOwn: true},
	{name: "KEYTIMEOUT", value: 40, base: 0, shellsOwn: false},
	{name: "LISTMAX", value: 100, base: 0, shellsOwn: false},
	{name: "MAILCHECK", value: 60, base: 0, shellsOwn: false},
	{name: "SAVEHIST", value: 0, base: 10, shellsOwn: true},
}
