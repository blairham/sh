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
	// `-x` is read before everything else here, and it has to be: it takes a
	// *number*, which may be the rest of its own word or the word after it,
	// so a scan that has not taken it cannot tell an operand from an
	// argument. See Runner.functionsIndentLetter.
	if strings.ContainsRune(r.sem().FunctionsOptions, 'x') {
		rest, indent, found, code := r.functionsIndentLetter(name, args)
		if code != 0 {
			return code
		}
		if found {
			args = rest
			// The listing's own arrangement for the length of this call:
			// what `-x` moves is how one level of structure is written, and
			// the printer already knows where the levels are.
			saved := r.functionLayout.Indent
			r.functionLayout.Indent = strings.Repeat(" ", indent)
			defer func() { r.functionLayout.Indent = saved }()
		}
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
	if strings.ContainsRune(r.sem().FunctionsOptions, 'M') &&
		r.sem().DeclareMappingLetter == DeclareMappingLetterRegistersAMathFunction {
		// The letter is read here only where it *is* the math facility. The
		// other dialect with the word spells the same letter and means a
		// character mapping by it, and that reading takes the ordinary
		// declaration path so the letters around it can refuse the line —
		// see interp/declaremapping.go and Semantics.DeclareMappingLetter.
		switch remove, stringArg, operands, verdict := mathFunctionLetter(args); verdict {
		case mathLetterAlone:
			return r.mathFunctionsBuiltin(name, remove, stringArg, operands)
		case mathLetterWithMatching:
			// Measured: the two letters together do nothing at all, quietly.
			return 0
		case mathLetterMixed:
			// **The letter is exclusive**, and a letter this shell *has*
			// beside it makes the whole line `invalid option(s)` at 1 with
			// nothing registered. Before this the set fell through to the
			// option parser, which has no complaint about a letter it
			// accepts — so `functions -Mu mf 1 1 g` registered `mf` in
			// silence at 0 (#5073).
			//
			// Only for a letter this dialect spells. One it has not got is
			// refused **by name** further down — `functions -Ma …` is `bad
			// option: -a` in the reference, not `invalid option(s)` — and
			// that is the fall-through this branch deliberately leaves in
			// place. See mathLetterMixed for the alphabet it was measured
			// over and for the two letters that never reach the set at all.
			if refusal := r.diag().MarkingUnderPlusRefusal; refusal != "" &&
				len(operands) == 1 && r.functionsSpellsLetter(operands[0]) {
				r.diagf("%s: %s\n", r.builtinComplaintName(name), refusal)
				return 1
			}
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
			r.fatalUsageQuiet()
		}
		return code
	}
	if written, removed := r.traceLettersWritten(f); (written != "" || removed != "") && matching {
		// A pattern line that *marks* rather than lists: measured on zsh
		// 5.9.2, `functions -tm 'f*'` writes nothing, answers 0 and leaves
		// `f` traced. So the trace letters take the line here too, and the
		// pattern picks the names instead of naming them.
		//
		// Ahead of the listing below rather than inside it, because the two
		// readings of `-m` are a listing and a selection and only one of
		// them writes anything.
		return r.setFunctionTraceMarks(r.functionNamesMatching(args), written, removed)
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
		// Never located: this is the other shell's spelling and that shell
		// has no extended debugging, so LocatesFunctions is off here in
		// every dialect that reaches this builtin. Written out rather than
		// asked, so a dialect adding the capability has to come and look.
		return r.declareFunctions(nil, false, namesOnly, false, false, false)
	}
	status := 0
	for _, pattern := range patterns {
		if r.refusedSelectionPattern(pattern) {
			status = 1
			continue
		}
		o := r.patternOpts(pattern)
		for _, name := range r.scriptFuncNames() {
			if !matchPattern(pattern, name, o) {
				continue
			}
			if namesOnly {
				r.printf("%s\n", r.listedFunctionNameOnly(name, r.funcs[name]))
				continue
			}
			r.printf("%s", r.listedFunctionLine(name, r.funcs[name]))
		}
	}
	return status
}

