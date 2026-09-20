// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"strings"

	"github.com/blairham/sh/syntax"
)

// `let` evaluates arithmetic, and is `(( … ))` with the expressions given as
// words instead of inside the parentheses.
//
// bash, ksh93 and zsh have it; dash does not, so its layer unregisters it —
// the same way ksh93 and dash unregister `builtin`, which each spells its own
// way or not at all.
//
// The status is the surprising part and is unanimous: `let "x=5"` reports 0
// and `let "x=0"` reports 1, because a shell reports *false* for an expression
// that came out zero. It is the last expression that decides, so `let a=0 b=1`
// succeeds.

func init() {
	builtins["let"] = biLet
}

func biLet(r *Runner, _ context.Context, args []string) int {
	if len(args) == 0 {
		d := r.diag()
		msg := Wording(d.LetNoExpression, "let: expression expected")
		if d.LetNoExpressionUnprefixed {
			r.errf("%s\n", msg)
		} else {
			r.diagf("%s\n", msg)
		}
		return orDefault(d.LetNoExpressionStatus, 1)
	}
	if r.sem().LetReadsALeadingZeroAsDecimal == Yes {
		// One shell's `let` is not the same reader as its `(( ))`: `let
		// "x=010"` is ten there and `(( y=010 ))` is eight. Every numeral in
		// the word goes decimal rather than only the leading one — `let
		// "x=1+010"` is 11 — so the octal rule is turned off for the
		// evaluation rather than one numeral being rewritten, which is what
		// the stored-value reader beside it does. See
		// Semantics.LetReadsALeadingZeroAsDecimal.
		//
		// On a copy of the vector, so it lasts exactly as long as the
		// builtin: a Runner is shared and the arithmetic after this one is
		// read the ordinary way.
		//
		// Read rather than asked, and the reason is that the question is
		// still put where it can be seen: leaving this alone gives `let` the
		// same reader `(( ))` has, which is what three of the four do, and
		// the leading zero itself still meets ArithLeadingZeroIsOctal at the
		// literal. Asking here would put a dialect question to every `let`
		// in a run with no dialect, including the ones with no zero in them.
		defer func(was *Semantics) { r.Semantics = was }(r.Semantics)
		r.swapSemantics(func(s *Semantics) { s.ArithLeadingZeroIsOctal = No })
	}
	// The truth of the last expression rather than its integer value: a
	// value between zero and one truncates to zero and is still true, which
	// is what `let 0.5` measures at 0 in ksh93 and zsh. See evalArithTruth.
	last := false
	for _, expr := range args {
		r.unspecified = false
		tree, text, perr := r.arithTreeOver(nil, expr)
		if perr != nil {
			r.mathDiagf("%s", r.diag().ParseFailure(perr))
			if v, ok := r.valueBeforeAnIllegalByte(perr, text); ok {
				// The reader stopped at a byte it could not read and what it
				// had by then stands. It stops *here* as well as there: the
				// expressions after this one are not evaluated, which is what
				// `let '0 @' '3'` measures — 1, from the zero, rather than 0
				// from the 3.
				return boolInt(v == 0)
			}
			return r.diag().StatusForParseError(perr)
		}
		v, err := r.evalArithTruth(tree)
		if r.unspecified {
			return 2
		}
		if errors.Is(err, errReadonlyRefusedInACommand) {
			return 1
		}
		if err != nil {
			if r.arithNounsetNamedTheParameter {
				// Not this builtin's failure where `set -u` calls the
				// refusal the shell's own: measured 2026-09-18, bash 5.3.20
				// writes a bare `b: unbound variable` for `set -u; let
				// "x=b"` and names itself — `let: x=1+: …` — for the same
				// builtin failing to read an expression. ksh93u+ keeps its
				// `let:` here and is the column that says this belongs to
				// the fatality and not to the refusal: there the refusal is
				// the expression's failure, the builtin reports it, and the
				// script runs on at 1 (#3574).
				r.diagf("%s\n", r.arithFailure(text, err))
				// The status is the shell's own — 1 from a script file and
				// 127 from a `-c` string in bash — and not the 1 a failed
				// `let` would leave, which is the same number on one route
				// and not on the other.
				return r.status
			}
			// Named with its expression, the way the expansion route names
			// one: `let: 1/0: division by 0` and not `let: division by 0`
			// (#1985). The text is the expanded one the tree was built from,
			// which is what the shells quote back.
			r.mathDiagf("%s", r.arithFailure(text, err))
			return 1
		}
		// Every expression is evaluated — they have side effects, and
		// `let x++ y=2` is two of them — but only the last decides.
		last = v
	}
	return boolInt(!last)
}

