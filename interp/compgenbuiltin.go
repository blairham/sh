// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"
)

// `compgen` writes the completions a word would have.
//
// bash alone: dash, ksh93 and zsh have no such command, so it is registered
// here and taken away by the three without it, the way `enable` is.
//
// It is a large builtin — a dozen option letters, a dozen actions, and hooks
// for running a function or a command to generate words. What is here is the
// part this shell can answer *from what it knows*: the names of its own
// commands and of the functions that have been defined. Everything else is
// refused rather than answered with a guess, on the same rule the `set -o`
// table follows — an action we cannot generate is a promise we cannot keep.
//
// Homebrew's `brew` uses the first of them, `compgen -A builtin`, to walk
// the shell's own commands and make sure none of them has been shadowed.
func init() {
	builtins["compgen"] = biCompgen
}

// compgenActions are the actions this shell can generate, by the name
// `-A` gives them and by the letter that is short for the same thing.
var compgenActions = map[string]func(r *Runner) []string{
	"builtin":  (*Runner).BuiltinNames,
	"function": (*Runner).FuncNames,
}

// compgenActionLetters are the short spellings of those actions. bash has one
// for most of its actions; these are the two that go with what is generated
// here.
var compgenActionLetters = map[byte]string{
	'b': "builtin",
	'u': "function",
}

// compgenKnownActions are every action bash has, so that one it has and this
// shell cannot generate is refused as unimplemented rather than reported as a
// name that does not exist. The difference matters to a script: the first is
// a shell that is missing something, the second is a typo.
var compgenKnownActions = map[string]bool{
	"alias": true, "arrayvar": true, "binding": true, "builtin": true,
	"command": true, "directory": true, "disabled": true, "enabled": true,
	"export": true, "file": true, "function": true, "group": true,
	"helptopic": true, "hostname": true, "job": true, "keyword": true,
	"running": true, "service": true, "setopt": true, "shopt": true,
	"signal": true, "stopped": true, "user": true, "variable": true,
}

func biCompgen(r *Runner, _ context.Context, args []string) int {
	var action, word string
	generated := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			if i+1 < len(args) {
				word = args[i+1]
			}
			i = len(args)
		case a == "-A":
			i++
			if i >= len(args) {
				r.diagf("compgen: -A: option requires an argument\n")
				return 2
			}
			action, generated = args[i], true
		case len(a) > 1 && a[0] == '-':
			for j := 1; j < len(a); j++ {
				name, ok := compgenActionLetters[a[j]]
				if !ok {
					// Reported as unimplemented rather than as a bad option:
					// bash has these letters and we do not generate them.
					r.diagf("compgen: -%c: not implemented\n", a[j])
					return 2
				}
				action, generated = name, true
			}
		default:
			word = a
		}
	}
	if !generated {
		// No action asked for, so nothing to generate and nothing to say —
		// which bash reports as success even though it produced no words.
		return 0
	}
	gen, ok := compgenActions[action]
	if !ok {
		if compgenKnownActions[action] {
			r.diagf("compgen: %s: not implemented\n", action)
		} else {
			r.diagf("compgen: %s: invalid action name\n", action)
		}
		return 2
	}
	matched := false
	for _, name := range gen(r) {
		if !strings.HasPrefix(name, word) {
			continue
		}
		matched = true
		r.printf("%s\n", name)
	}
	if !matched {
		// Nothing matched, which is a failure rather than an empty success:
		// a completer asks whether there is anything to offer.
		return 1
	}
	return 0
}
