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
	last := 0
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
		v, err := r.evalArith(tree)
		if r.unspecified {
			return 2
		}
		if err != nil {
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
	return boolInt(last == 0)
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
	if !r.diag().ArithErrorNamesTheBuiltin {
		// The shell's own failure rather than the builtin's, which is what
		// the dialect that names builtins in the location is saying by not
		// naming one here. Cleared rather than trimmed afterwards, because
		// the location is built from this field.
		outer := r.inBuiltin
		r.inBuiltin = ""
		defer func() { r.inBuiltin = outer }()
		r.diagf(format+"\n", args...)
		return
	}
	r.diagf("%s: "+format+"\n", append([]any{r.speaking()}, args...)...)
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
