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
var compgenActions = map[string]func(r *Runner, word string) []string{
	"builtin":  func(r *Runner, _ string) []string { return r.BuiltinNames() },
	"function": func(r *Runner, _ string) []string { return r.scriptFuncNames() },
	"file":     func(r *Runner, word string) []string { return r.compgenFilenames(word, false) },
	"directory": func(r *Runner, word string) []string {
		return r.compgenFilenames(word, true)
	},
	"variable": func(r *Runner, _ string) []string { return r.compgenVariableNames() },
}

// compgenActionOrder is the order the actions asked for are generated in, and
// it is the shell's rather than the command line's: measured 2026-09-12,
// `compgen -bdf -A function ec` and every permutation of the same four answer
// in one order — the builtins, then the functions, then the files, then the
// directories. The variables sit between the functions and the files,
// measured 2026-09-21 on bash 5.3.20 over one name of each kind spelled `zz…`:
// `compgen -bvf -A function zz` is the function, the variable, the directory
// and the file, in that order, and `compgen -v -A function zz` is the
// function before the variable on its own. So `compgen -df a` and `compgen -fd a` both give the files
// before the directory, and both give the directory twice, once for each
// action that generated it. Duplicates are not removed anywhere: a completion
// list is what the generators produced.
//
// A slice and not a walk of the map above, because a map has no order and the
// answer does.
var compgenActionOrder = []string{"builtin", "function", "variable", "file", "directory"}

