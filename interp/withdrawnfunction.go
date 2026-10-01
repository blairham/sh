// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"

	"github.com/blairham/sh/syntax"
)

// A **withdrawn function** is one a script defined and then switched off. It
// is not a function any more — a call does not reach it, nothing that names
// functions names it, and a lookup goes on to the builtins and PATH — but the
// body is kept, so switching it back on needs nothing to have been
// remembered elsewhere.
//
// It is the function half of what [Runner.SetBuiltinEnabled] does for a
// builtin, and one shell's `disable -f` is the whole of why it exists.
// Measured 2026-10-01 on zsh 5.9.2 (`-f`, a script file under `env -i`),
// with `zq() { print zq; }` and then `disable -f zq`:
//
//	zq                      command not found: zq, 127
//	whence -w zq            zq: none
//	typeset -f zq           nothing, 1
//	${+functions[zq]}       0          ${+dis_functions[zq]}  1
//	zq() { print new; }     defines it afresh, and it is no longer disabled
//	unfunction zq           removes it, at 0
//	enable -f zq            the old body back
//	( enable -f zq; zq )    zq — and the parent's is still off afterwards
//
// The last row is why the record is cloned into a subshell with the function
// table it was taken out of.
type withdrawnFunction struct {
	decl      *syntax.FuncDecl
	origin    funcOrigin
	hasOrigin bool
	exported  bool
}

// SetFunctionWithdrawn switches a script's function off, or back on, and
// reports whether there was one to switch.
//
// Off needs a function the script defined; a name the shell itself provides,
// or one already off, is not one, and the answer is false — except that
// switching off what is already off is no change and answers true, which is
// what the shell modeled says (`disable -f zq` twice is 0). On needs a
// withdrawn one, and switching on a function that is already on is likewise
// true.
func (r *Runner) SetFunctionWithdrawn(name string, withdrawn bool) bool {
	if !withdrawn {
		w, ok := r.withdrawnFuncs[name]
		if !ok {
			fn, live := r.funcs[name]
			return live && !r.speaksForTheShell(fn)
		}
		delete(r.withdrawnFuncs, name)
		if r.funcs == nil {
			r.funcs = map[string]*syntax.FuncDecl{}
		}
		r.funcs[name] = w.decl
		if w.hasOrigin {
			r.recordFunctionOrigin(name, w.origin)
		}
		if w.exported {
			if r.exportedFuncs == nil {
				r.exportedFuncs = map[string]bool{}
			}
			r.exportedFuncs[name] = true
		}
		return true
	}
	if _, already := r.withdrawnFuncs[name]; already {
		return true
	}
	fn, ok := r.funcs[name]
	if !ok || r.speaksForTheShell(fn) {
		return false
	}
	w := withdrawnFunction{decl: fn, exported: r.exportedFuncs[name]}
	w.origin, w.hasOrigin = r.funcOrigins[name]
	r.removeFunctionQuietly(name)
	if r.withdrawnFuncs == nil {
		r.withdrawnFuncs = map[string]withdrawnFunction{}
	}
	r.withdrawnFuncs[name] = w
	return true
}

// FunctionWithdrawn reports whether a name is a function that is switched off.
func (r *Runner) FunctionWithdrawn(name string) bool {
	_, ok := r.withdrawnFuncs[name]
	return ok
}

// WithdrawnFunctionNames is every function switched off, sorted.
func (r *Runner) WithdrawnFunctionNames() []string {
	names := make([]string, 0, len(r.withdrawnFuncs))
	for name := range r.withdrawnFuncs {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// WithdrawnFunctionText is a switched-off function's definition written back
// the way [Runner.FunctionText] writes a live one: a listing of the disabled
// table shows the same text the live table's listing does.
func (r *Runner) WithdrawnFunctionText(name string) (string, bool) {
	w, ok := r.withdrawnFuncs[name]
	if !ok {
		return "", false
	}
	return r.listedFunction(name, w.decl), true
}

// WithdrawnFunctionBodyText is [Runner.FunctionBodyText] for a switched-off
// function, which is what a parameter of the disabled table holds.
func (r *Runner) WithdrawnFunctionBodyText(name string) (string, bool) {
	w, ok := r.withdrawnFuncs[name]
	if !ok {
		return "", false
	}
	return r.functionBodyText(w.decl), true
}

// WithdrawnFunctionSourceFile is [Runner.FunctionSourceFile] for a
// switched-off function.
func (r *Runner) WithdrawnFunctionSourceFile(name string) string {
	return r.sourceFileAnswer(r.withdrawnFuncs[name].origin.file)
}

// RemoveWithdrawnFunction forgets a switched-off function outright, and
// reports whether there was one.
func (r *Runner) RemoveWithdrawnFunction(name string) bool {
	if _, ok := r.withdrawnFuncs[name]; !ok {
		return false
	}
	delete(r.withdrawnFuncs, name)
	return true
}

// definedOverAWithdrawal is what a new definition does to a name that was
// switched off: it is a function again, the new one, and nothing is left to
// switch back on. Measured — `disable -f zq; zq() { print new; }` leaves
// `${+dis_functions[zq]}` at 0.
func (r *Runner) definedOverAWithdrawal(name string) {
	delete(r.withdrawnFuncs, name)
}
