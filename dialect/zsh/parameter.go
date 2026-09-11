// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strings"

	"github.com/blairham/sh/interp"
)

// The `zsh/parameter` module: this shell's own tables, presented as
// associations a script can read.
//
// Measured 2026-09-06 against zsh 5.9.2 with a scratch HOME and no startup
// files. Five of the module's thirty-three parameters are **implemented** —
// the five a real plugin manager reads, counted in `~/.zi/bin/zi.zsh`:
// `functions` 46 times, `options` 24, `commands` 3, `builtins` 2 and
// `aliases` 1. The other twenty-eight are sorted into two kinds, and which
// kind a parameter is in is a statement about this shell rather than about
// how far along it is:
//
//   - **Ten are empty, and that is the right answer.** Each reports on
//     something that cannot happen here, and the thing that cannot happen
//     refuses by name where it is asked for — `alias -g`, `alias -s`,
//     `disable -p` and `hash -d` are `bad option`, and `disable -a`, `-f`
//     and `-r` are `not implemented yet`. Nothing is disabled, there are no
//     global or suffix aliases and no named directories, so an empty table
//     is *true*, and it starts reporting by itself the day one of those
//     letters lands. See zshEmptyParams, whose second column is what each is
//     waiting on and which the tests hold to it.
//   - **Eighteen are absent**, and they refuse by name when a script reads
//     one — `jobstates: parameter not implemented yet`, at the expansion
//     that asked. None of them reads as empty, which is the whole reason
//     the module can load without them; see the note on the module rule in
//     zmodload.go and [interp.Runner.SetAbsentParameter].
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
//
// **`unset "name[key]"` is a write, and it is not the same write as an
// assignment.** Each of the four below settles it separately, because what an
// unset *means* differs per table and a single rule for all of them was wrong
// in both directions (#1527):
//
//   - `functions` and `aliases` undefine, which is the assignment run
//     backwards and needs nothing said.
//   - `options` **turns the option off** — measured, not guessed: `setopt
//     noclobber; unset "options[noclobber]"` lets the next `>` truncate, and
//     an option that is *on by default* goes to `off` rather than back to its
//     default. An unknown name is silent where the assignment's is `no such
//     option`, and a fixed option is `can't change option` through either.
//   - the ten empty ones are **silent**. The assignment refuses because a
//     caller that believes it arranged a global alias would be told nothing
//     at either end; an unset asks for absence, and absence is what the table
//     already has, so there is nothing left to warn about and the read
//     afterwards agrees with zsh's.
//   - `commands` still refuses, and that is the case that shows the other
//     three are not one rule. There the read afterwards does *not* agree:
//     zsh's `unset "commands[ls]"` drops a cached entry and `${commands[ls]}`
//     is empty until the next search, where this shell searches every time and
//     would still answer `/bin/ls`. Silence would hand a script testing
//     `[[ -z $commands[ls] ]]` the opposite answer with nothing said.
//
// #1527 was filed the other way round — that `unset "functions[m]"` should
// *refuse*, because zsh 5.9.2 answers `functions: assignment to invalid
// subscript range` and leaves the function standing. It does, and the reason
// is not about `unset` on this table at all: in a shell where nothing has
// touched `$functions` yet, the name is still zsh's autoload stub for the
// module and its value is the string `zsh/parameter`, so the subscript is read
// as *arithmetic against a scalar* — `functions[1]` shortens that string to
// `sh/parameter` and the next read of the parameter fails to load a module by
// that name. Every other access materializes the parameter first and an
// `unset` is the one that does not. With the module loaded — which is the
// state every script that reads `$functions` is in, and which the plugin
// manager this was found in guarantees on its own line 232 — real zsh removes
// the function at status 0, exactly as here. Modeling the refusal would have
// meant reproducing a zsh bug against a state this shell cannot be in.

