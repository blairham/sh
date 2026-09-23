// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"
	"strings"
)

// Restricted mode: the shell one of the panel enters on `set -r`, where a
// script may still compute but may no longer reach past the directory and the
// PATH it was handed.
//
// It is not a security boundary and this file does not claim to be one — a
// restricted shell hands out no privilege and withholds none, and anything it
// *can* start can do whatever it likes. What it is is a named, measured set of
// refusals, and scripts written against it read the refusals: a restricted
// shell that quietly did the thing anyway would report 0 from `cd` and leave
// the caller believing it had stayed where it was.
//
// The whole of it is ten refusals, and every one of them was measured on bash
// 5.3.20 rather than read out of a manual — the wording, the status and which
// spellings are reached. They are, with the site each lives at:
//
//	cd                                        biCd
//	assigning one of the frozen names         enterRestricted, through the freeze
//	a command word with a `/` in it           Runner.exec
//	`.` or `source` on a path with a `/`      biDot
//	a redirection that opens a file to write  Runner.applyRedirs
//	`exec cmd`                                Runner.replaceSelf
//	`command -p cmd`                          biCommand
//	`hash -p path name`                       biHash
//	an assignment to the hash's parameter     Runner.RestrictedHashPath
//	`enable -d`                               biEnable
//
// Nine of the ten report **1** and leave the shell running, which is uniform
// and is measured rather than assumed: each was run in a restricted shell with
// `echo st=$?` behind it, and none of them is 126, 127 or 2, and none of them
// ends the script. The tenth is an assignment and reports an assignment's 0
// while hashing nothing — see Runner.restrictedHashEntry, where the two halves
// of that pair are set beside each other.
//
// **What is not refused** is as measured as what is. A descriptor duplication
// is not a redirection that opens anything — `echo x >&2` and `2>&1` are
// silent at 0 — and neither is a here-document, a here-string, or any form of
// input. `enable name` and `enable -n name` are taken; only the form that
// would load or unload a builtin is refused. `command -p` with no operand
// behind it is taken, because nothing would be searched for.

// enterRestricted puts this shell in restricted mode.
//
// Whether it can be left again is the dialect's, and the two shells that have
// the mode disagree: bash refuses `set +r` outright and ksh93 grants it and
// hands everything back. See Semantics.RestrictedModeIsLeftByTheLetter.
//
// Idempotent, because `set -r` in a shell that is already restricted is a
// request for the state it is in and every such request is granted: measured,
// it is silent at 0 and the freeze below is not applied twice.
func (r *Runner) enterRestricted() {
	if r.restricted {
		return
	}
	r.restricted = true
	for _, name := range r.restrictedFrozenVariables() {
		if r.restrictedFrozen == nil {
			r.restrictedFrozen = map[string]bool{}
		}
		// Recorded as well as frozen, because a dialect that words the
		// refusal apart from a readonly's needs to know which names the
		// *mode* froze — see Runner.restrictedFreeze.
		r.restrictedFrozen[name] = true
		// The ordinary freeze rather than a check of its own at every
		// assignment site, which is the measurement and not a shortcut:
		// `PATH=/bin` in a restricted bash is `PATH: readonly variable` —
		// the same sentence a `readonly PATH` earns — and `unset PATH` is
		// `unset: PATH: cannot unset: readonly variable`. Two spellings of
		// one state, so the state is what is written.
		//
		// A name with no value is frozen all the same, which is also
		// measured: `readonly -p` in a restricted shell with no `$ENV` set
		// lists `declare -r ENV`, a frozen name holding nothing.
		r.markReadonly(name)
	}
}

