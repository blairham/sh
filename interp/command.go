// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"
)

// The two ways to ask for a command without asking a function.
//
// `command name` runs the builtin or the external and never the function of
// that name, which is what lets a function wrap the thing it is named after —
// `ls() { command ls --color "$@"; }` is the whole reason it exists.
//
// `command -v name` asks *what* would run rather than running it, and is how a
// portable script tests whether it has a tool. It is unanimous across the
// panel except for the status when the answer is nothing.

// Registered here rather than in the table beside the others: `command` and
// `builtin` both reach the dispatcher that reads that table, and Go calls a
// literal closing that loop an initialization cycle. `eval` and `.` are added
// the same way and for the same reason.
func init() {
	builtins["command"] = biCommand
	builtins["builtin"] = biBuiltin
}

// biCommand runs a command with functions bypassed, or reports what one is.
func biCommand(r *Runner, ctx context.Context, args []string) int {
	verbose := false
	for len(args) > 0 && strings.HasPrefix(args[0], "-") && args[0] != "--" {
		switch args[0] {
		case "-v":
			verbose = true
		case "-p":
			// "Use a default PATH". Ours is already the Runner's rather than
			// the process's, and inventing a second one would be a guess
			// about this machine — so it is accepted and changes nothing,
			// which is what it means for a shell that never had the
			// developer's PATH to begin with.
		default:
			r.diagf("command: %s: invalid option\n", args[0])
			return 2
		}
		args = args[1:]
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return 0
	}
	if verbose {
		return r.reportWhatRuns(args[0])
	}
	return r.runWithoutFunctions(ctx, args)
}

// biBuiltin runs a builtin, and only a builtin.
//
// bash and zsh have it; dash has no such command, and ksh93 has one of the
// same name that does something else entirely — it *registers* builtins — so
// this is registered per dialect rather than being part of the substrate.
func biBuiltin(r *Runner, ctx context.Context, args []string) int {
	if len(args) == 0 {
		return 0
	}
	fn, ok := r.lookupBuiltin(args[0])
	if !ok {
		// The location does not name the builtin here, in the dialect that
		// names it everywhere else: this message is *about* a name that is
		// not one, so there is no builtin speaking. Measured — `zsh:cd:1:`
		// and `zsh:shift:1:` against a plain `zsh:1: no such builtin`.
		outer := r.inBuiltin
		r.inBuiltin = ""
		r.diagf("%s\n", Wording(r.diag().NotABuiltin, "builtin: %s: not a shell builtin", args[0]))
		r.inBuiltin = outer
		return 1
	}
	return fn(r, ctx, args[1:])
}

// reportWhatRuns answers `command -v`.
//
// A builtin or a function is named as it was written; an external is named by
// the path that would be run, because that is the part a script cannot work
// out for itself. Nothing found is a failure with no output at all — the
// silence is what makes `command -v x >/dev/null` the usual spelling.
func (r *Runner) reportWhatRuns(name string) int {
	if _, ok := r.funcs[name]; ok {
		r.printf("%s\n", name)
		return 0
	}
	if _, ok := r.lookupBuiltin(name); ok {
		r.printf("%s\n", name)
		return 0
	}
	if reservedWord(name) {
		r.printf("%s\n", name)
		return 0
	}
	if path, err := r.lookPath(name); err == nil {
		r.printf("%s\n", path)
		return 0
	}
	if r.ask(r.sem().CommandNotFoundStatusIsNotFound, "`command -v` reporting a missing name as not found") {
		// One shell answers with the status a missing *command* has — 127 —
		// rather than a plain failure, which matters to a script that tests
		// the number rather than just the truth of it.
		return 127
	}
	return 1
}

// reservedWord reports whether a name is part of the grammar rather than a
// command. `command -v if` answers `if` in every shell in the panel.
func reservedWord(name string) bool {
	switch name {
	case "if", "then", "else", "elif", "fi", "for", "while", "until", "do",
		"done", "case", "esac", "in", "function", "select", "time", "{", "}",
		"[[", "]]", "!":
		return true
	}
	return false
}

// runWithoutFunctions runs a command with the function table ignored, which
// is the whole of what `command name` means: a function may then wrap the
// thing it is named after without calling itself.
func (r *Runner) runWithoutFunctions(ctx context.Context, args []string) int {
	if fn, ok := r.lookupBuiltin(args[0]); ok {
		outer := r.inBuiltin
		r.inBuiltin = args[0]
		st := fn(r, ctx, args[1:])
		r.inBuiltin = outer
		return st
	}
	if err := r.exec(ctx, args, r.environ()); err != nil {
		r.diagf("command: %v\n", err)
		return 1
	}
	return r.status
}
