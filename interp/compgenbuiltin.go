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
//
// Specified in the `compgen` section of docs/spec/semantics.md, which lists
// what is out of scope and why; the corpus rows are under `compgen/`. Both
// arrived late — this file carried its measurements in comments alone for a
// while, and one of them was wrong for exactly as long.
func init() {
	builtins["compgen"] = biCompgen
}

// compgenActions are the actions this shell can generate, by the name
// `-A` gives them and by the letter that is short for the same thing.
//
// `function` asks the *script's* functions and not every function defined,
// which is the third caller of #603's rule and the one that took longest to
// find (#1081). A completion generator asking what functions exist is asking
// the question `declare -F` asks, so it gets the same answer: a dialect
// written as shell had `__dirs_rotate`, `dirs`, `popd` and `pushd` in the
// list, at status 0, and `compgen -A function pu` turned bash's "no matches"
// into a match. Both are the silent kind — a plausible list, and nothing
// said.
//
// The exported [Runner.FuncNames] is deliberately *not* what is asked here,
// and that is the whole of why #1035 could not reach this. It is every
// function callable, which is what the line editor completes from, and
// narrowing it would cost `pushd` its completion at a prompt. Two callers,
// two sets, and one predicate behind both — see [Runner.speaksForTheShell].
var compgenActions = map[string]func(r *Runner) []string{
	"builtin":  (*Runner).BuiltinNames,
	"function": (*Runner).scriptFuncNames,
}

// compgenActionLetters are the short spellings of those actions.
//
// One entry, and that is the measurement rather than a stub. bash's short
// letters are `abcdefgjksuv` and **none of them is `function`** — the only
// spelling for that action is the long `-A function`. This map said `u` meant
// function until it was measured; `compgen -u` in bash lists *user* names, so
// the claim was wrong in the one direction that is invisible, giving an answer
// where bash gives a different one rather than failing.
var compgenActionLetters = map[byte]string{
	'b': "builtin",
}

// compgenLetters is every letter bash's option string has, so that one it has
// and this shell does not generate is refused as unimplemented while a letter
// bash does not have is an invalid option — the same distinction
// compgenKnownActions draws for the long names. Measured from bash 5.3's own
// usage line:
//
//	compgen [-V varname] [-abcdefgjksuv] [-o option] [-A action]
//	        [-G globpat] [-W wordlist] [-F function] [-C command]
//	        [-X filterpat] [-P prefix] [-S suffix] [word]
//
// The lowercase run names actions; the rest take an argument and shape or
// generate a word list some other way. None of the second group is read here,
// so none of them reaches the point where the argument would be consumed.
var compgenLetters = map[byte]bool{
	'a': true, 'b': true, 'c': true, 'd': true, 'e': true, 'f': true,
	'g': true, 'j': true, 'k': true, 's': true, 'u': true, 'v': true,
	'o': true, 'V': true, 'G': true, 'W': true, 'F': true, 'C': true,
	'X': true, 'P': true, 'S': true,
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
	generated, haveWord := false, false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			if i+1 < len(args) && !haveWord {
				word, haveWord = args[i+1], true
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
					if compgenLetters[a[j]] {
						// bash has the letter and we do not generate it.
						r.diagf("compgen: -%c: not implemented\n", a[j])
					} else {
						// bash does not have it either, so the script has a
						// typo rather than a shell that is missing something.
						r.diagf("compgen: -%c: invalid option\n", a[j])
					}
					return 2
				}
				action, generated = name, true
			}
		default:
			// The *first* word is the one matched against, and the rest are
			// ignored: measured, `compgen -A builtin ret re` answers for
			// `ret` in bash and `compgen -A builtin re ret` for `re`.
			if !haveWord {
				word, haveWord = a, true
			}
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