// compgenActionLetters are the short spellings of those actions.
//
// bash's short letters are `abcdefgjksuv` and **none of them is `function`** —
// the only spelling for that action is the long `-A function`. This map said
// `u` meant function until it was measured; `compgen -u` in bash lists *user*
// names, so the claim was wrong in the one direction that is invisible, giving
// an answer where bash gives a different one rather than failing.
//
// `v` is `variable` and the two spellings are one action: measured
// 2026-09-21 on bash 5.3.20, `diff <(compgen -v) <(compgen -A variable)` is
// empty.
var compgenActionLetters = map[byte]string{
	'b': "builtin",
	'd': "directory",
	'f': "file",
	'v': "variable",
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

// compgenGenerating are the three `-o` names that produce words rather than
// shape a list somebody else produced. #2412's row read `-o` as an *option
// reader* — `compgen -o default | head -1` should print `cmd` — and it is
// nothing of the kind: `-o` names one of the nine completion options, and
// three of them generate. Measured 2026-09-12 against bash 5.3.15:
//
//   - `default` is readline's own completion, which is filenames, and only as
//     a **fallback**: `compgen -o default -A builtin ec` answers `echo` and no
//     file at all, so it contributes nothing once something else has matched.
//   - `dirnames` is the same fallback with directories, and it wins over
//     `default` where both are given — `compgen -o default -o dirnames a`
//     answers the directory alone.
//   - `plusdirs` is not a fallback: it appends the directories to whatever was
//     generated, so `compgen -o plusdirs -f a` answers the three names `-f`
//     found and then the directory again.
//
// The other six — `bashdefault`, `filenames`, `fullquote`, `noquote`,
// `nosort`, `nospace` — shape a word list and generate nothing, at status 1
// when nothing else did. They are still *asked for*, which is why an `-o` of
// any name makes an empty answer a failure where a bare `compgen a` is a quiet
// success.
type compgenGenerating struct {
	def      bool
	dirnames bool
	plusdirs bool
}

func biCompgen(r *Runner, _ context.Context, args []string) int {
	var word string
	var opts compgenGenerating
	asked := map[string]bool{}
	// The action types bash has and this shell cannot generate, so that each
	// is said once however many times it was asked for. bash itself folds a
	// repeated action — `compgen -A alias -A alias` over one alias answers
	// one name — so the set that reports follows the set that generates.
	said := map[string]bool{}
	generated, haveWord := false, false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			if i+1 < len(args) && !haveWord {
				word, haveWord = args[i+1], true
			}
			i = len(args)
		case len(a) > 1 && a[0] == '-':
			for j := 1; j < len(a); j++ {
				// The two letters that take an argument, which is attached
				// where there is more of the word left and the next operand
				// otherwise: measured, `-Abuiltin`, `-A builtin`, `-odefault`
				// and `-fo default` all read the way the spaced form does.
				if letter := a[j]; letter == 'A' || letter == 'o' {
					// Read off before the argument is taken: taking it moves
					// the cursor past the end of the cluster, so `a[j]` is no
					// longer this letter and was a panic on `compgen -o`.
					arg, ok := compgenOptArg(args, &i, a, &j)
					if !ok {
						r.diagf("compgen: -%c: option requires an argument\n", letter)
						r.compgenUsage()
						return 2
					}
					if letter == 'A' {
						if !compgenKnownActions[arg] {
							// A name bash has no action for, which is the
							// script's typo rather than this shell's gap.
							r.diagf("compgen: %s: invalid action name\n", arg)
							return 2
						}
						if _, ok := compgenActions[arg]; !ok {
							// Said, and then contributed nothing. It used to
							// end the call, which took the *implemented*
							// action types down with it: `compgen -A builtin
							// -A function -A alias -A keyword` answered
							// nothing at all where bash answers everything
							// but the gap. An action this shell cannot
							// generate is one that produced no words, and
							// bash's own answer for an action with no matches
							// is exactly that (#3899).
							r.compgenNotImplemented(arg, said)
							generated = true
							break
						}
						asked[arg], generated = true, true
						break
					}
					if !completionOption(arg) {
						r.diagf("compgen: %s: invalid option name\n", arg)
						return 2
					}
					switch arg {
					case "default":
						opts.def = true
					case "dirnames":
						opts.dirnames = true
					case "plusdirs":
						opts.plusdirs = true
					}
					// Every `-o`, generating or not, means the completion
					// machinery ran — see compgenGenerating.
					generated = true
					break
				}
				name, ok := compgenActionLetters[a[j]]
				if !ok {
					if compgenLetters[a[j]] {
						// bash has the letter and we do not generate it, so
						// it contributes nothing and the rest of the cluster
						// answers — the same door the long spelling goes
						// through, because it is the same gap written the
						// short way. `compgen -bv cd` is `cd` in bash, and
						// two copies of this decision is how one of them
						// comes to disagree with the other.
						r.compgenNotImplemented("-"+string(a[j]), said)
						generated = true
						continue
					}
					// bash does not have it either, so the script has a
					// typo rather than a shell that is missing something —
					// and *that* still ends the call, in bash as here:
					// measured, `compgen -z -b cd` writes the usage line and
					// reports 2 with no listing. The two categories part
					// exactly here.
					//
					// The usage line follows, as it does after a missing
					// option argument — and unlike after an `-o` name or an
					// action name that is not one, which are one line each.
					// Measured, all four.
					r.diagf("compgen: -%c: invalid option\n", a[j])
					r.compgenUsage()
					return 2
				}
				asked[name], generated = true, true
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
	var out []string
	for _, name := range compgenActionOrder {
		if !asked[name] {
			continue
		}
		for _, candidate := range compgenActions[name](r, word) {
			if strings.HasPrefix(candidate, word) {
				out = append(out, candidate)
			}
		}
	}
	if len(out) == 0 {
		// The fallbacks, and only here: they are what readline would have
		// done with a word nothing else could complete. `dirnames` ahead of
		// `default` because that is the order measured when both are given.
		switch {
		case opts.dirnames:
			out = append(out, r.compgenFilenames(word, true)...)
		case opts.def:
			out = append(out, r.compgenFilenames(word, false)...)
		}
	}
	if opts.plusdirs {
		// Added rather than fallen back on, so this is outside the branch
		// above and after it: `compgen -o plusdirs -f a` answers four names
		// with the directory in it twice.
		out = append(out, r.compgenFilenames(word, true)...)
	}
	if len(out) == 0 {
		// Nothing matched, which is a failure rather than an empty success:
		// a completer asks whether there is anything to offer.
		return 1
	}
	for _, name := range out {
		r.printf("%s\n", name)
	}
	return 0
}

// compgenNotImplemented says that an action bash has and this shell cannot
// generate was asked for, once per spelling however many times it was asked.
//
// One door for the long `-A alias` and the short `-v`, because they are the
// same gap written two ways and the two copies this replaced had already
// begun to differ from each other in what they took down with them.
//
// It reports and returns, and the caller carries on: an action this shell
// cannot generate is an action that produced no words, which is what bash's
// own answer for an action with no matches already is — measured 2026-09-20,
// `compgen -A alias zzz` on bash 5.3.20 is silence at 1. What must *not*
// carry on is a name or letter bash does not have either: that is the
// script's typo, and bash ends the call for it even in company (#3899).
func (r *Runner) compgenNotImplemented(name string, said map[string]bool) {
	if said[name] {
		return
	}
	said[name] = true
	r.diagf("compgen: %s: not implemented\n", name)
}

// compgenVariableNames is the `-v` action: every name the shell holds a
// **value** for, sorted, whether the value is visible from here or standing
// behind a declaration that displaced it.
//
// Two halves, and the second is the one the obvious implementation misses.
//
// A name has to hold a value: measured 2026-09-21, `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME, from a script file, on
// bash 5.3.20 — `declare xyz`, `declare -i ii`, `declare -a arr`, `declare -A
// tbl` and `declare -x ee` are none of them listed, where `arr2=(1 2)` and
// `e2=` both are. So it is not the declaration listing's population: a
// valueless declaration is a row in `declare -p` and is not a completion
// candidate, and an array with elements is both.
//
// And a value a *scope* is holding counts. With `v=global` outside,
// `f() { local v; unset v; compgen -v | grep -c '^v$'; }` is **1** there, and
// the same function over a name with no global — `local nov; unset nov` — is
// **0**. So what the first row lists is the global the declaration displaced
// and not the placeholder the `unset` left: the placeholder on its own is not
// a candidate, which is the same answer `[[ -v v ]]` gives it. #4069 was
// filed reading that row the other way round, and the `nov` control is what
// separates the two.
//
// The whole-table walk is deliberately not reused. declarableNames collects
// a name for an *attribute* as much as for a value, and it asks an axis about
// the valueless record on the way — neither of which this question wants.
func (r *Runner) compgenVariableNames() []string {
	seen := map[string]bool{}
	hold := func(name string) {
		if !r.removed[name] {
			seen[name] = true
		}
	}
	for _, kv := range r.Env {
		// A name the shell was launched with and has never assigned is in
		// none of the tables below and is a candidate all the same: `PATH`,
		// `HOME` and `LC_ALL` are all in bash's own `compgen -v` under
		// `env -i` with exactly those three set. See Runner.inheritedEnv,
		// which is the same reading on the other side of the same fact.
		if name, _, ok := strings.Cut(kv, "="); ok {
			hold(name)
		}
	}
	for name := range r.Vars {
		hold(name)
	}
	compound := func(name string) {
		// A compound a declaration brought into being and nothing has
		// written to is the compound sibling of the valueless scalar above,
		// and it is not a candidate either: measured in the same run,
		// `declare -a arr` and `declare -A tbl` are both 0 where
		// `declare -a e=()` and `declare -A g=()` — an **empty literal**, so
		// something did assign — are both 1. That is the difference
		// compounddeclaredonly.go already keeps, so it is read here rather
		// than counted again from the elements, which cannot tell an emptied
		// array from one nothing ever filled.
		if !r.declaredOnlyCompound[name] {
			hold(name)
		}
	}
	for name := range r.Arrays {
		compound(name)
	}
	for name := range r.AssocArrays {
		compound(name)
	}
	for name := range r.dynamicDeclarations {
		// A parameter the shell produces on each read is a name a script can
		// complete: bash lists `RANDOM`, `SECONDS` and the rest of its own
		// beside the script's. producedDeclaration is what says the name is
		// still there, so an `unset` of one takes it out of this too.
		if _, ok := r.producedDeclaration(name); ok {
			seen[name] = true
		}
	}
	for _, sc := range r.scopes {
		// The value a declaration displaced, which is still the shell's and
		// is what the measurement above says is listed. Through the
		// `existed` maps rather than the saved ones, because a scope records
		// a shadow of a name that held nothing at all and that name is not a
		// candidate either.
		for name, existed := range sc.existed {
			if existed {
				seen[name] = true
			}
		}
		for name, existed := range sc.arrayExisted {
			if existed {
				seen[name] = true
			}
		}
		for name, existed := range sc.assocExisted {
			if existed {
				seen[name] = true
			}
		}
	}
	return sortedNames(seen)
}

// compgenOptArg reads the argument of a letter that takes one, attached to the
// letter or standing as the next operand, and advances both cursors past it.
//
// The two indices are the caller's loop variables and are moved rather than
// returned, because a cluster is walked letter by letter and an argument ends
// the cluster wherever it was found: `-fo default` has read `f` already and
// must not go back for a letter that is now part of `default`.
func compgenOptArg(args []string, i *int, cluster string, j *int) (string, bool) {
	if *j+1 < len(cluster) {
		arg := cluster[*j+1:]
		*j = len(cluster)
		return arg, true
	}
	if *i+1 >= len(args) {
		return "", false
	}
	*i++
	*j = len(cluster)
	return args[*i], true
}

// compgenUsage writes the line that follows a missing option argument.
//
// Through errf and not diagf, which is the whole reason it is a function: the
// diagnostic above carries the shell's own location prefix and this line
// carries none. Measured 2026-09-12 — `bash -c 'compgen -o'` writes
// `<shell>: line 1: compgen: -o: option requires an argument` and then this
// text at the start of its own line. A missing argument and an invalid option
// *letter* both reach it; an `-o` name or an action name that is not one is a
// single line with no usage after it, which is measured too.
//
// The text is bash 5.3's. bash 3.2 writes a shorter one in a different order —
// no `-V varname`, and the filter and function letters the other way round —
// and it is in the record beside this; there is no bash 3.2 dialect to spell
// it for, so the one string stands rather than becoming a Diagnostics field
// with a single answer.
func (r *Runner) compgenUsage() {
	r.errf("compgen: usage: compgen [-V varname] [-abcdefgjksuv] [-o option] " +
		"[-A action] [-G globpat] [-W wordlist] [-F function] [-C command] " +
		"[-X filterpat] [-P prefix] [-S suffix] [word]\n")
}