// registerParameterModule installs all thirty-three: seven as views, ten as
// empty views, and sixteen as refusals.
func registerParameterModule(r *interp.Runner) {
	r.SetDynamicAssoc("functions", zshFunctionsView)
	// And the same table read one key at a time, which is what nearly every
	// read of it is. See zshFunctionValue.
	r.SetDynamicAssocElement("functions", zshFunctionValue)
	r.SetDynamicAssocWriter("functions", writeZshFunction)
	r.SetDynamicAssoc("options", zshOptionsView)
	r.SetDynamicAssocElement("options", zshOptionValue)
	r.SetDynamicAssocWriter("options", writeZshOption)
	r.SetDynamicAssoc("commands", zshCommandsView)
	r.SetDynamicAssocElement("commands", zshCommandValue)
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
	// The other two kinds, each with its own parameter, which is how this
	// shell says they are three namespaces rather than one table with flags:
	// `alias -g G=x; alias r=y; alias -s t=z` leaves `${(k)aliases}` naming
	// `r` alone, `${(k)galiases}` naming `G` and `${(k)saliases}` naming `t`.
	// Measured (#2081).
	r.SetDynamicAssoc("galiases", zshGlobalAliasesView)
	r.SetDynamicAssocWriter("galiases", writeZshGlobalAlias)
	r.SetDynamicAssoc("saliases", zshSuffixAliasesView)
	r.SetDynamicAssocWriter("saliases", writeZshSuffixAlias)
	r.SetDynamicArray("funcstack", funcstackNames)
	r.SetDynamicAssoc("parameters", zshParametersView)
	r.SetDynamicAssocElement("parameters", zshParameterValue)
	// Readonly and hidden together, the pair `builtins` needs and for the same
	// two reasons: zsh answers `parameters[x]=y` with `read-only variable:
	// parameters`, and a produced table with neither would take the assignment
	// into a stored table that then shadows the producer. See MarkHidden's
	// note above — an attribute puts the name in the tables a listing walks.
	r.MarkReadonly("parameters")
	r.MarkHidden("parameters")
	registerArgv(r)
	// `${(t)name}` is the one-name spelling of the table above, and it is
	// the same words: a script asking what it was handed and a script
	// reading `$parameters` must not be told two different things about one
	// parameter.
	r.SetParameterTypeWord(describeParameter)
	registerEmptyParameters(r)
	registerAbsentParameters(r)
}

// zshEmptyParams are the ten parameters of the module that are empty in this
// shell and *right* to be, each with the thing it reports on.
//
// Not stubs. A stub is a value nobody produced standing in for one nobody can;
// these are answers. `$galiases` is every global alias, `alias -g` is
// `bad option` here, so there are none and the table is empty — the same
// sentence a real zsh writes with none defined. Measured against zsh 5.9.2:
// all ten are empty in a fresh shell there too.
//
// `nameddirs` is the tenth and #1137's survey had it in the wrong column —
// filed as needing `hash -d`, which is true and is the point: `hash -d` is
// `bad option` here, so no named directory can exist, so the empty table is
// the answer rather than a gap. Measured both ways in zsh 5.9.2:
// `hash -d foo=/tmp` takes `${#nameddirs}` from 0 to 1, and nothing else
// does — `setopt autonamedirs` with a variable holding a path leaves it at 0.
//
// The second column is what each is waiting on, and it is load-bearing rather
// than a comment: the tests run that exact line and require it to still
// refuse. The day `alias -g` works, `$galiases` is silently wrong and nothing
// else in this file would have noticed — so the test fails instead, at the
// name of the parameter that has to move with it (#1137).
//
// zsh's own types decide the rest. Eight are associations and two are arrays,
// and `dis_functions_source`, `dis_patchars` and `dis_reswords` are readonly
// there — which is also what keeps a produced table from being shadowed by a
// write, the way `builtins` is.
var zshEmptyParams = []struct {
	name     string
	waitsFor string
	array    bool
	readonly bool
}{
	{name: "dis_aliases", waitsFor: "disable -a"},
	{name: "dis_functions", waitsFor: "disable -f"},
	{name: "dis_functions_source", waitsFor: "disable -f", readonly: true},
	// The *disabled* half of the two kinds #2081 added, and they are still
	// honestly empty: an alias is in these tables only once `disable` has
	// taken it out of the live one, and `disable -a` — which covers the
	// regular and the global kind alike — and `disable -s` are both `not
	// implemented yet` here. The letter each waits on moved with the
	// feature; the emptiness did not.
	{name: "dis_galiases", waitsFor: "disable -a"},
	{name: "dis_patchars", waitsFor: "disable -p", array: true, readonly: true},
	{name: "dis_reswords", waitsFor: "disable -r", array: true, readonly: true},
	{name: "dis_saliases", waitsFor: "disable -s"},
	{name: "nameddirs", waitsFor: "hash -d"},
}