// leaveRestricted hands back what the mode was withholding, for the one shell
// that lets a script ask.
//
// Measured 2026-09-22 on ksh93u+ 2012-08-01 from a script file: `set -r; set
// +r` is silent at 0, `$-` loses the letter, and `cd /`, `PATH=/bin` and
// `/bin/echo hi` are each taken afterwards — so this is the mode genuinely
// lifted and not the letter alone. The long spelling is refused in the same
// shell, which is why the two doors are not one: see the `restricted` entry in
// setoptions.go.
//
// The freeze is lifted only for the names the *mode* froze. A script that had
// already written `readonly PATH` of its own keeps it, which is what makes the
// record worth keeping rather than re-deriving the list.
func (r *Runner) leaveRestricted() {
	if !r.restricted {
		return
	}
	r.restricted = false
	for name := range r.restrictedFrozen {
		// The mark directly rather than removeReadonly, which asks a dialect
		// whether `typeset +r` may take the attribute off: this is not a
		// script asking to unfreeze a name, it is the mode giving back what
		// the mode took, and the shell that allows it is not the shell that
		// allows the attribute to be removed.
		delete(r.readonly, name)
	}
	r.restrictedFrozen = nil
}

// restrictedFreeze reports whether a name is one this mode froze *and* the
// dialect words that refusal apart from an ordinary readonly's.
//
// Two shells freeze names here and they do not say the same thing about them.
// bash makes them readonly outright — `PATH=/bin` is `PATH: readonly
// variable`, `readonly -p` lists them, and the two spellings cannot disagree
// because there is one state. ksh93 keeps its own sentence, `PATH:
// restricted`, and lists nothing under `readonly -p`; measured 2026-09-22.
//
// The mechanism is shared all the same — the names are marked readonly in both
// — because the status and the fatality of a refused assignment are a readonly
// assignment's in both shells, and those are measured facts that a second
// mechanism would have to reproduce by hand. What the axis moves is the
// sentence and the listing. See Semantics.RestrictedFreezeIsAReadonly.
func (r *Runner) restrictedFreeze(name string) bool {
	if !r.restrictedFrozen[name] {
		return false
	}
	return !r.ask(r.sem().RestrictedFreezeIsAReadonly,
		"a restricted shell's frozen names being readonly names")
}

