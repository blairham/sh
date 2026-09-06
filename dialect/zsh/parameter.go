// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"github.com/blairham/sh/interp"
)

// The `zsh/parameter` module: this shell's own tables, presented as
// associations a script can read.
//
// Measured 2026-09-06 against zsh 5.9.2 with a scratch HOME and no startup
// files. Five of the module's thirty-three parameters are here — the five a
// real plugin manager reads, counted in `~/.zi/bin/zi.zsh`: `functions` 46
// times, `options` 24, `commands` 3, `builtins` 2 and `aliases` 1. The other
// twenty-eight are absent, so `zmodload zsh/parameter` still refuses and
// still says how many; see the note on the module rule below.
//
// **Every one of them is a view and not a snapshot**, and that is the whole
// of what this file has to get right. A table filled in once would be correct
// until the first `f() { … }` and quietly wrong afterwards — quietly, because
// nothing about it changes shape: the caller still reads an association, still
// gets a value or an empty string, and is never told it stopped tracking. The
// producers below are called at the moment the parameter is read, so read,
// mutate, read again in one shell gives three answers where a snapshot gives
// one. The tests prove it that way round rather than by asserting that a value
// looks right, because a snapshot taken late enough passes the second kind of
// test.
//
// Where a script may write to one, it is written **through** to the thing the
// view is of — assigning to `functions[f]` defines a function, and unsetting
// the element undefines it. That is not a flourish: a write that landed in the
// stored table would shadow the producer from then on, since a stored table is
// what a read finds first. `builtins` is readonly here because it is readonly
// in zsh, and `commands` refuses a write by name because this shell has no
// command hash to put an entry in.

// registerParameterModule installs the five.
func registerParameterModule(r *interp.Runner) {
	r.SetDynamicAssoc("functions", zshFunctionsView)
	r.SetDynamicAssocWriter("functions", writeZshFunction)
	r.SetDynamicAssoc("options", zshOptionsView)
	r.SetDynamicAssocWriter("options", writeZshOption)
	r.SetDynamicAssoc("commands", zshCommandsView)
	r.SetDynamicAssocWriter("commands", refuseZshCommandsWrite)
	r.SetDynamicAssoc("builtins", zshBuiltinsView)
	// Readonly rather than given a writer, which is zsh's own answer:
	// `builtins[x]=y` is `read-only variable: builtins` there. A produced
	// association with neither would take the assignment into a stored table
	// and shadow itself.
	r.MarkReadonly("builtins")
	// And hidden with it, which is not decoration: readonly is an attribute,
	// an attribute puts the name in the tables a listing walks, and a listing
	// would then write out every builtin this shell has as an assignment
	// somebody could source back. Measured — zsh's own `typeset -r` writes
	// `builtins` as a bare name.
	r.MarkHidden("builtins")
	r.SetDynamicAssoc("aliases", zshAliasesView)
	r.SetDynamicAssocWriter("aliases", writeZshAlias)
}

// zshFunctionsView is `$functions`: every function the script has defined, to
// the text of its body.
//
// The names are the *listed* set and not every callable one, which is the
// same distinction `declare -F` makes and the same mechanism behind it. A
// prelude function is this shell speaking (#603): listing `pushd` here would
// hand this dialect's own implementation to a caller as though a person had
// written it, and a caller that captures a shell's state and sources it back
// would then redefine `pushd` on top of the prelude's on every command. That
// bug has been fixed three times through three callers — #1035 for
// `declare -F`, #1081 for `compgen -A function`, #1082 for `unset -f` — and
// this is the fourth, asking the identical predicate through
// [interp.Runner.ListedFuncNames] rather than adding a fifth notion of whose
// a function is.
//
// The body is the lines a listing puts between the braces: tab-indented, no
// header, no trailing newline. Measured against `f(){ echo hi }`, whose value
// is exactly `\techo hi`.
func zshFunctionsView(r *interp.Runner) interp.AssocArray {
	names := r.ListedFuncNames()
	out := make(interp.AssocArray, len(names))
	for _, name := range names {
		body, ok := r.FunctionBodyText(name)
		if !ok {
			continue
		}
		out[name] = body
	}
	return out
}

// writeZshFunction is `functions[f]=body` and `unset "functions[f]"`, both
// measured: the first defines a function and the second undefines one.
//
// A body that will not parse defines nothing. That is a decision rather than
// an oversight — a function whose body was stored unread fails at the call,
// several hundred lines from the assignment, with a message about something
// the caller did not write.
func writeZshFunction(r *interp.Runner, name, body string, set bool) {
	if !set {
		r.RemoveFunction(name)
		return
	}
	if !r.DefineFunctionFromText(name, body) {
		r.Diagnosef("%s: not a function body this shell can read\n", name)
	}
}

