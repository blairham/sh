// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// What a declaration line whose letters were **refused** does with the array
// literals written behind them.
//
// A refused letter stops the builtin: it prints its complaint and its usage
// line and declares none of its operands. In one column that is not the whole
// of it — the array-literal operands are declared and stored anyway, with the
// letters the line *could* read, and the plain `name=value` operands beside
// them are not. Measured 2026-10-03 on bash 5.3.20, `-c`:
//
//	typeset -U a=(1 1 2); declare -p a
//	    -U: invalid option, the usage line, status 2 — and then
//	    declare -a a=([0]="1" [1]="1" [2]="2")
//	declare -U a=(1) b=2 c=(3)
//	    a and c are arrays; b is not found
//	declare -Ui m=(1+1)      declare -ai m=([0]="2")
//	declare -U -A m=([k]=v)  declare -A m=([k]="v" )
//	f(){ typeset -U a=(1); }; f
//	    a is the function's local and gone once it returns, and with a `g`
//	    anywhere on the line — before or after the refused letter — it is
//	    the global
//	export -U a=(1) and readonly -U a=(1)
//	    the global, whatever the call depth: neither word makes locals
//
// zsh 5.9.2 is the other answer: `typeset -Q a=(1)` is `bad option: -Q` at 1
// and `a` does not exist afterwards. ksh93 never arrives, because there the
// refusal ends the script (Semantics.TypesetBadOptionFatal).
//
// So this is a question about a disagreement — Semantics.
// RefusedDeclarationKeepsItsArrayLiterals — and it is asked only where the
// two answers part: a letter was refused *and* the line carries an array
// literal. Every other refused line reaches no question.
//
// How it is done says what the measurement says: the line is read again with
// the refused letters taken out, and declared over its array-literal operands
// alone. The literals themselves are stored by the operand store that runs
// after the builtin, exactly as on a line that was accepted — see
// Runner.refusedLineKeptItsLiterals, which is how the builtin tells that store
// its non-zero status is not a reason to skip it.
//
// The listing and function letters are taken out with the refused ones: they
// would make the second reading a listing or a function operation, which is
// not what is being declared, and no row has measured a refused line carrying
// them.

// literalValueLetters are the letters a refused line still applies to its
// array literals: the ones that decide what the value *is* — its kind, an
// integer's arithmetic, a case fold — and the one that decides where it lands.
// Measured, `declare -Ux m=(1)` and `declare -Ur n=(1)` leave plain arrays
// neither exported nor frozen, where `declare -Ui m=(1+1)` is `-ai` holding 2
// and `declare -Uu m=(ab)` is `-au` holding `AB`.
const literalValueLetters = "aAilucg"

// refusedLineDeclaresItsLiterals declares a refused line's array-literal
// operands with the letters it could read, where the dialect says the line
// keeps them. The status of the line is the refusal's and is not changed.
//
// written is the line's words after the utility's own, and known the letters
// the utility accepts; local says whether the word makes a function's names
// local, which `export` does not.
func (r *Runner) refusedLineDeclaresItsLiterals(written []string, known string, local bool) {
	if len(r.literalOperands) == 0 || r.unspecified || r.ctl == controlExit {
		return
	}
	letters := r.sem().DeclareOptions
	if letters == "" {
		letters = declareOptionLetters
	}
	var opts, names []string
	refused := false
	i := 0
	for ; i < len(written); i++ {
		a := written[i]
		if a == "--" {
			i++
			break
		}
		if len(a) < 2 || (a[0] != '-' && a[0] != '+') {
			break
		}
		kept := a[:1]
		for _, c := range a[1:] {
			if !strings.ContainsRune(known, c) {
				refused = true
			}
			// Read against the declaration's own letters rather than the
			// utility's: `export -Ui q=(1+1)` is `-ai` holding 2, although
			// `export` takes no `i`.
			if strings.ContainsRune(literalValueLetters, c) && strings.ContainsRune(letters, c) {
				kept += string(c)
			}
		}
		if len(kept) > 1 {
			opts = append(opts, kept)
		}
	}
	if !refused {
		// The line failed for some other reason — a number a letter was
		// waiting for, say — and that is not the question asked here.
		return
	}
	for _, a := range written[i:] {
		if r.literalOperands[a] {
			names = append(names, a)
		}
	}
	if len(names) == 0 {
		return
	}
	if !r.ask(r.sem().RefusedDeclarationKeepsItsArrayLiterals,
		"a declaration's array literals outliving a refused letter") {
		return
	}
	if !local {
		opts = append(opts, "-g")
	}
	rest, f, code := r.parseDeclareFlags("declare", append(opts, names...), letters)
	if code != 0 || r.unspecified {
		return
	}
	r.declareNames(r.inBuiltin, rest, f)
	r.refusedLineKeptItsLiterals = true
}
