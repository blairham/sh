// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "context"

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
		tree, perr := r.arithTree(nil, expr)
		if perr != nil {
			r.diagf("%s\n", r.diag().ParseFailure(perr))
			return r.diag().StatusForParseError(perr)
		}
		v, err := r.evalArith(tree)
		if r.unspecified {
			return 2
		}
		if err != nil {
			r.diagf("%v\n", err)
			return 1
		}
		// Every expression is evaluated — they have side effects, and
		// `let x++ y=2` is two of them — but only the last decides.
		last = v
	}
	return boolInt(last == 0)
}
