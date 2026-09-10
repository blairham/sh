// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"
)

// `functions` and `unfunction`, which are `typeset -f` and `unset -f` under
// second names.
//
// zsh has both words and ksh93 has the first; bash and dash have neither, so
// which names exist is a dialect's answer rather than an axis — the same
// shape `declare` and `integer` have, and registered through the same
// extension seam. What they are *not* is a second implementation. A function
// said back is [Runner.declareFunctions] here as much as `typeset -f` is, and
// a function removed is [Runner.unsetFunction], so the listing's arrangement,
// the prelude's functions being left out of it, the status a name nobody
// defined leaves behind and the wording of the complaint all come from the
// one place. The seventh time a second helper in this tree quietly missed a
// fix the first one had is what makes that worth stating.
//
// Two things the second names decide for themselves, and each is a dialect's
// table rather than a branch here: which letters each takes —
// [Semantics.FunctionsOptions] and [Semantics.UnfunctionOptions], both
// *narrower* than the builtin they rename — and what their refusals call
// them, which follows the invoked name through r.inBuiltin the way every
// other builtin's does.
//
// Measured 2026-09-08 against zsh 5.9.2 and ksh93u+ in the oracle
// environment, with a scrubbed HOME and no startup files.
//
//   - **`functions name` is `typeset -f name` exactly**, down to the status:
//     the body is written out, a name nobody defined is silent at 1, and 1
//     stands however many other names printed.
//   - **`unfunction name` is `unset -f name` exactly**, including the
//     complaint. zsh's `unset -f nosuch` already said `no such hash table
//     element: nosuch` at 1 here, and `unfunction nosuch` says the identical
//     sentence with its own name in the location — which is what falls out
//     of running the same code rather than being arranged.
//   - **The letters are narrower than the builtin renamed.** `functions -f`
//     and `functions -p` are bad options in zsh though `typeset` takes both,
//     and `unfunction` takes `-m` and nothing else — not even the `-f` that
//     is its whole meaning. So the letter sets are their own fields and the
//     renamed builtin's are not reused.
//   - **`functions -M` is the one thing here that is not a rename.** It
//     registers a shell function as a *math* function callable from
//     arithmetic, which needs the evaluator to call back into the
//     interpreter. That seam is interp/mathfunc.go, which is also where the
//     measurement lives: the value a math function returns is the last
//     arithmetic evaluated during the call and is *not* `REPLY`, whatever the
//     documentation says. Nothing about it is decided here — this file only
//     separates the letter from the operands and hands them over, and whether
//     the dialect has the letter at all is [Semantics.FunctionsOptions].

// FunctionsBuiltin is the `functions` builtin, for a dialect that has the
// word to register.
//
// Exported for the same reason [IntegerBuiltin] is: a dialect package cannot
// reach an unexported function, and the alternative is a copy of the listing
// in `dialect/zsh` that will drift from the one here.
func FunctionsBuiltin() Builtin { return biFunctions }

// UnfunctionBuiltin is the `unfunction` builtin, for a dialect that has the
// word.
func UnfunctionBuiltin() Builtin { return biUnfunction }

