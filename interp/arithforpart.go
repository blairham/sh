// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// How much of a `for (( init; cond; post ))` header part a shell keeps.
//
// The part's text is quoted back in two places — a trace writes it and a
// complaint blames it — and they are the same string. That is the whole
// reason this is one field and not two: the two were measured apart and each
// grew its own trimming, which is how ksh93's trace came to write `((i=0 ))`
// where the shell writes `(( i=0))` and its complaint came to read ` i=1/0 `
// where the shell writes ` i=1/0`.
//
// Measured 2026-09-13 on ksh93u+ 2012-08-01, bash 5.3.15 and zsh 5.9.2, from
// `for ((  i=0  ;  i<1  ;  i++  ))` under `set -x` and from a part that will
// not evaluate.
//
//	                 init          cond          post
//	bash             `  i=0  `     `  i<1  `     `  i++  `
//	zsh              `  i=0  `     `  i<1  `     `  i++  `
//	ksh93            `  i=0`       `  i<1`       `i++  `
//
// So bash and zsh keep the part entire and each drops the leading blanks
// afterwards where its own spelling says to — `(( ` and ` ))` around the
// trimmed text in bash's trace, Diagnostics.ArithErrorSkipsLeadingSpace in
// its complaint. ksh93 keeps one end of each part and neither of the others
// does anything of the kind: the first two lose their trailing blanks and the
// third loses its leading ones, after which nothing is trimmed again — which
// is exactly why its trace is `((  i=0))` and `((i++  ))` in the same loop.
//
// There is no rule underneath the asymmetry that this panel agrees on, and it
// is recorded as measured rather than derived. The blanks next to a `;` go
// away on its left in the first two parts and on its right in the third; the
// cond part keeps the blanks that follow the `;` in front of it and the post
// part does not, two characters apart in the same header.
//
// **Before the expansions and not after**, which is measurable and is the
// half a reading could get backwards: `x=" 1/0 "; for (( $x ;; ))` is
// `  1/0 : divide by zero` in ksh93 — a trailing blank the value brought,
// standing after the blank the source's own had already been taken off. A
// shell that trimmed the expanded text would write `  1/0` and lose it.
//
// The part text is also what the evaluator reads, and taking blanks off
// either end of an expression changes nothing it can see. So this is one
// trim, at the top of the construct, rather than a rule each consumer
// remembers to apply.

// ArithForPartTextPolicy is how much of a `for (( ))` header part's text a
// dialect keeps once it has read it.
type ArithForPartTextPolicy int

const (
	// ArithForPartTextAsWritten keeps the part entire, blanks and all: bash
	// and zsh, and the substrate's own answer. Each then drops the leading
	// blanks where its own trace spelling and its own complaint say to,
	// which is a separate question and stays one.
	ArithForPartTextAsWritten ArithForPartTextPolicy = iota
	// ArithForPartTextLosesOneEnd is ksh93's: the first two parts lose their
	// trailing blanks and the third loses its leading ones, and nothing is
	// trimmed after that.
	ArithForPartTextLosesOneEnd
)

func (p ArithForPartTextPolicy) String() string {
	if p == ArithForPartTextLosesOneEnd {
		return "loses the blanks at one end"
	}
	return "the part as written"
}

// arithForPartsKept is the three parts as this dialect keeps them.
//
// Taken together rather than one at a time because which end a part loses
// depends on which part it is, and a helper that took one part and a position
// would let a caller ask about the third with the first's answer.
func (d Diagnostics) arithForPartsKept(init, cond, post string) (string, string, string) {
	if d.ArithForPartText != ArithForPartTextLosesOneEnd {
		return init, cond, post
	}
	return strings.TrimRight(init, " \t"),
		strings.TrimRight(cond, " \t"),
		strings.TrimLeft(post, " \t")
}

// arithForPartTraced is the part as its trace writes it.
//
// The leading blanks come off for the dialects that kept the part whole,
// which is what both of them do — bash writes `(( ` and ` ))` around the
// trimmed text and zsh writes the trimmed text bare. The dialect that keeps
// one end has already given up the blanks it gives up, and trimming again
// would take the other end's as well.
func (d Diagnostics) arithForPartTraced(expanded string) string {
	if d.ArithForPartText == ArithForPartTextLosesOneEnd {
		return expanded
	}
	return strings.TrimLeft(expanded, " \t")
}
