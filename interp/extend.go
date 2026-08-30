// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "context"

// Builtin is a command the shell runs itself rather than executing.
//
// It returns an exit status. Writing to r.Stdout and r.Stderr rather than to
// the process's own streams is what makes it work inside a pipeline and under
// a redirection, because those are what the runner points at the right place.
type Builtin func(r *Runner, ctx context.Context, args []string) int

// Register adds a builtin, replacing any of the same name.
//
// This is one of the two ways to build a shell on this package, and the line
// between them is worth stating because most of a dialect belongs on the
// other side of it.
//
// Register what cannot be written in shell: commands that must reach the
// interpreter's own state or the operating system. `cd` has to change the
// working directory the runner uses, `read` has to put a value into a
// variable of the calling shell, `local` has to create a scope, `trap` has to
// install a handler. None of those can be expressed as a shell function
// because a shell function has no way to say them.
//
// Everything else is better as a **shell function**, sourced as a prelude and
// needing no Go at all. Functions already shadow builtins and external
// commands alike, so a dialect layer can define, replace or wrap anything
// this package provides without touching it:
//
//	basename() { printf '%s\n' "${1##*/}"; }
//	echo()     { printf '%s\n' "$*"; }   # replaces the builtin
//
// The distinction is not stylistic. A prelude is portable across every
// implementation of this language, can be tested with any shell, and cannot
// break the substrate. Reach for Go when shell genuinely cannot express the
// thing, and not before.
func (r *Runner) Register(name string, fn Builtin) {
	if r.custom == nil {
		r.custom = map[string]Builtin{}
	}
	r.custom[name] = fn
}

// Unregister removes a builtin, including one this package provides.
//
// A dialect that does not have a command should not offer it, and hiding one
// is how that is said — the same reasoning as the parser refusing a construct
// the dialect lacks rather than accepting it and meaning something else.
func (r *Runner) Unregister(name string) {
	if r.custom == nil {
		r.custom = map[string]Builtin{}
	}
	r.custom[name] = nil
}

// lookupBuiltin resolves a name, letting a registration win over the built-in
// table so a dialect can replace as well as add.
func (r *Runner) lookupBuiltin(name string) (Builtin, bool) {
	if fn, ok := r.custom[name]; ok {
		// A nil entry is an explicit removal rather than a missing one.
		return fn, fn != nil
	}
	fn, ok := builtins[name]
	return fn, ok
}

// IsSpecialBuiltin reports whether a name is one of the POSIX special
// builtins, which a dialect layer needs in order to match the two behaviours
// that follow from that list: an assignment prefixed to one persists, and a
// failure in one is fatal to a non-interactive shell.
func IsSpecialBuiltin(name string) bool { return specialBuiltins[name] }