// restrictedFrozenVariables are the names a restricted shell may no longer
// assign, because every one of them is a way to reach a command the shell
// would otherwise not run.
//
// PATH and SHELL are where a command comes from, and the three startup names
// are files the *next* shell reads. Measured on bash 5.3.20 by asking
// `readonly -p` in a restricted shell: PATH, SHELL, ENV, BASH_ENV and
// HISTFILE are frozen and nothing else is.
//
// Three of them are written here, the fourth is asked of the dialect's
// semantics and the rest are the dialect's own, which is the split the
// measurement asks for: PATH, SHELL and ENV are POSIX's own names and are
// frozen in both shells that have the mode, the non-interactive startup
// variable is one the shell picks for itself — see
// Semantics.NonInteractiveStartupVariable — and beyond those the sets differ.
// Measured 2026-09-22: bash freezes HISTFILE and ksh93 does not; ksh93 freezes
// FPATH, where a function comes from, and bash has no such name. Neither is
// derivable from the other, so each dialect names its own through
// Runner.FreezeInRestrictedMode.
func (r *Runner) restrictedFrozenVariables() []string {
	names := []string{"PATH", "SHELL", "ENV"}
	if name := r.sem().NonInteractiveStartupVariable; name != "" && !slices.Contains(names, name) {
		names = append(names, name)
	}
	for _, name := range r.restrictedFreezes {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// FreezeInRestrictedMode names what else this shell's restricted mode freezes,
// beyond the three POSIX names and the non-interactive startup variable.
//
// A list and not an axis because it is a list: bash freezes HISTFILE and ksh93
// freezes FPATH, and neither shell has the other's name at all, so there is no
// question with two answers to ask. Measured by asking each shell in the mode
// — `readonly -p` in bash, one assignment at a time in ksh93, which lists
// nothing.
func (r *Runner) FreezeInRestrictedMode(names ...string) {
	r.restrictedFreezes = append(r.restrictedFreezes, names...)
}

// restrictedRefusal writes `<what>: restricted` and the status every refusal
// in this file reports.
//
// The subject is the *builtin* for the three that refuse themselves outright
// — `cd`, `exec` and `enable` — which is what makes this the one-argument
// form: there is no operand worth naming when the whole command is the thing
// refused.
func (r *Runner) restrictedRefusal(what string) int {
	r.diagf("%s\n", Wording(r.diag().Restricted, "%[1]s: restricted", what))
	return restrictedStatus
}

// restrictedOperandRefusal writes `<builtin>: <operand>: restricted`, for the
// three that refuse one word of an otherwise ordinary command.
//
// The operand is named because it is what the script has to change: `command
// -p ls` is refused over the letter and not over `ls`, and `. /etc/profile`
// over the slash and not over the file being unreadable. A sentence without
// it would send a script looking at the wrong word.
func (r *Runner) restrictedOperandRefusal(name, operand string) int {
	r.diagf("%s\n", Wording(r.diag().RestrictedOperand, "%[1]s: %[2]s: restricted", name, operand))
	return restrictedStatus
}

// endAfterRestrictedBuiltinRefusal ends the script where the dialect says one
// of the three builtin refusals does, and answers the status either way.
//
// Called after the sentence is written rather than instead of it: both shells
// say the same words and only one of them stops. See
// Semantics.RestrictedBuiltinRefusalIsFatal for which three and for why the
// split is not the special-builtin rule.
func (r *Runner) endAfterRestrictedBuiltinRefusal() int {
	if r.ask(r.sem().RestrictedBuiltinRefusalIsFatal,
		"restricted mode's refusal of `.`, `exec` or `command -p` ending the script") {
		r.fatalQuiet()
		return r.status
	}
	if r.unspecified {
		return r.status
	}
	return restrictedStatus
}

// restrictedStatus is what every refusal in this file reports.
//
// One number for the nine because that is what was measured, and it is
// deliberately not the 126 or 127 a command that would not start reports: a
// restricted shell's refusal is the *shell* declining, and a script that
// branched on 127 would read it as "no such command" and go looking for one.
const restrictedStatus = 1

// restrictedCommandName is the refusal a command word with a `/` in it earns,
// and it is the one restriction that is about the *name* rather than about a
// builtin.
//
// Before the PATH search rather than after, which is measured: `/no/such` in
// a restricted shell is this sentence and not `No such file or directory`, so
// the shell never looks. That ordering is the whole point of the rule — the
// search is exactly what a restricted shell is confining, and a refusal that
// came after it would already have told the script what is on the disk.
func (r *Runner) restrictedCommandName(name string) int {
	r.diagf("%s\n", Wording(r.diag().RestrictedCommandName,
		"%[1]s: restricted: cannot specify `/' in command names", name))
	return restrictedStatus
}

// restrictedRedirect is the refusal a redirection that would open a file to
// write earns, naming the target word as the script wrote it.
//
// The word and not the path it resolves to, measured: `echo x > f` in a
// directory that is not the root names `f`.
func (r *Runner) restrictedRedirect(word string) {
	r.diagf("%s\n", Wording(r.diag().RestrictedRedirect,
		"%[1]s: restricted: cannot redirect output", word))
}

// restrictedHashEntry reports whether restricted mode refuses a path being
// written into the command hash by hand, writing the refusal it makes.
//
// **Two checks, and the second is the surprising one.** A path with a
// separator in it is refused as restricted, which is the rule the command
// word and `.` already keep. A path *without* one is looked for on PATH, and
// a name nothing answers to is refused as missing — so a restricted shell
// will not take a hash entry it could not have reached on its own. Measured
// on bash 5.3.20, 2026-09-22, and it is a rule of the mode rather than of the
// hash: the same `hash -p zz a` in an ordinary shell is silent at 0 with the
// entry made, which is what this shell already does and what the comment at
// the `-p` case says.
//
// Why it matters is the third line of the same measurement. With the entry
// refused, the name it would have created is not a command: `hash -p /bin/sh
// a; a -c 'echo hello'` is `a: command not found` at 127 in a restricted
// shell and prints `hello` in an ordinary one. A shell that made the entry
// anyway would have handed the script the very command the first refusal was
// about.
//
// builtin names the builtin in the sentence, and is empty for the parameter
// route, which names none. That is not cosmetic: the two spellings report
// different *statuses* as well — 1 from the builtin and 0 from the
// assignment, which is an assignment's own status — so they are two routes
// rather than one with a prefix.
func (r *Runner) restrictedHashEntry(builtin, path string) bool {
	if !r.restricted {
		return false
	}
	if restrictedPath(path) {
		if builtin == "" {
			r.diagf("%s\n", Wording(r.diag().Restricted, "%[1]s: restricted", path))
			return true
		}
		r.restrictedOperandRefusal(builtin, path)
		return true
	}
	// The plain search rather than Runner.lookPath, which would count a hit
	// against the table and could retrack an entry: this is a question about
	// whether the name is reachable and not a lookup the script asked for.
	if len(r.lookPathAll(path)) > 0 {
		return false
	}
	if builtin == "" {
		r.diagf("%s\n", Wording(r.diag().RestrictedHashNotFound, "%[1]s: not found", path))
		return true
	}
	r.diagf("%s\n", Wording(r.diag().HashNotFound, "hash: %[1]s: not found", path))
	return true
}

// RestrictedHashPath reports whether restricted mode refuses a path written
// into the command hash through the *parameter* that presents it, writing the
// refusal it makes.
//
// Exported because one dialect shows the hash as a writable association and an
// assignment to one of its elements is a `hash -p` by another spelling — so
// the mode has to reach it, and the table's writer lives in that dialect. A
// caller that skipped this would leave the parameter as the one door into the
// hash a restricted shell does not guard, which is the door a script looking
// for a way out would try second.
func (r *Runner) RestrictedHashPath(path string) bool {
	return r.restrictedHashEntry("", path)
}

// restrictedPath reports whether a word names a path rather than a bare name,
// which is the test three of the refusals above share.
//
// A separator anywhere in the word, which is what each of the three was
// measured doing: `. ./f`, `. f/g` and `. /f` are all refused where `. f` is
// not, and a command word is refused for a leading, trailing or interior
// slash alike.
func restrictedPath(word string) bool {
	return strings.Contains(word, "/")
}

// The long spelling of the mode joins the option table here rather than in the
// literal beside its neighbors, for the reason interp/command.go's builtins do:
// the mode reaches that table back — through the freeze, through the store —
// so an entry written in the literal is an initialization cycle Go refuses to
// compile.
func init() {
	extraSetOptions["restricted"] = setOption{
		try: (*Runner).setRestrictedOption,
		get: func(r *Runner) bool { return r.restricted },
	}
}

// setRestrictedOption is the long spelling of the mode, `set -o restricted`.
//
// One shell in the panel has the name and one does not: bash has never had a
// `restricted` row in `set -o`, so only a dialect that declares this name
// reaches here. Measured 2026-09-22 on ksh93u+ 2012-08-01 from a script file:
//
//	set -o restricted    enters the mode; `set -o` then reads `restricted on`
//	set +o restricted    `set: restricted: restricted`, and the script ends
//
// So the long spelling is refused in the same shell whose `set +r` is granted,
// which is why the two doors are two and not one. See
// Semantics.RestrictedModeIsLeftByTheLetter for the letter's half.
//
// The refusal is fatal, which is measured rather than inherited from the other
// refusals in this file: every one of those reports 1 and leaves the shell
// running, and this one stops it.
func (r *Runner) setRestrictedOption(on bool, spelling string) bool {
	if on {
		r.enterRestricted()
		return true
	}
	if !r.restricted {
		// The state this shell is already in, granted as every such request
		// is.
		return true
	}
	r.fatal("%s\n", Wording(r.diag().RestrictedOperand,
		"%[1]s: %[2]s: restricted", "set", spelling))
	// The refusal's own status rather than `set`'s invalid-option one, which is
	// measured: the shell exits **1** here where an option name it does not
	// have exits 2. A refusal about the mode is the mode speaking.
	r.setOptionStatus = restrictedStatus
	return false
}
