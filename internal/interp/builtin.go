// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"
)

// builtins are commands the shell runs itself.
//
// They exist because they must: `set` and `shift` change the shell's own
// state, and a child process cannot. That is also why the gate does not see
// them — nothing leaves this process — while it does see every external
// command and every file opened for a redirection.
var builtins = map[string]func(*Runner, context.Context, []string) int{
	":":        biTrue,
	"true":     biTrue,
	"false":    biFalse,
	"set":      biSet,
	"unset":    biUnset,
	"export":   biExport,
	"shift":    biShift,
	"echo":     biEcho,
	"break":    biBreak,
	"continue": biContinue,
	"return":   biReturn,
}

// biBreak and biContinue transfer control out of a loop. They are recorded on
// the runner rather than returned as errors, because leaving a loop is
// ordinary control flow and modelling it as a failure would make every caller
// check for something that is not one.
func biBreak(r *Runner, _ context.Context, args []string) int {
	r.ctl, r.ctlDepth = controlBreak, loopDepth(args)
	return 0
}

func biContinue(r *Runner, _ context.Context, args []string) int {
	r.ctl, r.ctlDepth = controlContinue, loopDepth(args)
	return 0
}

func biReturn(r *Runner, _ context.Context, args []string) int {
	r.ctl = controlReturn
	if len(args) > 0 {
		if n, ok := atoi(args[0]); ok {
			return n
		}
	}
	return r.status
}

func loopDepth(args []string) int {
	if len(args) > 0 {
		if n, ok := atoi(args[0]); ok && n > 0 {
			return n
		}
	}
	return 1
}

// specialBuiltins are the ones POSIX marks special. Two consequences follow
// from the same list — an assignment prefixed to one persists, and a failure
// in one is fatal to a non-interactive shell — so it is one concept rather
// than two lists that could drift.
var specialBuiltins = map[string]bool{
	"break": true, ":": true, "continue": true, ".": true, "eval": true,
	"exec": true, "exit": true, "export": true, "readonly": true,
	"return": true, "set": true, "shift": true, "times": true,
	"trap": true, "unset": true,
}

func biTrue(*Runner, context.Context, []string) int  { return 0 }
func biFalse(*Runner, context.Context, []string) int { return 1 }

// biSet implements the part of `set` this slice needs: replacing the
// positional parameters.
//
// `set --` with nothing after it clears them, which is different from `set`
// with no arguments at all — that lists variables and is left unimplemented
// rather than guessed at.
func biSet(r *Runner, _ context.Context, args []string) int {
	if len(args) == 0 {
		r.errf("sh: set: listing variables is not implemented yet\n")
		return 2
	}
	if args[0] != "--" {
		r.errf("sh: set: only `set --` is implemented so far, not %q\n", args[0])
		return 2
	}
	r.Params = append([]string(nil), args[1:]...)
	return 0
}

func biUnset(r *Runner, _ context.Context, args []string) int {
	for _, name := range args {
		if name == "-v" || name == "-f" {
			continue
		}
		delete(r.Vars, name)
		delete(r.exported, name)
	}
	return 0
}

// biExport marks a name for the environment, and assigns when given a value.
func biExport(r *Runner, _ context.Context, args []string) int {
	if r.exported == nil {
		r.exported = map[string]bool{}
	}
	for _, a := range args {
		if a == "-p" {
			r.errf("sh: export: -p is not implemented yet\n")
			return 2
		}
		name, value, hasValue := strings.Cut(a, "=")
		if hasValue {
			r.setVar(name, value)
		}
		r.exported[name] = true
	}
	return 0
}

// biShift drops the first n positional parameters.
//
// Shifting past the end is where the panel splits: fatal in dash and ksh93,
// survivable in bash and zsh. The survivable answer is taken, and the
// difference is a dialect question the interpreter does not yet carry.
func biShift(r *Runner, _ context.Context, args []string) int {
	n := 1
	if len(args) > 0 {
		v, ok := atoi(args[0])
		if !ok {
			r.errf("sh: shift: %s: numeric argument required\n", args[0])
			return 2
		}
		n = v
	}
	if n > len(r.Params) {
		return 1
	}
	r.Params = r.Params[n:]
	return 0
}

// biEcho writes its arguments separated by spaces.
//
// It does not interpret backslash escapes. That is the bash and ksh93
// answer; dash and zsh expand them, which docs/spec/semantics.md records as
// an axis, and taking the majority here is a placeholder rather than a
// decision — the dialect will decide once the interpreter carries one.
func biEcho(r *Runner, _ context.Context, args []string) int {
	newline := true
	for len(args) > 0 && args[0] == "-n" {
		newline = false
		args = args[1:]
	}
	out := strings.Join(args, " ")
	if newline {
		out += "\n"
	}
	_, _ = r.stdout().Write([]byte(out))
	return 0
}
