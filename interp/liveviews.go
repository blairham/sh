// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/blairham/sh/syntax"
)

// What a dialect needs in order to present this shell's own state as
// parameters a script can read.
//
// One shell has a module whose whole content is a set of associations over
// the interpreter's tables — which functions exist and what each one's body
// is, which options are on, what a name resolves to on PATH. Every one of
// them is a **view**: the answer is produced at the moment it is read, so
// defining a function between two reads changes the second. A snapshot would
// be right until the first definition and then quietly wrong, which is the
// shape of bug this whole file exists to make impossible.
//
// The accessors are here rather than in the dialect because each is about a
// table this package owns and none of them can be reconstructed from outside:
// the function table, the alias table, the prelude's declarations, and the
// PATH search the runner does against its own variables.

// ListedFuncNames is every function a listing of "what functions exist" is
// asking about: the script's own, sorted, with the dialect's prelude left out.
//
// The fourth caller of one rule, and it deliberately adds no fifth notion of
// whose a function is. A prelude function is the shell speaking (#603), and
// listing it hands the shell's own implementation to a caller as though a
// person had written it — which is what #1035 fixed for `declare -F`, #1081
// for `compgen -A function` and #1082 for `unset -f`. This asks
// [Runner.speaksForTheShell] through scriptFuncNames, exactly as those do, so
// a script that redefines `pushd` is in the listing from that moment and by
// the same rule that moves the diagnostic's voice back to it.
//
// [Runner.FuncNames] is the other set — every function *callable*, which a
// completer wants — and the two must not be confused: one of them names the
// shell's own implementation and the other does not.
func (r *Runner) ListedFuncNames() []string { return r.scriptFuncNames() }

// FunctionIsListed reports whether one name is among the ones
// [Runner.ListedFuncNames] yields.
//
// The same predicate, asked of a name instead of answered for all of them,
// and it exists because the whole list is the wrong unit for a lookup: a
// dialect answering `${functions[precmd]}` needs to know about `precmd` and
// building a sorted slice of every other function to find out is the cost
// SetDynamicAssocElement was added to stop paying.
func (r *Runner) FunctionIsListed(name string) bool {
	fn, ok := r.funcs[name]
	return ok && !r.speaksForTheShell(fn)
}

// FunctionBodyText is a function's body as a listing writes it, without the
// header line and without the braces around it.
//
// The lines *between* the braces of what [Runner.FunctionText] returns, which
// is what one shell's `$functions` association holds — measured, the value for
// `f(){ echo hi }` is a single line `\techo hi`, tab-indented and with no
// trailing newline. Derived from the same printer the listing uses rather than
// from a second one, so a body reads back identically however it is asked for.
func (r *Runner) FunctionBodyText(name string) (string, bool) {
	fn, ok := r.funcs[name]
	if !ok {
		return "", false
	}
	// The printed body is `{`, the statements, `}` — one line each, in the
	// dialect's own layout. Dropping the first and last lines is the whole
	// of the difference, and it is done on the print rather than on the
	// tree so that the indentation is the listing's.
	text := syntax.PrintWith(fn.Body, r.functionLayout)
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if len(lines) < 2 {
		return "", true
	}
	return strings.Join(lines[1:len(lines)-1], "\n"), true
}

// DefineFunctionFromText defines a function from the text of its body, which
// is how one shell's `$functions` association is *written* to.
//
// The body is read with this shell's own grammar, so what a script may put in
// one is what it may put in a function it writes out — and a body that will
// not parse defines nothing and says so, rather than leaving a function whose
// call fails somewhere else entirely.
//
// The definition is the script's own however it arrived: nothing here calls
// preludeDefined, so assigning over a prelude function takes the shell's voice
// away from the name exactly as writing `pushd() { … }` does.
func (r *Runner) DefineFunctionFromText(name, body string) bool {
	return r.defineFromText(name, body, nil)
}

// RemoveFunction undefines one, and reports whether the script had one.
//
// The same operation `unset -f` is, through the same code — see
// removeFunction. That matters at exactly one name: removing a *redefined*
// prelude function gives the name back to the prelude's rather than leaving
// it undefined, so `unset "functions[pushd]"` and `unset -f pushd` leave the
// shell in one state and not two. A second implementation here would be a
// second answer to a question this package has already answered (#1082).
//
// A prelude function the script never redefined is not the script's to
// remove, and nothing happens: false, and the name still works.
func (r *Runner) RemoveFunction(name string) bool {
	fn, ok := r.funcs[name]
	if !ok || r.speaksForTheShell(fn) {
		return false
	}
	r.removeFunction(name)
	return true
}

// AliasTable is every *regular* alias defined now, as a copy.
//
// A copy for the reason GetAssoc's is: a caller reading the table, changing it
// and writing it back must not move the shell's state halfway through.
//
// Regular only, because the parameter this answers is: the shell with three
// kinds keeps three parameters, and `$aliases` there holds neither the
// global ones nor the suffix ones — measured, `alias -g G=x; alias r=y;
// alias -s t=z` leaves `${(k)aliases}` naming `r` alone. The other two are
// [Runner.GlobalAliasTable] and [Runner.SuffixAliasTable].
func (r *Runner) AliasTable() map[string]string {
	out := make(map[string]string, len(r.aliases))
	for k, v := range r.aliases {
		if v.global {
			continue
		}
		out[k] = v.value
	}
	return out
}

