// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"maps"
	"slices"
)

// Being *started* with a value for a name is not an assignment, and the names
// whose assignment does something have to hear it anyway.
//
// SetAssignmentAction is delivered from the store, which is every route a
// script can write a name by — plain, `export`, `declare`, a command prefix,
// `local` on the way in, and the frame being popped (#4045). What no route
// reaches is the environment the shell was launched with: nothing stores it,
// because a name that came in that way is answered out of Env and was never
// put in Vars at all.
//
// That is the one route a user actually reaches for. `BASH_COMPAT=44 make`, a
// level exported from a parent shell, a value in a profile — the environment is
// where a mistyped startup parameter lives, and it was the one place the
// complaint #4262 added could not be heard. Measured 2026-09-22 on bash 5.3.20:
// `BASH_COMPAT=abc bash -c :` complains before the first line on every route
// into the shell, `-c`, a script file and standard input alike.
//
// The sibling of SetOptionList's inherit function, and a second seam rather
// than the same one, because the two are different kinds of name. An option
// namespace is a produced, readonly variable holding a list this shell knows
// how to apply; this is an **ordinary parameter** whose value means something
// to the dialect, which no list can express.

// SetInheritedParameterAction says what happens when this shell is started
// holding a value for an ordinary parameter.
//
// The action is handed the inherited value and runs once, before the first line
// — see ApplyInheritedParameters for where the front end puts that. A name the
// environment did not carry is not called for at all; one it carried empty is,
// because "inherited and empty" is a value a dialect may have something to say
// about and is not the same fact as "not inherited".
//
// Per name, for the reason SetAssignmentAction is per name: a hook consulted
// for every entry in the environment is a cost every shell start pays for a
// question about one.
func (r *Runner) SetInheritedParameterAction(name string, act func(r *Runner, value string)) {
	if r.inheritedParameterActions == nil {
		r.inheritedParameterActions = map[string]func(*Runner, string){}
	}
	r.inheritedParameterActions[name] = act
}

// ApplyInheritedParameters delivers what this shell was launched holding to the
// names a dialect registered through SetInheritedParameterAction.
//
// **A front end has to call this, and has to call it beside
// ApplyInheritedShellOptions.** They are two methods because they are two kinds
// of name, and the ordering between them is measured rather than free: on bash
// 5.3.20, `SHELLOPTS=nosuchopt BASH_COMPAT=abc bash -c 'echo hi'` writes the
// parameter's complaint *first* and the option list's second. So this goes
// before that one, and both go after the argument vector has been judged — an
// invocation option this shell does not have ends the shell before either is
// read, measured on `bash -O nosuchopt`.
//
// Only the *inherited* value is read, never one a script assigned, which is the
// same rule ApplyInheritedShellOptions follows: what is being asked is what this
// shell was launched with, and a store has its own seam.
//
// Sorted, so that a shell registering two of these cannot have the order of
// what it writes depend on a map's iteration.
func (r *Runner) ApplyInheritedParameters() {
	for _, name := range slices.Sorted(maps.Keys(r.inheritedParameterActions)) {
		value, ok := r.inheritedValue(name)
		if !ok {
			continue
		}
		r.inheritedParameterActions[name](r, value)
	}
}

// DiagnoseAsTheShell writes a diagnostic located by the shell's **own name and
// nothing else** — no line, and not the script's name either.
//
// Diagnosef is the ordinary door and builds a location: the line a statement was
// entered at, the file it was read from, the function it is inside. This one is
// for a complaint made before any of that exists, and the shape is measured
// rather than chosen. On bash 5.3.20, an out-of-range `BASH_COMPAT` in the
// environment is:
//
//	bash: BASH_COMPAT: abc: compatibility value out of range
//
// with no line on any route — and `bash: line 0: …` for the inherited option
// list in the same run, which is what ApplyInheritedShellOptions already
// produces. Two shapes, measured side by side, so this is a second door rather
// than the same one with a zero in it.
//
// The name is the shell's own on every route, including a script file, where an
// ordinary diagnostic names the script: nothing of the script has been read yet.
// That is Runner.invokedAs — argv[0] — which is the same name
// Diagnostics.BuiltinNamesTheShellAlone writes for the other complaint that
// stands outside a script's lines.
func (r *Runner) DiagnoseAsTheShell(format string, args ...any) {
	r.errf("%s%s", r.diag().prefixWithoutLine(r.invokedAs(), ""), fmt.Sprintf(format, args...))
}