// mathDiagf writes a complaint about the expression, named the way this
// dialect names one raised from a builtin.
//
// Two answers, measured 2026-09-11 on `let '1+'`: bash 5.3 and ksh93 put the
// builtin in front of the sentence — `bash: line 1: let: 1+: …` and `ksh: let:
// 1+: …` — and zsh puts it nowhere at all, writing `zsh:1: bad math
// expression: …` where the same shell's `cd` complaint is `zsh:cd:1: …`.
//
// So the name is written here and the location rule strips it back out where
// the dialect carries it in the location instead — one door rather than two,
// which is how every other builtin's complaint already works.
func (r *Runner) mathDiagf(format string, args ...any) {
	r.mathWrite(r.diagf, format, args...)
}

// mathFatalf is mathDiagf for a site that ends the script with the complaint.
//
// The naming rule is the message's and not the builtin's, so the two writers
// share it rather than each spelling it out: a declaration whose *value* will
// not evaluate is the same sentence `let` writes about the same text, and
// every column words it the same way it words `let`'s. Measured 2026-09-18
// over `typeset -i a=1+` in a script file:
//
//	bash 5.3.20	loc.sh: line 1: typeset: 1+: arithmetic syntax error: …
//	ksh93u+    	loc.sh[1]: typeset: 1+: more tokens expected
//	zsh 5.9.2  	loc.sh:1: bad math expression: operand expected …
//
// and `integer a=1+`, `float a=1+`, `local -i a=1+` and `a=1+; integer a`
// are the same three answers in zsh, which is what says the rule is the
// message's rather than any one spelling's (#3342).
func (r *Runner) mathFatalf(format string, args ...any) {
	r.mathWrite(r.fatal, format, args...)
}

// mathWrite is the naming rule itself, applied to whichever writer the site
// uses. See mathDiagf for the two answers.
func (r *Runner) mathWrite(write func(string, ...any), format string, args ...any) {
	// Only where a builtin is speaking. The same evaluator is reached from a
	// plain assignment to a name carrying the integer attribute — `typeset -i
	// a=1; a+=2+` — and bash writes no name in front of that one, because
	// there is no builtin to name.
	if name := r.speaking(); name != "" && r.diag().ArithErrorNamesTheBuiltin {
		write("%s: "+format+"\n", append([]any{name}, args...)...)
		return
	}
	defer r.builtinAsideForAMathFailure()()
	write(format+"\n", args...)
}

// mathReportf is mathDiagf for a math complaint the *expression carries on
// from*, so the construct that raised it never sees it and cannot word it.
//
// There is one such sentence — the empty subscript of a write, which the
// reporting column reports and then drops — and it was the only math failure
// in the tree arriving with no name on it. The naming rule is not missing, it
// was simply not applied here: `(( 1/0 ))` and `let "1/0"` are byte-identical
// to bash because they travel back up as errors and are named at the
// construct's own site, while this one is written where it is raised.
//
// Measured 2026-09-20, bash 5.3.20 over a script file with `m=(1 2 3)` above:
//
//	(( m[] = 4 ))              ((: `m[]': not a valid identifier
//	let "m[] = 4"              let: `m[]': not a valid identifier
//	for (( m[] = 4; 0; ))      ((: `m[]': not a valid identifier
//	typeset -i q='m[]=4'       typeset: `m[]': not a valid identifier
//	x=$(( m[] = 4 ))           `m[]': not a valid identifier
//	declare -i y; y='m[]=4'    `m[]': not a valid identifier
//	(( z = $(( m[] = 4 )) ))   `m[]': not a valid identifier
//
// The last three are the controls and they are what fixes the shape of this:
// the expansion route names nothing, a plain assignment through the integer
// attribute names nothing because no builtin is speaking, and a `$(( ))`
// *inside* a `(( ))` names nothing because it is expanded before the
// construct starts evaluating. zsh names neither a construct nor a builtin
// anywhere, which the two existing flags already say (#3901).
//
// The construct wins over the builtin where both could be named, which is
// what `eval '(( m[] = 4 ))'` measures: `((: `m[]'…`, not `eval: `.
func (r *Runner) mathReportf(format string, args ...any) {
	if r.arithConstruct != "" && r.diag().ArithErrorNamesTheConstruct {
		defer r.builtinAsideForAMathFailure()()
		r.diagf("%s: "+format+"\n", append([]any{r.arithConstruct}, args...)...)
		return
	}
	r.mathDiagf(format, args...)
}

