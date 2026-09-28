// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// `$functions_source` is every function the script has defined, to the file
// its body was read from.
//
// The same names `$functions` lists — [interp.Runner.ListedFuncNames], so
// that a prelude function is this shell speaking rather than a definition a
// caller wrote, which is the predicate four other surfaces already ask — and
// the file half is [interp.Runner.FunctionSourceFile], the record this engine
// already keeps for every route that defines a function. So this is the
// publication of a fact that was there, which is the route `$reswords`,
// `$history`, the three job tables and `$dirstack` each left the absent
// roster by.
//
// **The empty string is a value here and not a gap**, which is the row that
// decides against a fallback. Measured 2026-09-27 on zsh 5.9.2 under `-f`
// from a script file with `zsh/parameter` loaded:
//
//	f() { : }                       functions_source[f] is the script's path
//	autoload -Uz af                 the key is there and the value is empty
//	af                              and after the call it is the file on fpath
//
// An autoload stub has no body yet, so there is no file it was read from, and
// that shell writes nothing rather than guessing at the file the search would
// find. [interp.Runner.FunctionSourceFile] is the raw record for exactly that
// reason — its neighbor functionDefinitionFile falls back to the shell's own
// name, which is right for the listing it feeds and would write a path here
// where the reference writes none.
//
// Its disabled half `$dis_functions_source` is already here and already
// empty, waiting on `disable -f` — see zshEmptyParams, which is where this
// name's absence was most visible: the *disabled* table was implemented and
// the live one was not.
func zshFunctionsSourceView(r *interp.Runner) interp.AssocArray {
	names := r.ListedFuncNames()
	out := make(interp.AssocArray, len(names))
	for _, name := range names {
		out[name] = interp.Scalar(zshFunctionSourceOf(r, name))
	}
	return out
}

// zshFunctionSourceOf is one name's answer, and the whole of what the stub
// row above costs.
//
// A name still waiting to be autoloaded has **no** file, and that is the row
// that would have been got wrong by reading the record alone: this engine
// records an autoload declaration's own file against the stub — which is
// right for every other reader, since that is where the declaration was
// written — and the reference has nothing there until the body is read.
// Measured 2026-09-27, `autoload -Uz af` in a script and
// `${functions_source[af]}` is the empty string there and was the script's
// path here.
//
// autoloadPending is the same predicate `${functions[af]}` already asks to
// give a stub its own body text, rather than a second notion of what a stub
// is.
func zshFunctionSourceOf(r *interp.Runner, name string) string {
	if autoloadPending(r, name) {
		return ""
	}
	return r.FunctionSourceFile(name)
}

// zshFunctionSourceValue is `${functions_source[f]}` — one name, without
// building the table.
//
// The same seam `$functions` takes for the same reason: a lookup of one
// function is nearly every read of this parameter, and answering it through
// the view walks every other function to hand back one string.
func zshFunctionSourceValue(r *interp.Runner, name string) (string, bool) {
	if !r.FunctionIsListed(name) {
		return "", false
	}
	return zshFunctionSourceOf(r, name), true
}

// registerFunctionsSource installs `$functions_source`.
//
// Readonly and hidden, which is measured: `${(t)functions_source}` is
// `association-readonly-hide-hideval-special` and
// `functions_source[x]=y` is `read-only variable: functions_source`. The
// freeze is also what keeps a produced table from being shadowed by a write,
// the way `builtins` is.
func registerFunctionsSource(r *interp.Runner) {
	r.SetDynamicAssoc("functions_source", zshFunctionsSourceView)
	r.SetDynamicAssocElement("functions_source", zshFunctionSourceValue)
	r.MarkReadonly("functions_source")
	hideModuleParameter(r, "functions_source")
}