// functionsIndentLetter reads `functions -x num`, which writes each level of
// the listing's structure as num spaces instead of as the tab the shell adds
// by default. Zero suppresses the indentation altogether.
//
// It is a property of the *listing* and not of the function, so nothing else
// moves for it: what the letter chooses is what one level is written as.
//
// The number is the rest of the letter's own word when anything follows it and
// the next word when nothing does — and the *whole* rest of the word, which is
// what the two refusals below separate. Measured 2026-09-26 on zsh 5.9.2,
// `env -u FPATH` over a script file:
//
//	functions -x 2 g     two spaces a level
//	functions -x2 g      the same, attached
//	functions -mx 2 g    the same: the letter ends its bundle
//	functions -x0 g      no indentation at all
//	functions -x -1 g    the same — below zero is zero, at status 0
//	functions -xm 2 g    `number expected after -x` — `m` is the argument
//	functions -x2m g     `number expected after -x` — and so is `2m`
//	functions -x abc g   `number expected after -x`
//	functions -x         `argument expected: -x` — nothing followed at all
//
// The last two are two different sentences for what looks like one fault, and
// the difference is whether a word was there to be read: a word that is not a
// number is the number's own complaint, and no word at all is the option
// reader's. The `+` sign is taken and means nothing here — `functions +x 2 g`
// writes the body out, where a bare `functions +` names the function instead.
//
// found says whether the letter was written at all, so that a listing with no
// `-x` keeps the layout it has rather than being handed a rebuilt copy of it.
func (r *Runner) functionsIndentLetter(name string, args []string) (rest []string, indent int, found bool, code int) {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" || len(a) < 2 || (a[0] != '-' && a[0] != '+') {
			// The options have ended, so every word left is an operand and
			// no `x` in one of them is this letter.
			return append(out, args[i:]...), indent, found, 0
		}
		j := strings.IndexByte(a, 'x')
		if j < 0 {
			out = append(out, a)
			continue
		}
		number := a[j+1:]
		if number == "" {
			if i+1 >= len(args) {
				return nil, 0, false, r.optionNeedsArgument(name, 'x')
			}
			i++
			number = args[i]
		}
		n, ok := atoiSigned(number)
		if !ok {
			r.complainAboutOption(name, "%s\n", Wording(r.diag().FunctionsIndentNeedsANumber,
				"%[1]s: -%[2]s: numeric argument required", r.builtinComplaintName(name), "x"))
			return nil, 0, false, orDefault(r.diag().BuiltinBadOptionStatus, 2)
		}
		if n < 0 {
			// Measured: below zero is zero rather than a refusal, and the
			// listing comes out flush.
			n = 0
		}
		if head := a[:j]; len(head) > 1 {
			// The letters in front of the `x` are options in their own right
			// and go on to the parser; a word with nothing but a sign left
			// is dropped, since a bare `-` is an option in this dialect and
			// a bare `+` would turn the listing into a list of names.
			out = append(out, head)
		}
		indent, found = n, true
	}
	return out, indent, found, 0
}

// functionNamesMatching is the names these patterns pick out of the function
// table, for a line whose `-m` selects rather than lists.
//
// The same walk functionsMatching writes from, without the writing: one
// pattern at a time over the script's own functions, so a name two patterns
// reach is picked twice — which costs nothing where the caller is setting a
// record rather than printing.
func (r *Runner) functionNamesMatching(patterns []string) []string {
	var out []string
	for _, pattern := range patterns {
		if r.refusedSelectionPattern(pattern) {
			continue
		}
		o := r.patternOpts(pattern)
		for _, name := range r.scriptFuncNames() {
			if matchPattern(pattern, name, o) {
				out = append(out, name)
			}
		}
	}
	return out
}

// functionsSpellsLetter reports whether this dialect's `functions` has the
// letter at all, however it answers it.
//
// **Both tables, and that pairing is the point.** A letter lives in the
// accepted set or in the unimplemented one, and a rule that read only the
// first would call a letter this shell merely has not built yet "a letter
// nobody has" — which is what `-Mt` did before the trace mark was
// implemented, and is exactly how the case that looked like it was checking
// exclusivity came to be graded on the implementation status instead (#5073).
//
// So the question is *does this shell spell it*, not *does this shell do it*.
// A letter that moves from one table to the other must not change the answer
// here, and there is a row for that.
func (r *Runner) functionsSpellsLetter(letter string) bool {
	if letter == "" || strings.Contains(functionsLettersReadFirst, letter) {
		return false
	}
	if strings.Contains(r.sem().FunctionsOptions, letter) {
		return true
	}
	return strings.Contains(r.diag().UnimplementedOptionLetters["functions"], letter)
}

// functionsLettersReadFirst are the `functions` letters that take words of
// their own, and so are read — and can refuse — **before** the letter set is
// judged for exclusivity.
//
// They are not exceptions to the exclusivity rule; they never reach it.
// Measured 2026-09-28 on zsh 5.9.2 over every letter of the alphabet beside
// `-M`, these two are the only ones that answer with anything but `bad
// option`, `invalid option(s)` or silence:
//
//	functions -Mx mf 1 1 g     number expected after -x, 1
//	functions -Mx 3 mf 1 1 g   **0, and mf is registered**
//	functions -Mc mf 1 1 g     -c: requires two arguments, 1
//	functions -M -c a b mf …   the same — its words are not there to take
//
// The second row is the one that settles `x`: given its number the letter
// composes with `-M` perfectly well, so a rule that refused it would have been
// wrong about a spelling that works. `x` is already taken off the words above
// this, before the set is read at all.
//
// `c` copies a function and wants two names — `functions -c a b` is `no such
// function: a` here — and this engine has not built it. Its own refusal is
// what a line naming it still gets, which is narrower and more useful than
// `invalid option(s)`: **the row is left exactly where it was rather than
// moved from one wrong answer to another**, and it is recorded on #5073 as a
// non-agreement with its cause rather than quietly swept into this rule.
const functionsLettersReadFirst = "xc"