// zshOptionsView is `$options`: every option name this shell knows, to `on`
// or `off`.
//
// The namespace is the one `setopt`, `unsetopt` and `[[ -o ]]` already share,
// which is what makes this a view of the same state rather than a second
// answer about it — see setopt.go. It is 197 keys: the 185 canonical names
// `setopt` and `unsetopt` list between them, and the twelve sh and ksh compat
// spellings, which `$options` carries as keys of their own where the listings
// never print them. Measured against real zsh's key set, name for name.
//
// A compat spelling shares its canonical entry's state exactly, negated where
// the two names mean opposite things: `braceexpand` is `on` where
// `ignorebraces` is `off`, and `hashall` and `trackall` are both whatever
// `hashcmds` is.
//
// This is #1080's table under another name — that issue is about `set +o`
// writing bash's 23 names where zsh writes its own — and it is worth saying
// which half was wrong. The *namespace* was already right: `setopt` and
// `unsetopt` here list the same 185 names real zsh does. It was `set +o`
// alone that read the substrate's table, so this parameter does not fix
// #1080 and never touched what was broken there.
func zshOptionsView(r *interp.Runner) interp.AssocArray {
	out := make(interp.AssocArray, len(zshOptions)+len(zshOptionAliases))
	for _, o := range zshOptions {
		out[o.base] = onOrOff(o.get(r))
	}
	for name, alias := range zshOptionAliases {
		if i, ok := zshOptionIndex[alias.base]; ok {
			out[name] = onOrOff(zshOptions[i].get(r) != alias.inv)
		}
	}
	return out
}

// onOrOff is how this parameter spells a state.
func onOrOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// writeZshOption is `options[name]=on`, which is `setopt name` written as an
// assignment, and it is the same lookup and the same refusals.
//
// Two complaints and both are measured, and both leave the *command's* status
// at 0 — the assignment says what went wrong and the shell carries on:
// `options[nosuchopt]=on` is `no such option: nosuchopt`, and
// `options[extendedglob]=maybe` is `invalid value: maybe` with the option
// unmoved. An unset of an element is neither on nor off and has nothing to
// mean, so it is refused by the second of those.
func writeZshOption(r *interp.Runner, name, value string, set bool) {
	if !set {
		r.Diagnosef("invalid value: %s\n", "")
		return
	}
	switch value {
	case "on", "off":
	default:
		r.Diagnosef("invalid value: %s\n", value)
		return
	}
	// A name nobody has is setOption's own complaint, in setOption's own
	// words, and asking here first would be a second copy of one sentence.
	// It was written that way and taken out: the mutant that dropped the
	// check changed nothing any test could see, which is what a duplicate of
	// a rule looks like.
	setOption(r, name, value == "on")
}

// zshCommandsView is `$commands`: every name PATH resolves, to the path it
// resolves to.
//
// Produced by the search the runner already does for a command word, against
// the *runner's* PATH and directory — see [interp.Runner.CommandsOnPath]. That
// is what makes it move when PATH does, which is the property this parameter
// has and a hash table would not: measured, `PATH=/nonexistent` takes
// `${+commands[ls]}` from 1 to 0 in the same shell.
func zshCommandsView(r *interp.Runner) interp.AssocArray {
	found := r.CommandsOnPath()
	out := make(interp.AssocArray, len(found))
	for name, path := range found {
		out[name] = path
	}
	return out
}

// refuseZshCommandsWrite is `commands[c]=/path`, which in zsh puts an entry in
// the command hash.
//
// Refused by name, because this shell has no command hash for it to go in: a
// lookup here is the PATH search every time, so there is nothing an entry
// could change. Accepting the assignment and dropping it would leave a caller
// holding a name it believes it has arranged for, which is the failure this
// whole file is arranged to avoid.
func refuseZshCommandsWrite(r *interp.Runner, name, _ string, _ bool) {
	r.Diagnosef("commands[%s]: assigning to the command hash is not implemented yet\n", name)
}

// zshBuiltinsView is `$builtins`: every builtin this shell has, to `defined`.
//
// A builtin switched off with `disable` is **not a key here** — measured, the
// count drops by one and `${+builtins[cd]}` is 0 — which is why the names come
// from [interp.Runner.BuiltinNames], the set that answers what running the
// word would find, rather than from the wider one that includes the ones put
// aside. zsh keeps those in `$dis_builtins`, which is one of the twenty-eight
// parameters not here.
func zshBuiltinsView(r *interp.Runner) interp.AssocArray {
	names := r.BuiltinNames()
	out := make(interp.AssocArray, len(names))
	for _, name := range names {
		out[name] = "defined"
	}
	return out
}

// zshAliasesView is `$aliases`: every alias defined now, to its text.
//
// The same table `alias` and `unalias` keep. Whether a *word* then expands as
// one is the parser's question and a different one — this shell expands
// aliases from a script file and from standard input and not from `-c`, which
// is measured and lives in syntax.Dialect.ExpandAliases — so an alias is in
// here whether or not the route it was defined on would expand it.
func zshAliasesView(r *interp.Runner) interp.AssocArray {
	table := r.AliasTable()
	out := make(interp.AssocArray, len(table))
	for name, text := range table {
		out[name] = text
	}
	return out
}

// writeZshAlias is `aliases[a]=text` and `unset "aliases[a]"`: the same pair
// `alias` and `unalias` are.
func writeZshAlias(r *interp.Runner, name, text string, set bool) {
	if !set {
		r.RemoveAlias(name)
		return
	}
	r.SetAlias(name, text)
}