func biFunctions(r *Runner, _ context.Context, args []string) int {
	name := r.inBuiltin
	if name == "" {
		name = "functions"
	}
	// Read before the flags rather than out of them: `-m` is not one of the
	// declaration's attributes and putting a case for it in
	// parseDeclareFlags would add a letter to a parser shared with `typeset`,
	// `declare`, `local` and `integer` for the sake of one caller.
	matching := hasOption(args, 'm')
	// `-M` and `+M` are read before the flags for the same reason `-m` is,
	// and one more: they take *operands*, not names to declare, so the
	// declaration's parser has nothing to do with them. The letter being in
	// FunctionsOptions is what says this dialect has the facility at all —
	// where it is not, `-M` falls through to the option parser and is
	// refused as a letter this engine does not spell, which is what a shell
	// with the word and without the facility should say.
	if strings.ContainsRune(r.sem().FunctionsOptions, 'M') {
		switch remove, operands, verdict := mathFunctionLetter(args); verdict {
		case mathLetterAlone:
			return r.mathFunctionsBuiltin(name, remove, operands)
		case mathLetterWithMatching:
			// Measured: the two letters together do nothing at all, quietly.
			return 0
		}
	}
	args, f, code := r.parseDeclareFlags(name, args, r.sem().FunctionsOptions)
	if code != 0 {
		// The same fatality `typeset` has and for the same reason: ksh93
		// counts its declaration builtins among the special ones, so an
		// honest refusal of a letter it spells and this shell does not stops
		// the script exactly where a bad option to `typeset` stops it.
		if r.ask(r.sem().TypesetBadOptionFatal, "a bad `typeset` option ending the script") {
			r.status = code
			r.fatalQuiet()
		}
		return code
	}
	if matching {
		return r.functionsMatching(args, false)
	}
	// The name asked for the function table, so the operands are function
	// names however the letters were written — and the *sign* the word was
	// written with says which listing, exactly as the sign of `typeset`'s
	// `f` letter does. This word spells no `f`, so there is no letter to
	// carry the sign and the option word carries it: measured 2026-09-10 on
	// zsh 5.9.2, `functions +` names every function where `functions -`
	// writes them all out, and `functions + f` names the one (#1576).
	//
	// Not the same reading `-m` gets, which is measured too and is the
	// reason for the guard rather than a bare assignment: `functions +m
	// 'f*'` writes the body exactly as `functions -m 'f*'` does, so the
	// pattern letter takes the sign away from the word. `hasOption` above
	// reads a minus word alone, so a `+m` line arrives here rather than at
	// functionsMatching and would otherwise carry the word's sign into
	// declareMatching, where the same field means the other thing.
	f.function = true
	if !f.matching {
		f.functionOff = f.remove
	}
	return r.declareNames(name, args, f)
}

func biUnfunction(r *Runner, _ context.Context, args []string) int {
	name := r.inBuiltin
	if name == "" {
		name = "unfunction"
	}
	args, opts, code := r.builtinOptions(name, args, r.sem().UnfunctionOptions)
	if code != 0 {
		return code
	}
	if len(args) == 0 {
		// Not the silence a bare `unset` leaves in three of the panel: the
		// only shell with this word complains, and it is the same sentence
		// its `unset` with no operand writes. See unsetWithoutOperands.
		return r.unsetWithoutOperands(name)
	}
	return r.unsetFunctions(args, strings.ContainsRune(opts, 'm'))
}

// functionsMatching is `functions -m` and `typeset -fm`: each operand is a
// pattern and every function whose *name* it matches is written out.
//
// Patterns outside and names inside, which is measured rather than chosen —
// `functions -m 'f*' fa` writes `fa` twice in zsh 5.9.2, so a name reached by
// two patterns is said once per pattern. Nothing matching at all is still 0,
// where the same letter on `unfunction` reports 1: a listing that found
// nothing has answered the question, and a removal that removed nothing has
// not. No pattern at all is the whole listing, which is the one place the
// letter decides nothing.
//
// namesOnly is the sign of the `f` letter the declaration builtin was given —
// `typeset +fm '_*'` names the matches and `typeset -fm '_*'` writes their
// bodies. `functions` spells no `f` at all, so it is always the body: measured,
// `functions +m 'f*'` writes the function out exactly as `functions -m` does.
// One walk for both names rather than two, for the reason the file comment
// gives.
func (r *Runner) functionsMatching(patterns []string, namesOnly bool) int {
	if len(patterns) == 0 {
		return r.declareFunctions(nil, namesOnly, false)
	}
	for _, pattern := range patterns {
		o := r.patternOpts(pattern)
		for _, name := range r.scriptFuncNames() {
			if !matchPattern(pattern, name, o) {
				continue
			}
			if namesOnly {
				r.printf("%s\n", name)
				continue
			}
			r.printf("%s\n", r.listedFunction(name, r.funcs[name]))
		}
	}
	return 0
}
