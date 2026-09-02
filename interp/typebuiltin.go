// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "context"

// `type` says what a name would run, in a sentence rather than as a path.
//
// The same question `command -v` answers and the same lookup, which is why
// they share one: a function first, then a builtin, then a keyword, then
// PATH. What differs is that this one is for a person to read, so every part
// of it is worded — and the four shells word all four parts differently, down
// to whether a missing name gets the shell's name in front of it.
func init() { builtins["type"] = biType }

func biType(r *Runner, _ context.Context, args []string) int {
	// `--` ends the options in three of the four. dash has no options at
	// all and reads it as a name, which is why this is asked rather than
	// assumed: `type -- ls` prints `--: not found` there and then goes on to
	// answer about `ls`.
	names, code := r.typeOperands(args)
	if code != 0 {
		return code
	}
	status := 0
	for _, name := range names {
		if bad := r.typeOne(name); bad != 0 {
			status = bad
		}
	}
	return status
}

// typeOperands separates the options from the names.
func (r *Runner) typeOperands(args []string) ([]string, int) {
	// Asked only where there is a `--` to decide about. `type ls` is the
	// same in all four, and refusing it over a question nothing turned on
	// would be refusing to answer.
	if len(args) == 0 || args[0] != "--" {
		return args, 0
	}
	if !r.ask(r.sem().TypeEndsOptionsWithDashDash, "`type --` ending the options") {
		if r.unspecified {
			return nil, 2
		}
		// No options anywhere, so every operand is a name — `--` included.
		return args, 0
	}
	return args[1:], 0
}

// typeOne accounts for one name, and reports a status if it could not.
func (r *Runner) typeOne(name string) int {
	dg := r.diag()
	if _, ok := r.funcs[name]; ok {
		if r.ask(r.sem().TypePrintsFunctionBody, "`type` printing a function's body") {
			// The sentence without the body would be most of an answer, and
			// the missing half is the half that was asked for. Refused
			// whole: printing a body needs a printer for the syntax tree,
			// and there is not one yet.
			r.diagf("type: printing a function's body is not implemented yet\n")
			return 1
		}
		if r.unspecified {
			return 2
		}
		r.printf("%s\n", Wording(dg.TypeFunction, "%[1]s is a function", name))
		return 0
	}
	if _, ok := r.lookupBuiltin(name); ok {
		r.printf("%s\n", Wording(dg.TypeBuiltin, "%[1]s is a shell builtin", name))
		return 0
	}
	if reservedWord(name) {
		r.printf("%s\n", Wording(dg.TypeKeyword, "%[1]s is a shell keyword", name))
		return 0
	}
	// The same guard `command -v` has, and for the same reason: there is an
	// executable called /usr/bin/umask and this shell will not run it, so
	// saying where it is would be answering about the wrong thing.
	if !r.reservedBuiltin(name) {
		if path, err := r.lookPath(name); err == nil {
			r.printf("%s\n", Wording(dg.TypeExternal, "%[1]s is %[2]s", name, path))
			return 0
		}
	}
	msg := Wording(dg.TypeNotFound, "type: %[1]s: not found", name)
	if dg.TypeNotFoundUnprefixed {
		// Two of the four write this one with nothing in front of it, where
		// every other message they print carries the shell's name.
		r.errf("%s\n", msg)
	} else {
		r.diagf("%s\n", msg)
	}
	return orDefault(dg.TypeNotFoundStatus, 1)
}
