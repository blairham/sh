// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A parameter the **shell** owns that an `unset` has removed, and what a
// declaration listing does with its name.
//
// The third route into the state interp/baredeclaration.go describes — a name
// the shell still has and has no value for — and the one that arrives from
// outside a function scope. The other two are a script's doing: a declaration
// that carried neither a value nor an attribute, and an `unset` of a local the
// running call declared. This one is the shell's: it is `unset RANDOM`, and
// the name that is gone is one the shell put there.
//
// The distinction is real in one column and invisible in the rest, which is
// why it is an axis rather than a rule. Measured 2026-09-27, `env -i
// PATH=/usr/bin:/bin TERM=dumb` with a scratch `HOME`, from a script file:
//
//	                              unset RANDOM     w=1; unset w
//	                              typeset -p RANDOM   typeset -p w
//	zsh 5.9.2                     nothing, st 0    `no such variable: w` at 1
//	bash 5.3.20, bash 3.2         `RANDOM: not found` at 1   the same at 1
//	ksh93u+ (/bin/ksh)            nothing, st 0    nothing, st 0
//	dash, BusyBox ash             no such builtin  no such builtin
//
// So one shell tells the two apart, one reports both and one is silent about
// both — and only the first needs this field. ksh93 reaches the same silence
// through DeclarePrintReportsAMissingName, which it answers `No`, so either
// value here leaves it where it is.
//
// # What the rule is keyed on, and the pairs that say so
//
// The word is **`special`** — [ParameterAttributes.Provided], which is what a
// dialect renders as that word — and not the kind, not the export, and not
// "the shell set it at startup". A grid that varied the kind alone would have
// agreed everywhere and said nothing, so the discriminating pairs hold one of
// the other words fixed and move only this one. Same run, same arrangement,
// `${(t)name}` taken in a shell of its own so that the read primes nothing:
//
//	HOME       scalar-export-special  nothing, st 0   ← export held fixed
//	LOGNAME    scalar-export          `no such variable: LOGNAME` at 1
//
//	HISTSIZE   integer-special        nothing, st 0   ← integer held fixed
//	MAILCHECK  integer                `no such variable: MAILCHECK` at 1
//
//	PS1        scalar-special         nothing, st 0   ← scalar held fixed
//	TTY        scalar                 `no such variable: TTY` at 1
//
// Three pairs, three attribute words held still, and the answer moves with
// `special` in every one. Twenty further names carrying it are silent —
// `RANDOM`, `SECONDS`, `PS2`, `PS4`, `PATH`, `IFS`, `OPTIND`, `OPTARG`,
// `COLUMNS`, `LINES`, `SHLVL`, `USERNAME`, `path`, `cdpath`, `fignore`,
// `manpath` and `pipestatus` among them, across `integer`, `scalar`, `array`,
// `tied` and `export` — and `TMPDIR`, `EDITOR`, `ZSH_NAME`, `KEYTIMEOUT` and a
// name nothing has heard of all report. The kind does not decide it; the word
// does.
//
// # The name is still there, and that is measurable rather than inferred
//
// It is the *slot* that survives and not a value: `${+RANDOM}` is 0 after the
// removal and `${(t)RANDOM}` is empty, so nothing about the parameter is left
// to print. A second `unset RANDOM` is still silent at 0, and an assignment
// brings the specialness back — `unset RANDOM; RANDOM=5` reads
// `integer-special` again and lists as `typeset -i10 RANDOM=…`. That is why
// the listing says nothing rather than reporting a name it has never heard
// of: it has heard of it.
//
// # Every listing form, not the `-p` word alone
//
// Unlike a private binding, which declines the `-p` form and is still written
// by a bare `typeset` in the same call, this state is absent from all of them.
// `typeset -p RANDOM`, `export -p RANDOM` and `readonly -p RANDOM` are each
// nothing at 0, `typeset + RANDOM` is nothing at 0, and the whole-table dump
// writes no row. So the question is asked of the *name* and the test is at the
// top of the loop, ahead of the row and ahead of the missing-name route.

// removedShellOwnIsStillAName reports whether a declaration listing naming a
// parameter the shell owns that an `unset` has removed writes nothing, at 0,
// rather than the row or the missing-name refusal it would otherwise reach.
//
// Narrow by construction, the way valuelessRecordIsStillAName is: the `&&`
// means a dialect is asked only about a name that is really in this state, so
// a shell whose parameters are all a script's never arrives.
//
// The shell's own is read the way [ParameterAttributes] reads it and not
// through [Runner.shellOwnParameter], which masks a removed name on purpose —
// the mask is the whole reason a `${(t)}` of one is empty, and asking through
// it here would be asking whether a name the shell still owns has been
// removed, which is never true.
func (r *Runner) removedShellOwnIsStillAName(name string) bool {
	return r.removed[name] && r.wasTheShellsOwnParameter(name) &&
		r.ask(r.sem().RemovedShellOwnParameterIsStillAName,
			"what a listing does with a parameter of the shell's own that an `unset` removed")
}

// wasTheShellsOwnParameter is [ParameterAttributes.Provided]'s three durable
// sources, read without the removal mask.
//
// A registration outlives the `unset` that took the value: a producer stays in
// its table and a refusal stays in absentParams, and [Runner.removed] is what
// stands in front of both. So this is the same union
// [Runner.ParameterAttributes] builds `Provided` from, less the one source
// that is a scope rather than a registration — a **private** binding, which
// the listing has already declined a row for by the time this is asked and
// whose removal is a local's question rather than the shell's.
//
// The pipeline record is named directly rather than through
// [Runner.DynamicParameter], and that is the one place the two unions part.
// It is the fourth kind of produced parameter and the only one whose own
// membership test consults `removed` — see
// Runner.namesTheProducedPipelineStatus, where `unset` is an axis because bash
// fills the name again on the next pipeline and zsh does not. Asking through
// it here would have meant asking whether a *live* producer had been removed,
// which is never true in the column that ends it: `unset pipestatus; typeset
// -p pipestatus` is nothing at 0 in zsh 5.9.2, and was `no such variable` here
// until this line named the field.
func (r *Runner) wasTheShellsOwnParameter(name string) bool {
	if name != "" && name == r.pipeStatusName {
		return true
	}
	return r.shellOwn[name] || r.DynamicParameter(name) || r.AbsentParameter(name)
}