// registerEmptyParameters installs the ten as *views* that happen to be
// empty rather than as empty tables.
//
// A view rather than a stored table for the reason every other one here is:
// a stored table is what a read finds first, so a name given one has stopped
// answering for anything from that moment. These produce nothing today
// because there is nothing to produce, and the producer is where the answer
// goes when there is.
func registerEmptyParameters(r *interp.Runner) {
	for _, p := range zshEmptyParams {
		if p.array {
			r.SetDynamicArray(p.name, func(*interp.Runner) []string { return nil })
		} else {
			r.SetDynamicAssoc(p.name, func(*interp.Runner) interp.AssocArray { return nil })
		}
		if p.readonly {
			// zsh's own attribute, and the same pair `builtins` needs: a
			// produced table with neither a writer nor this would take an
			// assignment into a stored table and shadow itself, and readonly
			// alone would put the name in the tables a listing walks.
			r.MarkReadonly(p.name)
			r.MarkHidden(p.name)
			continue
		}
		r.SetDynamicAssocWriter(p.name, refuseEmptyParameterWrite(p.name, p.waitsFor))
	}
}

// refuseEmptyParameterWrite is `galiases[x]=ls` — which in zsh defines a
// global alias, and here has nothing to define it with.
//
// Refused by the name of the letter that is missing, so the sentence is a
// to-do rather than a wall: `galiases[x]: alias -g is not implemented yet`.
// Silence would be worse than a wall — a caller that believes it has arranged
// for a global alias and reads the table back empty is told nothing at either
// end.
//
// **An unset is not that caller**, and used to get the same sentence (#1527).
// `unset "galiases[x]"` asks for the key to be absent, the table it is asked
// of is empty and stays empty, and the read afterwards is the empty string in
// this shell and in zsh alike — so there is no answer left for the caller to
// be misled by and nothing the refusal could still be protecting. Measured:
// zsh is silent at status 0 for `unset "galiases[f]"` and for
// `unset "nameddirs[x]"`. `commands` is the table where this reasoning does
// *not* carry, and the note at the top of this file says why.
func refuseEmptyParameterWrite(name, waitsFor string) func(*interp.Runner, string, string, bool) {
	return func(r *interp.Runner, key, _ string, set bool) {
		if !set {
			return
		}
		r.Diagnosef("%s[%s]: %s is not implemented yet\n", name, key, waitsFor)
	}
}

// zshAbsentParams are the eighteen this shell has not got at all.
//
// Every one of them is non-empty, or can be, in a shell that has it: `$modules`
// is 14 entries in a fresh zsh, `$parameters` 214, `$reswords` 31, `$patchars`
// 15, `$usergroups` 16, and the job and history tables fill as a session runs.
// So none of them can be answered with an empty table the way the nine above
// are — an empty `$reswords` is not "no reserved words", it is "nobody asked
// the shell" — and each refuses by name instead.
//
// What each would cost is surveyed in #1137: eight are answerable from a table
// this shell already keeps and ten need a seam that does not exist. Neither is
// a distinction a script can see, so it is not one this file makes; both kinds
// refuse identically until they are implemented.
//
// Two of them are worth naming, because each looks like it belongs with the
// empty ten and does not. `dirstack` would be empty if nothing could push a
// directory, and `pushd` and `dirs` both work here, so an empty stack is a
// claim and not an answer. `userdirs` fills as `~user` is expanded, and
// `~root` here does not expand — but it does not *refuse* either, it stays
// literal, so there is no line to hold an honesty check against and no way to
// tell "no users looked up yet" from "this shell cannot look one up".
var zshAbsentParams = []string{
	"dirstack", "dis_builtins", "funcfiletrace", "funcsourcetrace",
	"functions_source", "functrace", "history", "historywords",
	"jobdirs", "jobstates", "jobtexts", "modules",
	"patchars", "reswords", "userdirs", "usergroups",
}