// GlobalAliasTable is every global alias, and SuffixAliasTable every suffix
// alias, on the same terms.
func (r *Runner) GlobalAliasTable() map[string]string {
	out := map[string]string{}
	for k, v := range r.aliases {
		if v.global {
			out[k] = v.value
		}
	}
	return out
}

// SuffixAliasTable is the second namespace, keyed on the extension.
func (r *Runner) SuffixAliasTable() map[string]string {
	out := make(map[string]string, len(r.suffixAliases))
	for k, v := range r.suffixAliases {
		out[k] = v
	}
	return out
}

// SetAlias defines one, and RemoveAlias undefines one.
//
// The same table `alias` and `unalias` keep, because a shell with two alias
// tables is two shells. Whether a *word* then expands as one is the parser's
// question and not this one — see [Runner.LookupAlias].
//
// The regular kind, to pair with AliasTable: the parameter these answer for
// writes a regular alias, and a name already holding a global one is
// replaced by a regular one rather than keeping the letter it was defined
// with.
func (r *Runner) SetAlias(name, value string) {
	r.defineAlias(name, value, AliasAnyKind)
}

// RemoveAlias undefines one, and reports whether there was one.
func (r *Runner) RemoveAlias(name string) bool {
	return r.removeAlias(name, AliasAnyKind)
}

// SetGlobalAlias and SetSuffixAlias are the pair for the other two kinds, for
// the parameters that present them — `$galiases` and `$saliases`. A global
// alias shares the table the regular ones are in, so this *replaces* a
// regular alias of the same name, exactly as `alias -g` does.
func (r *Runner) SetGlobalAlias(name, value string) {
	r.defineAlias(name, value, AliasGlobalKind)
}

// SetSuffixAlias writes the second namespace, keyed on the extension.
func (r *Runner) SetSuffixAlias(suffix, value string) {
	r.defineAlias(suffix, value, AliasSuffixKind)
}

// RemoveSuffixAlias undefines one, and reports whether there was one. There
// is no global counterpart because there is no separate table to take it out
// of: RemoveAlias is the removal for both kinds, which is what `unalias`
// having no `-g` says.
func (r *Runner) RemoveSuffixAlias(suffix string) bool {
	return r.removeAlias(suffix, AliasSuffixKind)
}

// CommandsOnPath is every name PATH would resolve, to the path it resolves to.
//
// The listing half of the lookup [Runner.LookPath] does one name at a time,
// and against the same two things: the *runner's* PATH and the *runner's*
// directory, never the process's. That is the rule lookpath.go exists for —
// delegating to os/exec once produced a shell where `PATH=/tmp/mine` still
// found everything on the developer's machine — and it matters more here, not
// less, because this answers about every name at once.
//
// **Earlier PATH entries win**, which is the same rule a single lookup
// follows: two directories holding `ls` resolve to the first one's.
//
// It reads directories, which is what makes it a fact about the machine and
// not about the Runner — but so is every command this shell has ever run, and
// the search is the identical one. Nothing process-wide is touched and no
// state is kept: a caller asking twice with a different PATH gets two answers,
// which is the point of a view.
func (r *Runner) CommandsOnPath() map[string]string {
	out := map[string]string{}
	path, _ := r.getVar("PATH")
	for _, dir := range r.pathElements(path) {
		if dir == "" {
			dir = "."
		}
		entries, err := os.ReadDir(r.absolute(dir))
		if err != nil {
			// A PATH entry that is not a directory is not an error: real
			// shells carry stale entries for years and say nothing.
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if _, taken := out[name]; taken {
				continue
			}
			full := r.absolute(filepath.Join(dir, name))
			if r.runnable(full) != nil {
				continue
			}
			out[name] = full
		}
	}
	return out
}

// MarkReadonly freezes a name against assignment, which is what `readonly`
// and `typeset -r` do.
//
// For a dialect whose *produced* parameter a script must not write to. One
// shell's `builtins` association is readonly and its neighbors are not, and
// the difference has to be sayable: a produced association with no writer
// would otherwise take an assignment into a stored table and shadow itself,
// where this refuses with the sentence the dialect already has for the case.
func (r *Runner) MarkReadonly(name string) { r.markReadonly(name) }

// MarkHidden keeps a name's *value* out of the listings, which is what
// `typeset -H` does.
//
// A produced association needs it for the same reason a produced scalar never
// needed anything: a produced name is not in any of the tables a listing walks,
// so it stays out by itself — until some other attribute puts it there.
// Marking one readonly does exactly that, and a listing would then print the
// whole of a table the shell generates. Measured in the shell this models: its
// readonly `builtins` appears in `typeset -r` as a bare name with no `=` and
// no value, and in a plain `typeset` not at all.
func (r *Runner) MarkHidden(name string) {
	if r.hidden == nil {
		r.hidden = map[string]bool{}
	}
	r.hidden[name] = true
}