// builtinAsideForAMathFailure takes the builtin out of the *location* where
// the dialect does not read a math failure as the builtin's, and returns what
// puts it back.
//
// Cleared rather than trimmed afterwards, because the location is built from
// this field. One door for the two writers below it, so a site cannot take
// the name out of the sentence and leave it in the location.
func (r *Runner) builtinAsideForAMathFailure() func() {
	if r.diag().ArithErrorNamesTheBuiltin {
		return func() {}
	}
	outer := r.inBuiltin
	r.inBuiltin = ""
	return func() { r.inBuiltin = outer }
}

// arithDiagf writes an arithmetic complaint raised from inside a builtin with
// the builtin taken out of the *location*, where the dialect does not read
// the failure as the builtin's.
//
// The same answer ArithErrorNamesTheBuiltin gives for the front of the
// sentence, applied to the other place a name can appear: zsh 5.9.2 writes
// `zsh:printf:1: no such option` for `printf`'s own refusal and `zsh:1: bad
// math expression: operator expected at `x'` for `printf '%d' 12x`, which is
// the identical line `let '12x'` and `echo $((12x))` write there. Measured
// 2026-09-15 on the `-c` and file routes (#2906); on the file route both are
// the script's name, `/tmp/zz.sh:1:`, so it is the builtin segment that goes
// and not the location.
//
// Cleared rather than trimmed afterwards, because the location is built from
// this field.
func (r *Runner) arithDiagf(format string, args ...any) {
	defer r.builtinAsideForAMathFailure()()
	r.diagf(format, args...)
}

// valueBeforeAnIllegalByte is what an expression comes to when the reader
// stopped at a byte it could not read, for the dialect that keeps it.
//
// Measured 2026-09-11, zsh 5.9.2: `let '1 @'` reports the illegal character
// and exits **0**, `let '0 @'` exits 1, `let '1+2 @'` exits 0 and `let '@'`
// exits 1. So the value is what stood before the byte, and `let`'s ordinary
// rule — false for an expression that came out zero — then decides. bash 5.3,
// bash 3.2 and ksh93 are 1 for all four.
//
// Only the illegal byte, which is the discriminating half: `let '1+'` and
// `let '5 5'` are 1 in that shell too, though a value stood before those
// failures as well. It is a fact about the reader giving up mid-stream rather
// than about arithmetic failure in general.
//
// Only `let`, because nowhere else can it be seen: `$(( 1 @ ))` and `(( 1 @ ))`
// abandon the line and report in that shell whatever value stood.
func (r *Runner) valueBeforeAnIllegalByte(perr error, text string) (int, bool) {
	var se *syntax.Error
	if !errors.As(perr, &se) || se.Kind != syntax.ErrArithIllegalByte {
		return 0, false
	}
	if !r.ask(r.sem().LetKeepsTheValueBeforeAnIllegalByte,
		"the value `let` is left with when its expression has a byte the reader refuses") {
		return 0, false
	}
	// The byte's *first* occurrence is where the reader stopped, which follows
	// from what the kind means rather than being a guess: this failure is
	// raised only for a byte the dialect's arithmetic reader refuses as part
	// of no token at all, so it cannot have appeared inside anything already
	// read. The error carries the position of the expression rather than of
	// the byte, and widening it to carry both would move every other
	// arithmetic diagnostic's position with it.
	before := strings.Index(text, se.Token)
	if before <= 0 {
		// Nothing stood before it, so nothing is what it comes to — which is
		// `let '@'`, and is 1.
		return 0, true
	}
	p := syntax.NewParser("", r.dialect())
	tree := p.ParseArithFor(text[:before], syntax.Pos{})
	if p.Err() != nil || tree == nil {
		return 0, true
	}
	v, err := r.evalArith(tree)
	if err != nil {
		return 0, true
	}
	return v, true
}