// registerAbsentParameters makes each of them refuse by name when it is read.
//
// The parameter's own call site, which is what a builtin has had all along —
// see [interp.Runner.SetAbsentParameter]. `zregexparse` is the shape: nothing
// is registered under the name, `whence -w zregexparse` says `none`, and the
// line that calls it is where a script finds out. A parameter had no such
// line until this, which is why one absent parameter used to hold thirty-two
// others shut.
func registerAbsentParameters(r *interp.Runner) {
	for _, name := range zshAbsentParams {
		r.SetAbsentParameter(name, "parameter not implemented yet")
	}
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
//
// Except for a function whose body has not been read yet, whose value is not
// its listing and not the tree either: `autoload -Uz f1` lists three lines and
// reads back here as the one string `builtin autoload -XU`, unindented. So
// this asks autoloadBodyValue first.
func zshFunctionsView(r *interp.Runner) interp.AssocArray {
	names := r.ListedFuncNames()
	out := make(interp.AssocArray, len(names))
	for _, name := range names {
		if body, ok := autoloadBodyValue(r, name); ok {
			// A name still waiting to be defined holds a third text, which
			// is neither its listing nor the tree printed back — see
			// autoloadBodyValue for the measurement.
			out[name] = body
			continue
		}
		body, ok := r.FunctionBodyText(name)
		if !ok {
			continue
		}
		out[name] = body
	}
	return out
}

// zshFunctionValue is `${functions[f]}`: the one key, without rendering the
// rest of the table to reach it.
//
// The same three answers zshFunctionsView gives, in the same order and from
// the same two helpers, which is the contract SetDynamicAssocElement states
// — this is a shorter route to that function's value and not a second
// opinion about it. A name that is not one the listing yields is not a key
// here, so it reads as absent exactly as it would from the built table.
//
// # Why the short route exists
//
// The long one is quadratic in a way that only shows up on a real startup.
// Every read of `${functions[x]}` built the whole association: a body for
// each of the names, each body rendered through the printer, and each name
// asked whether it was still waiting to be defined — which was itself a
// linear scan of the autoload set. Measured on this machine's own startup on
// 2026-09-10, that was **341 renderings costing 10.46 seconds**, out of 13.2
// seconds from process start to a usable prompt. Real zsh answers the same
// reads in half a millisecond, because there the parameter is a view onto
// the shell's function table and a key is a lookup in it.
func zshFunctionValue(r *interp.Runner, name string) (string, bool) {
	if !r.FunctionIsListed(name) {
		return "", false
	}
	if body, ok := autoloadBodyValue(r, name); ok {
		return body, true
	}
	return r.FunctionBodyText(name)
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
	if !zshDefineFromText(r, name, body, false) {
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

// zshOptionValue is `${options[extendedglob]}`: one option's state, which is
// an index into the same two tables the view walks.
func zshOptionValue(r *interp.Runner, name string) (string, bool) {
	if i, ok := zshOptionIndex[name]; ok {
		return onOrOff(zshOptions[i].get(r)), true
	}
	if alias, ok := zshOptionAliases[name]; ok {
		if i, ok := zshOptionIndex[alias.base]; ok {
			return onOrOff(zshOptions[i].get(r) != alias.inv), true
		}
	}
	return "", false
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
// unmoved.
//
// **`unset "options[name]"` turns the option off.** That was written here as a
// refusal — "an unset of an element is neither on nor off and has nothing to
// mean" — and the measurement says otherwise (#1527): zsh moves the option,
// and moves it *off* rather than back to its default, which two probes tell
// apart. `setopt noclobber; unset "options[noclobber]"` lets the next `>`
// truncate the file, so it is the option and not just the view; and `equals`
// and `banghist`, both on in a fresh shell, read `off` afterwards. The refusal
// meanwhile said `invalid value: ` with nothing after the colon and left the
// option standing at status 0, which is a script being told it has turned
// `xtrace` off while the trace goes on.
//
// The one thing an unset does not share with `unsetopt` is what it says about
// a name nobody has: measured silent, where `options[nosuchopt]=on` is `no
// such option`. So the lookup is asked here for that alone — which spelling
// exists — and the move, the fold of the compat spellings and both remaining
// sentences stay setOption's, unduplicated.
func writeZshOption(r *interp.Runner, name, value string, set bool) {
	if !set {
		if _, _, known := resolveOptionName(normalizeOption(name)); !known {
			return
		}
		setOption(r, name, false)
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

// zshCommandValue is `${commands[git]}`: one PATH search rather than a
// listing of every directory on PATH.
//
// The same search by the same rule — [interp.Runner.LookPath] is the
// resolution a command word gets, and the view is that resolution run over
// every entry — so the first PATH element holding the name wins in both.
// Measured, reading one key by building the view was nine milliseconds
// against tens of microseconds for the lookup.
//
// A name with a slash in it is refused, and that is the one place the two
// could have parted: the view's keys are directory entries and never contain
// one, where `LookPath` would happily resolve `/bin/ls` as a path. So
// `${commands[/bin/ls]}` is empty here, as it is in the table.
func zshCommandValue(r *interp.Runner, name string) (string, bool) {
	if strings.ContainsRune(name, '/') {
		return "", false
	}
	return r.LookPath(name)
}

// refuseZshCommandsWrite is `commands[c]=/path` and `unset "commands[c]"`,
// which in zsh put an entry in the command hash and take one out.
//
// Refused by name, because this shell has no command hash for either to reach:
// a lookup here is the PATH search every time, so there is nothing an entry
// could change and nothing a removal could drop. Accepting and dropping the
// request would leave a caller holding a name it believes it has arranged for,
// which is the failure this whole file is arranged to avoid.
//
// The unset is refused where the empty tables' is not, and the difference is
// what the *next read* says rather than a preference. Measured: zsh's
// `unset "commands[ls]"` is silent at status 0 and `${commands[ls]}` is empty
// afterwards until something searches again — where this view searches every
// time and would answer `/bin/ls`. A script guarding on `[[ -z
// $commands[ls] ]]` would take the opposite branch here with nothing said.
// Which verb is named follows the request, so the sentence is about what the
// caller asked for and not about the other one.
func refuseZshCommandsWrite(r *interp.Runner, name, _ string, set bool) {
	verb := "removing an entry from"
	if set {
		verb = "assigning to"
	}
	r.Diagnosef("commands[%s]: %s the command hash is not implemented yet\n", name, verb)
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
	return aliasAssoc(r.AliasTable())
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

// zshGlobalAliasesView is `$galiases`: every *global* alias, which is the
// kind expanded wherever a word stands rather than only in command position.
//
// A table of its own here and a shared one inside — see interp.aliasDef —
// because that is what this shell shows: `alias -g` over a regular name
// replaces it, so a name is in one of these two parameters and never both.
func zshGlobalAliasesView(r *interp.Runner) interp.AssocArray {
	return aliasAssoc(r.GlobalAliasTable())
}

// writeZshGlobalAlias is `galiases[G]=text`, which is `alias -g G=text`.
func writeZshGlobalAlias(r *interp.Runner, name, text string, set bool) {
	if !set {
		r.RemoveAlias(name)
		return
	}
	r.SetGlobalAlias(name, text)
}

// zshSuffixAliasesView is `$saliases`: the second namespace, keyed on a
// command word's extension.
func zshSuffixAliasesView(r *interp.Runner) interp.AssocArray {
	return aliasAssoc(r.SuffixAliasTable())
}

// writeZshSuffixAlias is `saliases[txt]=text`, which is `alias -s txt=text`.
func writeZshSuffixAlias(r *interp.Runner, name, text string, set bool) {
	if !set {
		r.RemoveSuffixAlias(name)
		return
	}
	r.SetSuffixAlias(name, text)
}

// aliasAssoc is one of those tables as the parameter type. One conversion for
// the three, because three copies of a two-line loop is three places for the
// next fix to reach two of.
func aliasAssoc(table map[string]string) interp.AssocArray {
	out := make(interp.AssocArray, len(table))
	for name, text := range table {
		out[name] = text
	}
	return out
}
