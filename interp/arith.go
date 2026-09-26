// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/blairham/sh/syntax"
)

// arithError is a failure inside an expression, such as dividing by zero.
// It is a runtime error rather than a syntax one: the expression parsed.
//
// token is the part of the expression the failure is attributed to, which
// bash prints and the other three do not. Empty means "the whole
// expression", which is both a sensible default and what bash itself says
// when the failing literal is the entire thing: `$((08))` names `08`.
type arithError struct {
	msg   string
	token string
	// blameTail says the blamed text runs from where the failing operand was
	// written to the *end* of the expression, rather than being the operand
	// alone. Measured 2026-09-12 on bash 5.3.15, which parts the two by where
	// the failure was raised:
	//
	//	$(( 1/0 + 2 ))   1/0 + 2 : division by 0 (error token is "0 + 2 ")
	//	$(( 8#9 + 1 ))   8#9: value too great for base (error token is "8#9")
	//
	// An *evaluation* failure names the tail; a literal the number reader
	// refused names the literal, and the expression stops there. So it is a
	// property of the failure and not of the dialect — the other five columns
	// quote no token at all, and the one that does quotes both shapes.
	blameTail bool
	// from is where that tail begins, as a byte offset into the expression's
	// own text, and -1 where only the token's text is known. The tree cannot
	// always answer it: `1/(0)` is blamed on `(0) ` and the parenthesised
	// divisor has no text of its own, so the offset comes from the parser —
	// see [syntax.ArithBinary].YStart.
	from int
	// badNumeral says the failure was a *numeral the number reader refused*
	// — `42abc`, `0b11`, `1#` — rather than anything about the expression
	// around it. One column's `printf` parts the two: ksh93 writes a second
	// line, `printf: warning: invalid argument of type d`, and reports 1 for
	// an operand whose reading failed, and writes its complaint and reports
	// 0 for a division by zero or an assignment that wanted an lvalue. See
	// Diagnostics.PrintfArithArgumentType.
	badNumeral bool
	// keptTheValue says the evaluation went on past the failure and reached
	// the end, so the number beside this error is the whole expression's and
	// not a fragment. See Semantics.ArithDivisionByZeroYieldsAValue.
	keptTheValue bool
	// dividedByZero says the failure was an integer division whose divisor
	// was zero, which one column goes on evaluating past. See
	// Semantics.ArithDivisionByZeroYieldsAValue.
	dividedByZero bool
	// digits is the numeral's digit run with any radix prefix taken off, and
	// numBase the base they were read in. Kept only for a numeral past the
	// word, where the reading that answers it has to run over the digits
	// again: two of the three re-read them, and one of those counts how far
	// it got. See Semantics.ArithNumeralPastTheWord.
	digits  string
	numBase int
	// pastTheWord says the numeral is well formed and larger than the machine
	// word holds. Every reference shell answers *something* for one and the
	// six answers are six different readings; the only one this shell has is
	// ksh93's, where the numeral simply becomes the double its arithmetic is
	// carried in. See Semantics.ArithValuesAreCarriedInAFloat, and #3202
	// for the other columns.
	pastTheWord bool
	// complete says the message is the whole diagnostic and must not be
	// wrapped. Measured: dash wraps a division by zero — `arithmetic
	// expression: division by zero: "1/0"` — but reports a non-numeric
	// operand bare, as `Illegal number: abc`. One wrapper for every
	// arithmetic failure would put a shape on the second that dash does not
	// use, so the failure says which it is.
	complete bool
}

func (e arithError) Error() string { return e.msg }

// arithFailure is the sentence a dialect writes about an expression that
// would not evaluate, given the text it was written as and what went wrong.
//
// One function rather than one per caller, because an expression is written
// in more places than `$(( ))` and every one of them is reported the same way
// by every shell measured. A subscript is the case that proved it: `${a[b c]}`
// is the identical complaint to `$((b c))` in all four, and reached its own
// code path, which said nothing at all.
func (r *Runner) arithFailure(text string, err error) string {
	// The expression as written is what dash and ksh93 quote back, and the
	// caller still has it: the parser keeps the raw text beside the tree it
	// built from it.
	ae, _ := err.(arithError)
	token := ae.token
	expr := r.diag().arithBlamedText(text)
	if ae.blameTail {
		if at := ae.tailStart(text, token); at >= 0 {
			token = text[at:]
		}
	}
	if token == "" {
		// bash blames the whole expression when the failing part is the whole
		// expression, which is also the honest answer when the tree cannot
		// name a smaller piece.
		token = expr
	}
	if ae.complete {
		return err.Error()
	}
	if r.diag().ArithErrorNamesThePrefix && token != "" {
		// One dialect's leading position is what it had consumed when the
		// token failed: `08+1` is blamed as `08` and `1+08` as `1+08`.
		if i := strings.Index(expr, token); i >= 0 {
			expr = expr[:i+len(token)]
		}
	}
	return Wording(r.diag().ArithError, "%[2]s", expr, err.Error(), token)
}

// tailStart is where the blamed tail begins in text, or -1 when nothing in
// the failure locates it.
//
// The recorded offset is preferred over a search because a search cannot tell
// two identical operands apart: `$(( 0/0 ))` is blamed on the *divisor*, and
// the first `0` in the text is the dividend.
func (e arithError) tailStart(text, token string) int {
	if e.from >= 0 && e.from <= len(text) {
		return e.from
	}
	if token == "" {
		return -1
	}
	return strings.Index(text, token)
}

// arithToken names the part of an expression a failure should be blamed on.
//
// Only a literal and a bare name can be named this way. Anything else — a
// parenthesised sub-expression, say — has no text of its own in the tree,
// because the parser records what an operand *is* rather than the characters
// it was written with. Those return empty and are reported against the whole
// expression, which is the honest answer rather than a reconstructed one.
func arithToken(e syntax.ArithExpr) string {
	switch x := e.(type) {
	case *syntax.ArithNum:
		return x.Text
	case *syntax.ArithVar:
		return x.Name
	case *syntax.ArithUnary:
		// A signed operand is blamed on its leaf: `2**-1` names the `1`,
		// which is measured — the shell that names error tokens reads the
		// sign as part of the expression and stops on the literal.
		if !x.Postfix {
			return arithToken(x.X)
		}
	}
	return ""
}

// evalArith evaluates an expression tree.
//
// Evaluation order is part of the specification rather than an implementation
// detail, because assignment is an operator whose effect outlives the
// expression: `x=0; $((0 && (x=9)))` leaves x alone in five of the panel's
// seven columns and sets it to 9 in BusyBox ash. So the logical operators
// short-circuit here, nothing evaluates both sides eagerly, and the one shell
// that does says so on an axis — see evalDecidedOperand.
// arithNum is a value in an arithmetic expression.
//
// Two shells in the panel do floating point and two do not, and an expression
// in the two that do is integer until a float enters it: `3/2` is 1 there as
// well, and `3.0/2` is 1.5. So a value carries which it is rather than being
// promoted everywhere, and the promotion happens per operation.
type arithNum struct {
	i     int
	f     float64
	float bool
	// wide says the value is an *integer* one the machine word cannot hold,
	// so it is carried in f with the floats. Only the shell whose arithmetic
	// is a C double throughout can produce one — see
	// Semantics.ArithValuesAreCarriedInAFloat — and the distinction is not
	// cosmetic: `$(( 2**64 / 3 ))` there is an integer division of the
	// saturated value, 3074457345618258432, and not 6.14891469123652e+18.
	// So the *kind* is integer while the *representation* is the double,
	// which is exactly the pair ksh93 keeps.
	wide bool
}

func intNum(i int) arithNum       { return arithNum{i: i} }
func floatNum(f float64) arithNum { return arithNum{f: f, float: true} }

// wideNum is an integer value past the word, carried in the double.
func wideNum(f float64) arithNum { return arithNum{f: f, float: true, wide: true} }

// floatKind reports whether the value is a float as far as an *operator* is
// concerned, which a wide integer is not.
func (n arithNum) floatKind() bool { return n.float && !n.wide }

// asFloat is the value as a float, whichever it is.
func (n arithNum) asFloat() float64 {
	if n.float {
		return n.f
	}
	return float64(n.i)
}

// asInt truncates, which is what an integer context does with a float: an
// array subscript, the truth of `(( ))`, or a shell that has no floats at all.
func (n arithNum) asInt() int {
	if n.float {
		i, _ := intFromDouble(n.f)
		return i
	}
	return n.i
}

func (n arithNum) isZero() bool {
	if n.float {
		return n.f == 0
	}
	return n.i == 0
}

// evalArith is the integer answer, for the callers that can only use one: an
// array subscript, the truth test of `(( ))`, the integer attribute.
func (r *Runner) evalArith(e syntax.ArithExpr) (int, error) {
	v, err := r.evalNum(e)
	return v.asInt(), err
}

// evalArithTruth is the truth of an expression, for the two commands whose
// exit status is that truth: `(( ))` and `let`.
//
// It is not evalArith != 0. An integer context *truncates*, and a value
// between zero and one truncates to zero — so `(( 0.5 ))` came out false here
// where the two panel columns that have floats at all both call it true.
// Measured 2026-09-16: `(( 0.5 ))`, `(( -0.5 ))` and `let 0.5` are status 0 in
// ksh93u+ 2012-08-01 and in zsh 5.9.2, `(( 0.0 ))` is 1 in both, and bash
// 5.3.20, bash-as-sh, bash 3.2, dash 0.5.12 and BusyBox ash 1.37.0 have no
// float to ask. `(( 1.5 ))` was already true here, which is why nothing had
// noticed: truncation and the truth agree for every value of one or more, and
// part company only under it.
//
// The operators inside an expression were already right — `$(( !0.5 ))` is 0
// and `$(( 0.5 ? 7 : 9 ))` is 7 — because those ask isZero. Only the two
// commands' status went through the integer.
func (r *Runner) evalArithTruth(e syntax.ArithExpr) (bool, error) {
	v, err := r.evalNum(e)
	return !v.isZero(), err
}

// evalNum is the value of an expression, and the point where the shell's
// record of the *last* arithmetic value is kept up to date.
//
// The record is written here, at every node, rather than at the outermost one
// only, and that is what makes the reading right: a math function's value is
// the last arithmetic evaluated anywhere during its call, and the arguments of
// the call are part of that — which is why an implementation that evaluates
// nothing at all hands back its last argument. See mathfunc.go for the
// measurement.
func (r *Runner) evalNum(e syntax.ArithExpr) (arithNum, error) {
	v, err := r.evalNumNode(e)
	if err == nil {
		r.lastArith = v
	}
	return v, err
}

func (r *Runner) evalNumNode(e syntax.ArithExpr) (arithNum, error) {
	switch x := e.(type) {
	case nil:
		return intNum(0), nil

	case *syntax.ArithNum:
		return r.parseArithNum(x.Text, x.Tail)

	case *syntax.ArithVar:
		return r.arithValueOf(x.Name)

	case *syntax.ArithIndex:
		return r.arithElement(x)

	case *syntax.ArithCharCode:
		return intNum(r.charCode(x)), nil

	case *syntax.ArithOutput:
		// The value is the operand's, unchanged: the specifier decides how the
		// answer is written and never what it is. See arithoutput.go.
		return r.evalArithOutput(x)

	case *syntax.ArithCall:
		// The seam: an expression that runs a shell function. See
		// mathfunc.go, which is where everything about it lives.
		return r.evalMathFunc(x)

	case *syntax.ArithUnary:
		return r.evalUnary(x)

	case *syntax.ArithCond:
		c, err := r.evalNum(x.Cond)
		if err != nil {
			return intNum(0), err
		}
		if !c.isZero() {
			return r.evalNum(x.Then)
		}
		return r.evalNum(x.Else)

	case *syntax.ArithAssign:
		return r.evalAssign(x)

	case *syntax.ArithBinary:
		return r.evalBinary(x)
	}
	return intNum(0), arithError{msg: fmt.Sprintf("unsupported expression %T", e)}
}

// arithElement reads `a[i]` written inside an expression.
//
// The subscript is an expression, so it is evaluated first; the base it counts
// from is the dialect's, the same one `${a[1]}` uses. An element that is not
// there is zero rather than an error, which is what every shell in the panel
// does with a subscript past the end and with a name that was never an array.
func (r *Runner) arithElement(x *syntax.ArithIndex) (arithNum, error) {
	// The name first, where the dialect looks at the name first: one shell
	// answers zero for a name that is not there without reading the brackets
	// at all, so `$(( nodecl[1/0] ))` divides nothing and `$(( nodecl[i++] ))`
	// steps nothing. Ahead of the associative test because a name that is not
	// there is not an associative array either, and ahead of the empty-
	// subscript answer because that is the row this decides: an empty
	// subscript on a name nothing declared is the plain unset operand
	// `$(( nosuchvar ))` is (#1745).
	if r.sem().ArithSubscriptSkippedWhenNameUnset == Yes && !r.arithNameIsSet(x.Name) {
		return intNum(0), nil
	}
	if x.Empty {
		if handled, v, err := r.emptyArithSubscript(x.Name); handled {
			return v, err
		}
		// Not handled: the dialect reads the brackets as the empty
		// *expression*, so the ordinary read below answers it — a subscript
		// of zero on an indexed name and the empty key on an associative one,
		// which is exactly what a nil Index and an empty Sub already mean to
		// the two paths that follow.
	}
	if x.Flags != nil {
		// A flag group decides how the subscript is *read*, so it is asked
		// before every reading below — before the whole-array spelling,
		// which `(r)*` is not, and before the association, whose key would
		// otherwise be the group's own letters.
		if v, handled := r.arithFlaggedElement(x); handled {
			return v, nil
		}
		// Not handled: the group selects nothing — `$(( a[(e)2] ))` is the
		// second element — so the operand behind it is an ordinary subscript
		// and every reading below applies to that instead.
		x = arithIndexOfTheOperand(x, r.joinWord(x.Flags.Arg))
	}
	// A quotation the subscript opened and never closed, which one column
	// calls a bad subscript and two read as a key. Ahead of the association
	// below because that is the reading it refuses: the key is what the
	// giving-up scan produced, and the column that refuses never gets there.
	if r.reportArithSubscriptUnclosedQuote(x) {
		// Named and answered zero, and *not* an error: measured, the column
		// that refuses leaves the element as it was and lets the expression
		// finish at status 0, so `let "x = a[$k] + 1"` is 1 there.
		return intNum(0), nil
	}
	// A `*` or `@` is the whole array rather than a subscript at all where
	// the dialect reads the slice here, and it is asked *before* the
	// association below: the key `*` is what the other answer makes of it,
	// and the two would collapse into one if the table were consulted first.
	if v, whole := r.arithWholeArraySlice(x); whole {
		return r.arithElemValue(arithIndexWritten(x), v)
	}
	// An associative name's subscript is a key and not an expression, which
	// is the same reading `${m[k]}` takes and for the same reason: with
	// `m[k]=7`, `m[0]=99` and `k=0`, all three shells with the attribute
	// answer `$(( m[k] ))` with 7. Evaluating it instead read the wrong
	// element and said nothing, which is the silent half of a wrong answer.
	if a, ok := r.assocFor(x.Name); ok {
		return r.arithElemValue(arithIndexWritten(x), a[r.arithAssocKey(r.arithSubscriptRead(x.SubMarked, subscriptAsKey))].scalar())
	}
	if r.reportArithWholeArraySubscript(x) {
		// Named and answered: the operand is zero and the expression keeps
		// going, which is the whole difference from the refusal below.
		return intNum(0), nil
	}
	idx, err := r.arithSubscriptIndex(x)
	if err != nil {
		return intNum(0), err
	}
	// No check that the name is an array: an unset one yields nothing, and
	// nothing is out of range for every subscript, so the bounds test inside
	// elemAt already answers it. A guard here would be a line no test could
	// tell from its absence.
	//
	// A negative subscript counts back from the end here too — `$((a[-1]))`
	// is the last element in all three shells with arrays — which elemAt
	// answers the same way for `${a[-1]}`, so the two spellings cannot drift.
	elems, _ := r.arrayElems(x.Name)
	v, ok := r.elemAt(x.Name, elems, idx.asInt())
	if !ok {
		return intNum(0), nil
	}
	return r.arithElemValue(arithIndexWritten(x), v)
}

// arithFlaggedElement is a subscript that opened with a flag group, read
// inside an expression.
//
// The same machinery the expansion route uses, because it is the same
// construct: `$(( a[(r)20] ))` selects what `${a[(r)20]}` selects, and the
// answer is then read as a number the way every other element's value is.
// Measured on zsh 5.9.2, the one shell with the construct, 2026-09-12:
//
//	a=(10 20 30); $(( a[(r)20] ))       20   the value the search found
//	a=(10 20 30); $(( a[(i)20] ))       2    the index it found it at
//	a=(10 20 30); $(( a[(i)99] ))       4    and the miss, one past the end
//	a=(10 20 30); $(( a[(r)99] ))       0    whose value is nothing, so zero
//	a=(10 20 30); $(( a[(e)2] ))        20   no selecting letter, so a subscript
//	a=(10 20 30); $(( a[(r)20] + 1 ))   21   an operand like any other
//	typeset -A m; m[k]=9; $(( m[(k)k] ))  9  and the same over a table
//	s=hello; $(( s[(r)l] ))             0    a character is no number
//
// handled is false for a group that selects nothing, which is the read
// side's own rule: the operand behind it is then the subscript, and the
// caller reads it as one.
//
// A refusal — a letter this does not carry, a search over a table on the
// write side — has already been reported by name, and the operand is zero.
// The expansion is marked failed, which is what abandons the word: an
// arithmetic answer of zero and no complaint would be the silent wrong
// answer this construct is worth having a reading for (#1986).
func (r *Runner) arithFlaggedElement(x *syntax.ArithIndex) (arithNum, bool) {
	e := &syntax.ParamExpr{Name: x.Name, Index: x.Flags.Arg, IndexFlags: x.Flags}
	// The letters are read here rather than off flaggedSubscript's second
	// return value, because that one folds two answers into one: a group with
	// no selecting letter and a group carrying a letter this does not have
	// both come back unhandled, and only the first of them is a subscript the
	// caller should go on to read as arithmetic. Reading the second as one
	// swallowed the refusal and answered a plausible element.
	search, ok := r.subscriptSearch(e)
	if !ok {
		// Refused by name already, and the expansion is marked failed.
		return intNum(0), true
	}
	if search == 0 {
		return intNum(0), false
	}
	v, _ := r.flaggedSubscript(e)
	n, err := r.arithElemValue(arithIndexWritten(x), strings.Join(v, r.ifsFirst(r.ifs())))
	if err != nil {
		// Worded where every other subscript failure is worded, so the join
		// of several matches complains as the text it is.
		r.diagf("%s\n", r.arithFailure(x.Sub, err))
		r.expandErr = true
		return intNum(0), true
	}
	return n, true
}

// arithIndexOfTheOperand is the node a group that selects nothing leaves
// behind: the same name with the operand behind the group as its subscript,
// and no group.
func arithIndexOfTheOperand(x *syntax.ArithIndex, sub string) *syntax.ArithIndex {
	// The operand behind a group is a *word* the caller has already joined,
	// so it carries no marks and the two readings of the subscript are the
	// one text.
	return &syntax.ArithIndex{
		Name: x.Name, Sub: sub, SubMarked: sub, Start: x.Start, Stop: x.Stop,
	}
}

// arithWholeArraySlice is `$(( a[*] ))` and `$(( a[@] ))` where the dialect
// reads the brackets as the slice `${a[*]}` takes rather than as a subscript.
//
// The joined text is then read the way every other element's value is read —
// as an expression, not as a numeral — which is measured and is what makes
// the two spellings of the same array agree: `a=(1+1); $(( a[*] * 3 ))` is 6
// in the shell that answers yes, exactly as `$(( a[1] * 3 ))` is.
//
// The join is the first character of IFS, `@` and `*` alike: measured
// 2026-09-11 on zsh 5.9.2, `a=(3 4); IFS=:; $(( a[*] ))` and `$(( a[@] ))`
// both complain about the `:` they were handed, and `IFS=` makes the same
// array 34. So this is not the unquoted `@` question, where the two spellings
// part — there is no field splitting inside an expression for them to part
// over.
//
// The re-read is the element route's, so it inherits that route's own gap:
// a value holding an expression rather than a numeral is refused here where
// every column reads it, which is #1977 and reaches `$(( v * 3 ))` on a plain
// name as squarely as it reaches this.
//
// A slice of more than one element is therefore usually a *failure* rather
// than a number, and that is the answer rather than a defect in it:
// `a=(3 4 5); $(( a[*] ))` is `operator expected at ` + "`4 5'" + ` there.
// An empty array joins to nothing and is zero, and a scalar is its own value.
func (r *Runner) arithWholeArraySlice(x *syntax.ArithIndex) (string, bool) {
	if !arithWholeArraySubscript(x) {
		return "", false
	}
	if !r.ask(r.sem().ArithWholeArraySubscriptIsTheSlice,
		"`$(( a[*] ))`, a whole-array subscript inside an expression") {
		return "", false
	}
	return strings.Join(r.wholeArrayElems(x.Name), r.ifsFirst(r.ifs())), true
}

// arithWholeArraySubscript reports whether the brackets hold the whole-array
// spelling and nothing else.
//
// Untrimmed, which is measured rather than tidy: `$(( a[ * ] ))` is an
// arithmetic syntax error in bash and in zsh alike — `operand expected at
// `* '` there — so a `*` with a blank beside it is an expression that will
// not read and not the spelling. Trimming answered a different question, and
// answered it wrongly for both columns.
func arithWholeArraySubscript(x *syntax.ArithIndex) bool {
	return x.Index == nil && !x.Empty && wholeArraySubscript(x.Sub)
}

// reportArithWholeArraySubscript is the whole-array spelling on an indexed
// name where the dialect neither reads it as the slice nor lets the
// arithmetic refuse it: the subscript is named, the operand is zero, and the
// expression carries on.
//
// The same shape emptyArithSubscript's reported answer takes, and for the
// same reason — the report is written here rather than returned as an error,
// because an error is what abandons the expression and this answer does not.
// See Semantics.ArithWholeArraySubscriptIsReportedAsBad.
func (r *Runner) reportArithWholeArraySubscript(x *syntax.ArithIndex) bool {
	if !arithWholeArraySubscript(x) {
		return false
	}
	if !r.ask(r.sem().ArithWholeArraySubscriptIsReportedAsBad,
		"`$(( a[*] ))` on an indexed name, which one column reports and answers zero for") {
		return false
	}
	r.errf("%s\n", r.diag().Report(r.name(), r.line,
		Wording(r.diag().ArithWholeArraySubscript,
			"%[1]s[%[2]s]: bad array subscript", x.Name, x.Sub)))
	return true
}

// arithSubscriptQuotationRefused is a subscript whose brackets were found
// only after the scan gave up on a quotation that never closed — the shape
// `let "++a[$k]"` takes once a `$k` holding an apostrophe has gone in — and
// the column that calls it a bad subscript rather than reading the three
// characters as the key they look like.
//
// The panel parts at its defaults, which is what makes this an axis rather
// than only the option bash spells `assoc_expand_once`. Measured 2026-09-20,
// `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> p.sh` over a script file with
// standard input on the null device, against `typeset -A a; k="q'r";
// a[$k]=4`:
//
//	                         bash 5.3.20            ksh93u+  zsh 5.9.2
//	let "++a[$k]"            refused, element 4     5        5
//	let "a[$k] += 1"         refused, element 4     5        5
//	(( a[$k]++ ))            5                      5        5
//	$(( a[$k] + 100 ))       104, element 4         104      104
//
// Rows three and four are the controls and they are what locate the surface:
// the same key reached by arithmetic's *own* expansion is unanimous, because
// there the value's apostrophe carries a mark and opens no quotation at all.
// So the question is only ever put to text that arrived already
// word-expanded — a `let` operand, where the shell's word expander has taken
// the quoting off and the apostrophe reaches the reader as a byte. A script
// cannot write the shape directly either: `(( a[q'r]++ ))` is an
// unterminated word and never reaches a subscript scan in any column.
//
// Asked only where a quotation really went unclosed, so the ordinary
// `$(( a[$i] ))` needs no answer from anyone, and asked *before* Runner
// .ExpandsAnOperandsSubscriptAgain in the order Runner.operandSubscriptText
// keeps: a dialect that reads the key is never asked about an option it has
// no name for, and a session that turned the round off does not record an
// answer to an axis it then ignores.
//
// See Semantics.ArithSubscriptQuotationMustClose.
func (r *Runner) arithSubscriptQuotationRefused(x *syntax.ArithIndex) bool {
	if !x.UnclosedQuote {
		return false
	}
	if !r.ask(r.sem().ArithSubscriptQuotationMustClose,
		"an arithmetic subscript whose quotation never closes, which one column calls a bad subscript") {
		return false
	}
	// The session asked for the one round it already had, which is the
	// answer the two other columns give by default.
	return r.ExpandsAnOperandsSubscriptAgain()
}

// reportArithSubscriptUnclosedQuote is the refusal above written out, which
// the *read* does and the store does not.
//
// Twice, and the count is measured rather than a loop written twice by
// accident. Against the same table, `let "x = a[$k] + 1"` — one read and a
// store to a name that is fine — writes `a[q'r]: bad array subscript` twice
// in bash 5.3.20 and answers the operand zero, and `let "a[$k] = 9"`, which
// is a store and no read at all, writes the sentence *no* times. So the
// column writes it per subscript read, and `let "++a[$k]"`'s two lines are
// that one read's pair rather than one line from each side of the operator.
//
// What the store contributes there instead is a third line of its own —
// “let: `a[q'r]': not a valid identifier“, `let`'s complaint about its whole
// operand rather than the subscript's — which
// reportArithSubscriptUnclosedQuoteTarget writes beside this one.
func (r *Runner) reportArithSubscriptUnclosedQuote(x *syntax.ArithIndex) bool {
	if !r.arithSubscriptQuotationRefused(x) {
		return false
	}
	line := r.diag().Report(r.name(), r.line,
		Wording(r.diag().ArithSubscriptUnclosedQuote,
			"%[1]s[%[2]s]: bad array subscript", x.Name, x.Sub))
	r.errf("%s\n%s\n", line, line)
	return true
}

// reportArithSubscriptUnclosedQuoteTarget is the **store** half of the same
// refusal: what the column that refuses says about the operand it would have
// written through, rather than about the subscript it would have read.
//
// Once, and per store rather than per read, which is what parts it from the
// pair above. Measured 2026-09-20 on bash 5.3.20 from a script file with
// standard input on the null device, over `typeset -A a; k="q'r"; a[$k]=4`:
//
//	let "x = a[$k] + 1"	the read's sentence twice, this one never
//	let "a[$k] = 9"    	this one once, the read's never
//	let "++a[$k]"      	the read's twice and then this one — three lines
//
// So the two sentences count different things and neither is the other
// written again, which row two settles on its own: a store with no read in it
// was silent here and one line short of what that column says (#3870).
//
// Named through mathDiagf rather than written with a `let: ` in front of it,
// because the name is the dialect's answer and not this site's. It is the
// same naming Diagnostics.ArithErrorNamesTheBuiltin gives every other
// complaint `let` raises about an expression, and the one column that words
// none of its math failures after the builtin would otherwise have had this
// sentence alone carrying a name.
//
// **Only an associative name**, and that is measured rather than a narrowing
// for safety. On an indexed name, an unset one or a scalar, that column does
// not write this sentence at all: it fails the *whole expression* as
// unreadable arithmetic — `let: ++b[q'r]: bad array subscript (error token is
// "b[q'r]")`, status 1, nothing stored — where every associative row above
// keeps the expression's own value and its own status. That is a different
// answer to a different question, which this shell already parts from on both
// the sentence and the status; writing the association's sentence there would
// be a second wrong answer rather than this one reaching further.
func (r *Runner) reportArithSubscriptUnclosedQuoteTarget(name, sub string) {
	if !r.assocDeclared(name) {
		return
	}
	r.mathDiagf("%s", Wording(r.diag().ArithSubscriptUnclosedQuoteTarget,
		"`%[1]s[%[2]s]': not a valid identifier", name, sub))
}

// wholeArrayElems is every element a name holds, whichever of the three
// shapes holds them: an association's values, an array's elements, or a
// scalar as the one value it is.
func (r *Runner) wholeArrayElems(name string) []string {
	if a, ok := r.assocFor(name); ok {
		return r.assocValues(name, a)
	}
	if elems, ok := r.arrayElems(name); ok {
		return elems
	}
	if v, ok := r.getVar(name); ok {
		return []string{v}
	}
	return nil
}

// arithSubscriptIndex is the number a subscript counts from, on a name that is
// not an association.
//
// Where the parser built a tree that tree is used. Where it did not, the
// brackets held a text it could not read as an expression — which is not a
// parse failure, because the identical text on an associative name is a key
// and the parser cannot see which kind of name it followed. So the reading is
// finished here, at the one point where that is known: read it as an
// expression, and refuse it as one if it will not.
//
// The refusal is worded from the subscript's own text, which makes it the same
// complaint `$(( .accept-line ))` earns. That is what the parser wrote from
// the same text before this moved, and it is what the panel writes: a shell
// says about `a[b c]` exactly what it says about `b c`.
//
// The text is read as it stands, without a second round of expansion. It is
// already the result of one — an arithmetic expansion substitutes into the
// whole expression before reading any of it — so a `$` still in it is a
// literal `$` and not the start of anything.
func (r *Runner) arithSubscriptIndex(x *syntax.ArithIndex) (arithNum, error) {
	// Inside the brackets, for the length of the expression they hold: one
	// dialect refuses an unset name *there* where the same name written
	// outside them is zero. See Semantics.ArithSubscriptNameMustBeSet.
	r.arithSubscriptDepth++
	defer func() { r.arithSubscriptDepth-- }()
	if x.Index != nil || x.Empty {
		n, err := r.evalNum(x.Index)
		return r.blamedOnTheSubscript(x, n, err)
	}
	// Read from the marked text and *shown* from the plain one: the marks
	// are what keep a value's own bracket or quote out of the reading, and
	// they are no part of what a script wrote. See syntax.ArithValueMark.
	marked := r.arithSubscriptRead(x.SubMarked, subscriptAsExpression)
	sub := stripArithValueMarks(marked)
	p := syntax.NewParser("", r.dialect())
	// The subscript's own read rather than the expression's: a double
	// quotation between an index's brackets comes off wherever the brackets
	// came from, which is not true of the expression around them. See
	// syntax.Parser.ParseArithSubscript.
	tree := p.ParseArithSubscript(marked, syntax.Pos{})
	err := unmarkArithFailure(p.Err())
	if err == nil && tree == nil {
		// Brackets holding only space, which is the shape `a[$w]` takes once
		// a `$w` holding spaces has gone in. There is no expression in them
		// and nothing was wrong with what was there either, so the parser has
		// no complaint to hand over, and the panel divides over what that
		// means — see Semantics.BlankArithSubscriptIsTheEmptyExpression. Not
		// the empty pair `a[]`, which is a different answer again and has an
		// axis of its own.
		switch r.sem().BlankArithSubscriptIsTheEmptyExpression {
		case Yes:
			// The brackets hold the blank expression, which is zero, so the
			// element this names is element zero — which is what a nil tree
			// already means to evalNum.
			return r.evalNum(nil)
		case Unspecified:
			// Refused by name rather than guessed at: one answer is a value
			// and the other is no value at all, and neither can stand in for
			// the other.
			return intNum(0), arithError{
				msg:      r.unanswered("a subscript holding only whitespace"),
				complete: true,
			}
		}
		// No: the expression simply ran out, which is the failure the panel
		// names — measured on zsh 5.9.2, `$(( a[ ] ))` against a declared
		// array is `operand expected at end of string`, the same sentence
		// `$(( 1+ ))` earns. Worded through the same path every other
		// subscript failure takes rather than a second copy of it.
		err = &syntax.Error{Kind: syntax.ErrArithOperandEnd, Expr: sub, Token: sub}
	}
	if err != nil {
		return intNum(0), arithError{msg: r.subscriptFailure(sub, err), complete: true}
	}
	n, evalErr := r.evalNum(tree)
	return r.blamedOnTheSubscript(x, n, evalErr)
}

// blamedOnTheSubscript words a failure raised while *evaluating* a subscript
// against the subscript's own text, the way the parser's refusal above is
// already worded against it.
//
// Measured 2026-09-14: `$(( nodecl[1/0] ))` is `1/0: divide by zero` in ksh93
// and `1/0: division by 0 (error token is "0")` in bash, and `$(( 2 + nodecl[
// 1/0] ))` and `$(( x[1/0] ))` are those same two sentences — so the blamed
// extent is what the brackets hold, wherever the brackets stand and whatever
// the name in front of them is. Ours quoted the whole expression, which put
// `nodecl[1/0]` where the shells write the three characters that failed, and
// left bash slicing its error token out of the wrong string: `error token is
// "odecl[1/0] "`, one byte in from a `strings.Index` that had found the
// subscript inside the name (#2420).
//
// Core rather than an axis: the two columns that reach it agree, and the
// other three never arrive — dash and BusyBox ash have no such subscript at
// all, and zsh answers zero without reading the brackets.
//
// Nothing is rewritten when the subscript has no text of its own: an empty
// pair of brackets has nothing to quote, and blaming the empty string would
// send arithFailure back to the whole expression by a longer road.
func (r *Runner) blamedOnTheSubscript(x *syntax.ArithIndex, n arithNum, err error) (arithNum, error) {
	if err != nil {
		// Whatever it comes to be worded as, the failure is a subscript's,
		// which is what decides how much a give-up over it gives up: see
		// Runner.giveUpForABadSubscript. Marked here and not only in
		// Runner.subscriptFailure because a subscript the *parser* built a
		// tree for is refused by the evaluator and worded by the general
		// arithmetic path — `$(( a[1+] ))` names `1+` with no brackets in
		// the sentence, and bash gives a `-c` string up for it exactly as it
		// does for `$(( a[b c] ))` (#3502).
		r.badSubscript = true
	}
	if err == nil || x.Sub == "" {
		return n, err
	}
	if ae, ok := err.(arithError); ok && ae.complete {
		// Already the whole diagnostic — a refusal worded by name further
		// in, or a nested subscript that has been through here — so the
		// extent has been decided and this must not decide it again.
		return n, err
	}
	return intNum(0), arithError{msg: r.arithFailure(x.Sub, err), complete: true}
}

// arithElemValue reads an element as a number, whichever kind of array it
// came out of, by the rule a plain name reads by: empty is zero — what every
// shell in the panel gives a subscript past the end and a name that was never
// an array — and a value that is no literal is re-read as a name where the
// dialect does that.
func (r *Runner) arithElemValue(written, v string) (arithNum, error) {
	if n, err, stop := r.arithRecursionExceeded(written); stop {
		return n, err
	}
	return r.arithNumOfStored(v)
}

// arithIndexWritten is the element as the script wrote it, which is what both
// shells quote back when a read of it will not terminate.
func arithIndexWritten(x *syntax.ArithIndex) string {
	return x.Name + "[" + x.Sub + "]"
}

// arithNameIsSet reports whether the name a subscript follows exists at all,
// which is the question ArithSubscriptSkippedWhenNameUnset asks and not a
// question about the name's value: a scalar holding the empty string is there,
// and so is an array declared with nothing in it.
//
// Three stores rather than one, because a name reaches this from any of them
// and the plain reader cannot see the other two: an associative array is
// declared before it holds a key, and an indexed array declared empty has no
// bare-name value to hand back.
func (r *Runner) arithNameIsSet(name string) bool {
	if r.assocDeclared(name) {
		return true
	}
	if _, ok := r.Arrays[name]; ok && !r.removed[name] {
		return true
	}
	_, ok := r.getVar(name)
	return ok
}

// emptyArithSubscript is `a[]` where an expression reads or writes it — the
// shape `a[$w]` takes once an empty `$w` has been substituted, since an
// arithmetic expansion puts its parameters in before it parses.
//
// handled is false for the one answer that is not a fault at all: the brackets
// are the empty *expression*, and the caller carries on with the ordinary
// element it names. The rest are answered here, and an unanswered preset is
// refused by name rather than given one of them — the value, the stream and
// whether the expression survives all differ. See EmptyArithSubscriptPolicy.
func (r *Runner) emptyArithSubscript(name string) (handled bool, v arithNum, err error) {
	switch r.sem().EmptyArithSubscript {
	case EmptyArithSubscriptIsTheEmptyExpression:
		return false, intNum(0), nil
	case EmptyArithSubscriptIsReported:
		// Reported and then answered: the expression keeps going and the
		// operand is zero, which is why this writes here rather than
		// returning an error for a caller to word.
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			Wording(r.diag().ArithEmptySubscript, "%[1]s[]: bad array subscript", name)))
		return true, intNum(0), nil
	case EmptyArithSubscriptIsInvalid:
		// complete, because the sentence is the subscript machinery's whole
		// complaint: measured, `zsh:1: invalid subscript` with no `bad math
		// expression` in front of it, where an expression that will not parse
		// in the same place carries that prefix.
		return true, intNum(0), arithError{
			msg:      Wording(r.diag().ArithEmptySubscript, "invalid subscript", name),
			complete: true,
		}
	}
	return true, intNum(0), arithError{
		msg:      r.unanswered("a subscript written with nothing in it"),
		complete: true,
	}
}

// emptyArithSubscriptTarget is the same axis where the brackets name a place
// to *write* — `(( m[] = 4 ))` and `(( m[]++ ))` — and reports whether the
// write was answered here rather than by the store below.
//
// One axis and not a second beside it, which is measured rather than tidy:
// each of the three shells with the construct gives the write the same
// disposition it gives the read. ksh93 reads the brackets as the empty
// expression and writes the element that names; bash reports and carries on,
// dropping the store and leaving the expression its value; zsh fails the
// expression. Measured 2026-09-12, `-c`:
//
//	(( m[] = 4 )); echo "st=$?"; echo after
//
//	zsh 5.9.2      `not an identifier: m[]`, the (( )) at 2, `after` runs,
//	               and the expansion spelling ends the script outright
//	bash 5.3.15    m[]': not a valid identifier — with bash's own leading
//	               backquote — nothing written, and
//	               the (( )) at **0** — `x=$(( m[] = 4 ))` gives x the 4 and
//	               `(( m[] = 0 ))` is 1, so the expression keeps its value
//	               and only the store is dropped
//	ksh93u+        silent at 0, and `a=(9 8 7); (( a[] = 4 ))` leaves
//	               `4 8 7` while a table gains the empty key
//	bash 3.2.57    silent at 0 with nothing written, which is bash 5.3
//	               minus the sentence and the one column that splits the
//	               write from the read — its read reports. No dialect here
//	               targets that build, so it is a corpus row rather than a
//	               fourth value.
//
// **The wording is the half that does not carry over**, which is why there is
// a second Diagnostics field and not a second axis: the same shell says
// `m[]: bad array subscript` of a read and, of a write, the sentence
// `m[]': not a valid identifier — with the leading backquote bash puts on it.
// zsh says `invalid subscript` and `not an identifier: m[]`.
//
// One thing measured is deliberately not reproduced: bash puts `((: ` in
// front of this sentence on the *command* route and not on the expansion one,
// which is Diagnostics.ArithErrorNamesTheConstruct's rule reaching a sentence
// that is not an error — the expression carries on. Doing it would mean
// holding the report until the construct that raised it flushes it, at every
// one of the five sites that word a math failure, and a site left out is
// silence. The expansion route is byte-identical today (#1764).
func (r *Runner) emptyArithSubscriptTarget(name string) (handled bool, err error) {
	switch r.sem().EmptyArithSubscript {
	case EmptyArithSubscriptIsTheEmptyExpression:
		return false, nil
	case EmptyArithSubscriptIsReported:
		// Reported and then dropped: the expression keeps going and keeps
		// its value, which is why this writes here rather than returning an
		// error for a caller to word — and why the naming has to be done
		// here too, through the one door every other math failure reaches by
		// traveling back up. See Runner.mathReportf.
		r.mathReportf("%s", Wording(r.diag().ArithEmptySubscriptTarget,
			"`%[1]s[]': not a valid identifier", name))
		return true, nil
	case EmptyArithSubscriptIsInvalid:
		// complete, for the reason the read's is: the sentence is the whole
		// complaint, with no `bad math expression` in front of it.
		return true, arithError{
			msg: Wording(r.diag().ArithEmptySubscriptTarget,
				"not an identifier: %[1]s[]", name),
			complete: true,
		}
	}
	return true, arithError{
		msg:      r.unanswered("a subscript written with nothing in it, naming a place to write"),
		complete: true,
	}
}

// arithPlace is what an expression reads from and writes back to: a name, and
// the subscript it carries when it names an element.
//
// One type rather than two paths, because `++` and `=` disagreed about what a
// target could be — the assignment reached an element and the increment
// refused anything but a bare name, so `(( m[k]++ ))` was an error in every
// dialect while `(( m[k] += 1 ))` was not. A name that can be assigned to can
// be incremented; the operator is not what decides it.
type arithPlace struct {
	name string
	// index is nil when the target is a plain name.
	index syntax.ArithExpr
	// sub is the subscript as written, which is the key on an associative
	// name and the text a refusal quotes on an indexed one.
	sub string
	// subMarked is sub with the value marks still on it, for the reason
	// syntax.ArithIndex.SubMarked carries them: the key a write stores
	// under has to leave a value's own quote characters alone.
	subMarked string
	// empty says the brackets held nothing — `(( a[]++ ))` — which index
	// alone cannot say, since a plain name has no index either. Carried so
	// the read the operator makes reaches the same answer `$(( a[] ))` does.
	empty bool
	// flags is the group the subscript opened with, where the dialect has
	// them: `(( a[(r)20] = 9 ))` writes the element the same search reads,
	// and the write has to name it by the same rule the read does or the two
	// spellings write different elements.
	flags *syntax.SubscriptFlags
	// subscripted says brackets were written at all, which neither of the two
	// above can say on its own: `(( m[.k]++ ))` has no index and is not
	// empty, and so does a plain name. Without it a key that is not an
	// expression read and wrote the *bare name* — `(( m[.k] = 3 ))` would set
	// m rather than the element, which is a wrong answer with no diagnostic.
	subscripted bool
	// unclosedQuote says the subscript's brackets were found only once the
	// scan gave up on a quotation that never closed, carried so the write
	// reaches the same answer the read does — the two together are what
	// write bash's sentence twice for one `++`. See
	// syntax.ArithIndex.UnclosedQuote.
	unclosedQuote bool
}

// arithAssignmentDeclaresAnInteger reports whether writing this name from
// inside arithmetic gives it the integer attribute.
//
// Only a name the assignment *creates*, which is the half of the rule the
// issue's table could not show and a probe on a fresh shell cannot see:
// measured 2026-09-12 on zsh 5.9.2, `(( x = 5 ))` leaves `typeset -i x=5`
// where `x=3; (( x = 5 ))` and even `typeset x; (( x = 5 ))` leave an
// ordinary scalar. So it is a declaration and not an attribute the operator
// applies.
//
// The axis is **read** rather than asked, which is the reason
// Semantics.FailedExpansionAbandonsTheLine gives one file over: where the
// answer is not yes, leaving an ordinary scalar is what three of the four
// shells do and what this path already did, so an unanswered axis has a
// correct answer to fall back on rather than a missing one to complain about.
// Asking would refuse `for (( i=0; i<3; i++ ))` in a run with no dialect,
// which is a construct the core has and a question the script never posed.
func (r *Runner) arithAssignmentDeclaresAnInteger(name string) bool {
	if r.sem().ArithmeticAssignmentDeclaresAnInteger != Yes {
		return false
	}
	_, set := r.getVar(name)
	return !set
}

// declareIntegerFromArithmetic gives a name the arithmetic just created the
// integer attribute, and the output base a radix literal in the expression
// wrote.
//
// The base comes from the *expression* rather than from the text stored,
// which is the only place it survives: the answer is a decimal number by the
// time it is written down. Measured, `(( y = 0x1f ))` is `typeset -i16 y=31`
// and `(( y = 1 + 0x1f ))` is `typeset -i16 y=32`, so it is any radix literal
// the expression holds and not only one standing alone — while `y=0x1f; ((
// z = y ))` is a plain `typeset -i z=31`, the prefix having arrived through a
// value rather than been written here.
func (r *Runner) declareIntegerFromArithmetic(name string, from syntax.ArithExpr) {
	if r.integer == nil {
		r.integer = map[string]bool{}
	}
	r.integer[name] = true
	// The expression's own output specifier first, which is a base the script
	// wrote as squarely as a literal is: `(( i = [#16] 255 ))` on an unset
	// name is `typeset -i16 i=255`. It is not a base a name that *already*
	// had the attribute learns — `typeset -i i; (( i = [#16] 255 ))` stays
	// plain — but that route never reaches here, this being a declaration.
	base := 0
	if f := r.arithOutput; f != nil && f.Based {
		base = f.Base
	}
	padded := false
	if base == 0 {
		base, padded = radixWritten(from)
	}
	if base == 0 && padded && r.octalLeadingZero() {
		// The leading zero is a radix as much as a prefix is, where the
		// dialect reads one as octal — `let "x=010"` under `setopt
		// octal_zeroes` is `8#10` on zsh 5.9.2, the same answer the
		// declaration route gives `typeset -i d=010`. Asked here rather than
		// inside the walk because what numerals the expression holds is a
		// property of the text and what a leading zero *means* is the
		// dialect's, and asked only where there is such a numeral to ask
		// about: `(( x = 5 ))` poses no question (#3520).
		base = 8
	}
	if base == 0 || !r.validIntegerBase(base) {
		return
	}
	if base == 10 && r.integerBaseTenIsNone() {
		return
	}
	if r.integerBase == nil {
		r.integerBase = map[string]int{}
	}
	r.integerBase[name] = base
}

// radixWritten is the base named by the first radix literal in an expression,
// or 0 where it holds none — and whether any numeral in it is a **bare
// leading zero**, which is a base where the dialect reads one as octal.
//
// Both in one walk because they are one question asked of the same numerals,
// and because a second walk beside this one is how a node kind comes to be
// read by one of them and not the other.
//
// The leading zero reaches as far into the expression as a prefix does, which
// is measured rather than assumed: on zsh 5.9.2 under `setopt octal_zeroes`,
// `let "y=1+010"` is `8#11` exactly as `(( u = 1 + 0x1f ))` is `16#20`. The
// first guess here was that a leading zero counted only as the whole of what
// was written, and the probe said otherwise (#3520).
//
// The base wins where an expression holds both. Nothing measures that pair —
// `(( q = 0x1f + 010 ))` is not a spelling anyone writes — and it is the
// order the two readings already stood in.
func radixWritten(e syntax.ArithExpr) (base int, padded bool) {
	switch x := e.(type) {
	case nil:
		return 0, false
	case *syntax.ArithNum:
		return integerBaseOfLiteral(x.Text), octalNumeral(x.Text)
	case *syntax.ArithUnary:
		return radixWritten(x.X)
	case *syntax.ArithCond:
		if b, p := radixWritten(x.Then); b != 0 || p {
			return b, p
		}
		return radixWritten(x.Else)
	case *syntax.ArithBinary:
		if b, p := radixWritten(x.X); b != 0 || p {
			return b, p
		}
		return radixWritten(x.Y)
	case *syntax.ArithAssign:
		return radixWritten(x.Value)
	}
	return 0, false
}

// arithPlaceOf is the target an operator can write through, and false for an
// expression that names no storage — `(( 1++ ))`, `(( (a)++ ))`.
func arithPlaceOf(e syntax.ArithExpr) (arithPlace, bool) {
	switch x := e.(type) {
	case *syntax.ArithVar:
		return arithPlace{name: x.Name}, true
	case *syntax.ArithIndex:
		return arithPlace{
			name: x.Name, index: x.Index, sub: x.Sub, subMarked: x.SubMarked,
			empty: x.Empty, flags: x.Flags, unclosedQuote: x.UnclosedQuote,
			subscripted: true,
		}, true
	}
	return arithPlace{}, false
}

// readPlace is the value a target currently holds.
func (r *Runner) readPlace(p arithPlace) (arithNum, error) {
	if !p.subscripted {
		return r.arithValueOf(p.name)
	}
	return r.arithElement(&syntax.ArithIndex{
		Name: p.name, Index: p.index, Sub: p.sub, SubMarked: p.subMarked,
		Empty: p.empty, Flags: p.flags, UnclosedQuote: p.unclosedQuote,
	})
}

// writePlace stores a value back through a target, written the way the
// dialect writes a number — so `i+=1.5` leaves 1.5 behind and not 1.
func (r *Runner) writePlace(p arithPlace, v arithNum, from syntax.ArithExpr) error {
	was := r.refusedInACommand
	r.refusedInACommand = false
	err := r.storePlace(p, v, from)
	if err == nil && r.refusedInACommand {
		// Refused and already reported, and the evaluation stops here: `let
		// x=2 y=3` and `(( x = 2, y = 3 ))` leave y unset over a frozen x in
		// every column. See Runner.refuseReadonlyInACommand.
		err = errReadonlyRefusedInACommand
	}
	r.refusedInACommand = r.refusedInACommand || was
	return err
}

// errReadonlyRefusedInACommand is an evaluation stopped by a refusal that
// was already written, so the caller says nothing more about it.
var errReadonlyRefusedInACommand = errors.New("readonly refusal already reported")

func (r *Runner) storePlace(p arithPlace, v arithNum, from syntax.ArithExpr) error {
	// The expression's output format reaches the value an assignment stores,
	// not only the answer an expansion produces: measured, `x=5; (( x = [#16]
	// 255 ))` leaves x holding the six characters `16#FF`.
	text := r.formatArith(v)
	if p.empty {
		// A write through brackets with nothing in them is the same axis the
		// read asks, in a sentence of its own — see
		// emptyArithSubscriptTarget, and Semantics.EmptyArithSubscript for
		// the three answers.
		if handled, err := r.emptyArithSubscriptTarget(p.name); handled {
			return err
		}
		// Not handled: the brackets hold the empty *expression*, which names
		// element zero on an ordered name and the empty key on a table —
		// exactly the element `$(( a[] ))` reads. Measured 2026-09-12 on
		// ksh93u+, `a=(9 8 7); (( a[] = 4 ))` leaves `4 8 7` and a table
		// gains the empty key.
		//
		// The flag stays set, because it is what makes arithSubscriptIndex
		// answer zero below without reading a text there is none of. What
		// the branch beneath used to do instead is write through the *bare
		// name*, which would have replaced the array with a number.
	}
	if !p.subscripted {
		// The bare name alone. The empty pair used to join it here, on the
		// grounds that a dialect reading `a[]` as the empty expression and
		// not answering it above had nowhere else to go — and that was the
		// wrong place: the shell with that reading writes the *element* the
		// empty expression names, so the pair goes to the element path with
		// everything else and only a target with no brackets at all is here
		// (#1764).
		if r.arithAssignmentDeclaresAnInteger(p.name) {
			// A name the arithmetic itself created carries the base on the
			// *name* rather than in the characters it holds, which is the
			// other half of the note above: `x=5; (( x = [#16] 255 ))` leaves
			// the six characters `16#FF` in an ordinary scalar, and the same
			// expression on an unset name is `typeset -i16 x=255` reading
			// back as `16#FF`. So the plain number is stored and the
			// declaration renders it.
			r.setVar(p.name, r.formatNum(v))
			r.declareIntegerFromArithmetic(p.name, from)
			r.rerenderInTheNewBase(p.name)
			return nil
		}
		r.setVar(p.name, text)
		return nil
	}
	if p.flags != nil && r.assocDeclared(p.name) {
		// The group is read before the association, exactly as arithElement
		// reads it before the association on the way in: a table consulted
		// first takes the group's own letters for part of the key, which is
		// what this shell did — `(( m[(r)1] = 5 ))` left a table holding a
		// key literally named `(r)1`, silently, at status 0.
		//
		// Measured 2026-09-12 on zsh 5.9.2, the one shell with the construct,
		// with `typeset -A m; m=(aa 1)`:
		//
		//	(( m[(r)1] = 5 ))     nothing written, nothing said, status 0
		//	(( m[(k)aa] = 5 ))    the same, and the key it matched is untouched
		//	(( m[(k)aa]++ ))      the same again, so it is the store and not
		//	                      the operator
		//	x=$(( m[(k)aa] = 5 )) x is 5 and the table is unchanged, so the
		//	                      expression has its value and only the store
		//	                      is dropped
		//	(( m[(e)aa] = 5 ))    the key `aa`, so a group selecting nothing
		//	                      leaves an ordinary key behind it
		//
		// A search naming a place to *write* in a table is refused by name on
		// the left of `=` — see assignFlaggedTableElement — and dropped in
		// silence here, which is one shell giving two answers to what looks
		// like one question. The reading is not this engine's to reconcile:
		// the store is the same store, and only the route to it differs.
		search, ok := r.subscriptSearch(&syntax.ParamExpr{
			Name: p.name, Index: p.flags.Arg, IndexFlags: p.flags,
		})
		if !ok || search != 0 {
			// Either a letter this does not carry, refused by name already,
			// or a search over a table — which writes nowhere and says
			// nothing, and is why this returns no error: the expression keeps
			// its value and `(( ))` still ends at the value's own status.
			return nil
		}
		// The group selects nothing, so the operand behind it is the key —
		// the same rewrite arithElement makes on the way in, and through the
		// same accessor, so the two spellings cannot name different keys.
		// The operand behind the group is a joined *word* and carries no
		// marks, so both readings of the subscript are the one text.
		operand := r.joinWord(p.flags.Arg)
		p.sub, p.subMarked, p.flags = operand, operand, nil
	}
	if r.arithSubscriptQuotationRefused(&syntax.ArithIndex{
		Name: p.name, Index: p.index, Sub: p.sub, SubMarked: p.subMarked,
		Empty: p.empty, UnclosedQuote: p.unclosedQuote,
	}) {
		// Nothing written, and the sentence is the *operand's* rather than
		// the subscript's — the read's `bad array subscript` belongs to the
		// read and is written there. No error either, which would fail the
		// whole expression where the column that refuses lets it finish.
		// See reportArithSubscriptUnclosedQuoteTarget.
		r.reportArithSubscriptUnclosedQuoteTarget(p.name, p.sub)
		return nil
	}
	if r.assocDeclared(p.name) {
		r.setAssocElem(p.name, r.arithAssocKey(r.arithSubscriptRead(p.subMarked, subscriptAsKey)), text)
		return nil
	}
	// Through the same reader the element is read by, so a text that is no
	// expression is refused here as it is there — refused rather than
	// written, which is what bash 5.3 and zsh 5.9.2 both do: `(( a[.k] = 9 ))`
	// on an indexed array complains and leaves every element as it was. One
	// call rather than a guard and an evaluation, so the number a subscript
	// counts from cannot be worked out one way for the read and another for
	// the write.
	if p.flags != nil {
		// A group names the element on this side too, by the same rule the
		// read side names it: measured, `a=(10 20 30); (( a[(r)20] = 9 ))`
		// leaves `10 9 30`, which is `${a[(r)20]}`'s element. Through
		// flaggedTargetIndex, so an assignment's refusal is the fatal one and
		// `unset`'s is not — the one thing the two sides do not share.
		idx, ok := r.flaggedTargetIndex(&syntax.ParamExpr{
			Name: p.name, Index: p.flags.Arg, IndexFlags: p.flags,
		}, true)
		if !ok {
			// Reported by name already, and nothing written.
			return nil
		}
		r.setArrayElem(p.name, idx, p.sub, text)
		return nil
	}
	target := &syntax.ArithIndex{Name: p.name, Index: p.index, Sub: p.sub, SubMarked: p.subMarked, Empty: p.empty}
	if r.reportArithWholeArraySubscript(target) {
		// Named and nothing written, and *not* an error: measured, `(( a[*] =
		// 5 ))` reports, leaves every element as it was and ends at 0 in the
		// column that reports — where an error here would fail the whole
		// `(( ))`. `(( a[*]++ ))` writes the sentence twice, once for the
		// read and once for this.
		return nil
	}
	idx, err := r.arithSubscriptIndex(target)
	if err != nil {
		return err
	}
	sub := p.sub
	if sub == "" {
		sub = r.formatNum(idx)
	}
	r.setArrayElem(p.name, idx.asInt(), sub, text)
	return nil
}

// charCode answers the character-code operator, which is a *character* and
// never a number: `$((#b))` on `zebra` is 122 and not 5, and the length is
// spelled `$(( $#b ))`.
//
// Nothing here reports. Measured on zsh 5.9.2, every operand that names
// nothing is zero: a parameter never set, one holding the empty string, a `#`
// with nothing after it at all, and a name written with a subscript — which
// the shell reads and then finds nothing under, so `$((#a[1]))` is 0 however
// `${a[1]}` reads. The last of those is a fact about that shell rather than
// something derived, and the quiet answer is the one to keep: a refusal here
// would be louder than the shell a script was written for.
func (r *Runner) charCode(x *syntax.ArithCharCode) int {
	if x.Char != "" {
		text := x.Char
		if x.Op != "#" {
			// The escapes are the ones `$'…'` decodes, against this dialect's
			// table rather than a second copy of it. That covers `##c` and
			// the `'c'` constant alike — measured, `$(( '\101' ))` is 65 in
			// the shell that has the constant, so its escapes are the same
			// ones. The single-`#` spelling is the exception and has none: a
			// backslash there takes the next character as itself.
			text = r.expandDollarSingle(text)
		} else {
			text = strings.TrimPrefix(text, `\`)
		}
		return firstCharCode(text)
	}
	if x.Name == "" || x.Subscripted {
		return 0
	}
	v, ok := r.getVar(x.Name)
	if !ok {
		v, _ = r.specialParam(&syntax.ParamExpr{Name: x.Name})
	}
	return firstCharCode(v)
}

// firstCharCode is the code of the first character of a string, and 0 for a
// string with no first character.
//
// A character and not a byte where the text holds one: `é` is 233 rather than
// the 195 its first byte is. But a byte that is no character at all is its own
// value — measured, a lone 0x80 is 128 — rather than the replacement rune,
// which is a number no shell produces and which the decoder would otherwise
// hand back for every high byte a `\x` escape can write.
func firstCharCode(s string) int {
	if s == "" {
		return 0
	}
	c, size := utf8.DecodeRuneInString(s)
	if c == utf8.RuneError && size <= 1 {
		return int(s[0])
	}
	return int(c)
}

func (r *Runner) evalUnary(x *syntax.ArithUnary) (arithNum, error) {
	// ++ and -- read and write a variable, so they need its name rather than
	// its value.
	if x.Op == "++" || x.Op == "--" {
		place, ok := arithPlaceOf(x.X)
		if !ok {
			return intNum(0), arithError{
				msg: Wording(r.diag().ArithIncrementNeedsAPlace,
					"%[1]s needs a variable", x.Op),
			}
		}
		old, err := r.readPlace(place)
		if err != nil {
			return intNum(0), err
		}
		step := 1.0
		if x.Op == "--" {
			step = -1
		}
		next := r.addNum(old, step)
		if err := r.writePlace(place, next, nil); err != nil {
			return intNum(0), err
		}
		if x.Postfix {
			// The difference between the two spellings is what they evaluate
			// to, not what they do.
			return old, nil
		}
		return next, nil
	}

	v, err := r.evalNum(x.X)
	if err != nil {
		return intNum(0), err
	}
	switch x.Op {
	case "-":
		if v.floatKind() {
			return floatNum(-v.f), nil
		}
		return r.carriedInADouble(-v.asFloat(), -v.asInt()), nil
	case "+":
		return v, nil
	case "~":
		i, err := r.integerOperand(v, "~")
		if err != nil {
			return intNum(0), err
		}
		return r.carried(^i), nil
	case "!":
		return intNum(boolInt(v.isZero())), nil
	}
	return intNum(0), arithError{msg: "unknown unary " + x.Op}
}

// addNum steps a value by one, keeping it whichever kind it was.
func (r *Runner) addNum(n arithNum, step float64) arithNum {
	if n.floatKind() {
		return floatNum(n.f + step)
	}
	return r.carriedInADouble(n.asFloat()+step, n.asInt()+int(step))
}

func (r *Runner) evalAssign(x *syntax.ArithAssign) (arithNum, error) {
	place := arithPlace{
		name: x.Name, index: x.Index, sub: x.Sub, subMarked: x.SubMarked,
		flags: x.Flags, empty: x.Empty, unclosedQuote: x.UnclosedQuote,
		// Brackets were written at all, which none of the three fields above
		// can say on its own: an empty pair has no index and no text, and so
		// has a plain name. The empty pair used to be refused while parsing
		// and so could be left out of this — see ArithAssign.Empty (#1764).
		subscripted: x.Index != nil || x.Sub != "" || x.Empty,
	}
	v, err := r.evalNum(x.Value)
	if err != nil {
		return intNum(0), err
	}
	if x.Op != "=" {
		old, err := r.readPlace(place)
		if err != nil {
			return intNum(0), err
		}
		v, err = r.apply(strings.TrimSuffix(x.Op, "="), old, v)
		if err != nil {
			return intNum(0), err
		}
	}
	if x.Op == "^^=" {
		// The one compound assignment that assigns nothing. Measured
		// 2026-09-18 on zsh 5.9.2, the only shell with the operator:
		// `x=5; echo $(( x ^^= 1 ))` writes 0 and leaves `x` at 5, where
		// `||=` and `&&=` beside it in the same manual both store — `x ||= 0`
		// leaves 1. So the value is the exclusive-or and the place is not
		// touched, which also means a frozen name is no refusal here.
		return v, nil
	}
	// The numeric type the target carries, read **before** the store because
	// a store can create one: see numericAttribute.
	was := r.numericAttributeOf(place.name)
	// The side effect that outlives the expression.
	if err := r.writePlace(place, v, x.Value); err != nil {
		return intNum(0), err
	}
	if x.Op == "=" {
		// And the value of the assignment is the number that type makes of
		// it. Applied **after** the store and not to the value the store is
		// handed: the store renders a number and reads the characters back
		// through the name's own attribute, so handing it one already
		// converted is a second trip through the text. Measured, `integer h;
		// (( h = -1e30 ))` stores the word's least value in zsh 5.9.2 —
		// converting first writes `-9223372036854775808`, which the integer
		// attribute reads as a unary minus over a literal too wide for the
		// word and truncates to `-922337203685477580`, with the same
		// complaint real zsh makes about that literal written out.
		v = was.converts(v)
	}
	return v, nil
}

// numericAttribute is the numeric type a name carries: `-i`, one of the float
// letters, or neither.
//
// A value rather than a question asked of the Runner, because the answer has
// to be taken **before** an assignment stores anything and used **after**. An
// arithmetic assignment to a name that does not exist declares one in zsh —
// see arithAssignmentDeclaresAnInteger — so asking afterwards finds an
// attribute the assignment itself created, and a name the assignment created
// takes its type from the value and converts nothing. Measured 2026-09-26 on
// zsh 5.9.2, `$(( xx = 1.5 ))` on an unset name is `1.5` and leaves `xx` a
// float; asking after the store makes it `1`.
type numericAttribute struct {
	isFloat   bool
	isInteger bool
}

// numericAttributeOf reads the numeric type a name carries right now.
func (r *Runner) numericAttributeOf(name string) numericAttribute {
	_, isFloat := r.floatPrecision[name]
	return numericAttribute{isFloat: isFloat, isInteger: r.integer[name]}
}

// converts is the number this numeric type makes of a value written into it by
// a plain `=`: the truncated integer under `-i`, the float under `-E` or `-F`,
// and the value untouched where the name carries neither.
//
// **It is the value of the assignment expression and not only what is
// stored**, which is the whole of #4595: `integer i; float f=3.1415` makes
// `$(( i = f * 10000 ))` the five characters `31415` where the same expression
// without the assignment is `31415.`, trailing point and all. We stored the
// truncated integer correctly and handed the untruncated float back, so the
// expansion rendered a float the parameter never held.
//
// **The value is the converted *number* and not the *text* that was stored**,
// and that is the discriminating half. A store renders the number in the
// name's own places — `16#6C` under `-i16`, `3.142` under `-F3` — and the
// assignment's value is neither of those. Measured 2026-09-26 on zsh 5.9.2
// (aarch64-apple-darwin25.4.0) `-f`, and on ksh93u+ 2012-08-01, which answers
// the same:
//
//	typeset -i16 a   $(( a = 108 ))                108      $a  16#6C
//	typeset -F3 g    $(( g = 3.14159265358979 ))   3.14159265358979   $g  3.142
//
// So reading the stored text back would be a different rule that agrees with
// this one only on a name with no rendering of its own. The two rows above
// hold the *type* fixed and move the rendering, and the answer does not move;
// moving the type moves it — `integer i` makes `$(( i = 1.5 ))` 1 and a name
// with no attribute leaves it 1.5.
//
// **Only the plain `=`.** zsh converts what `=` stores and hands a compound
// assignment its computed value: measured in the same run, `integer b=1` makes
// `$(( b += 0.5 ))` 1.5 while `$(( b = b + 0.5 ))` is 1, with `b` at 1 either
// way. That pair holds the attribute fixed and moves the operator, which is
// what says the rule is keyed on the operator and not on "an assignment".
// ksh93 converts both — `$(( n += 0.5 ))` on a `typeset -i n=1` is 1 there —
// and that divergence is recorded and not modeled here; it is a shell's answer
// to a question this rule does not ask. Nothing in this repository asks it
// yet, so it is an issue rather than an axis (#4606).
//
// The conversion is asInt and asFloat rather than a rule written out again
// here, because it is the one the store already makes: `attributeFolded`
// evaluates an integer name's text through the same `evalArith`, so a second
// truncation beside it is the shape that drifts. Float ahead of integer for
// that function's reason too — the two attributes cannot both stand, because
// applyAttributes takes one off as the other arrives, and reaching the float
// first is what makes that a statement rather than a hope. **Swapping the two
// branches is an equivalent mutant today**, and is kept this way round rather
// than reordered: nothing a script can write gives a name both, so the order
// is unobservable until something does, and then this is the order that is
// right. `typeset -iE 3 a=1.5` is `typeset -i3 a=1` here — one letter, not
// half of each.
//
// The name is the target's, subscripted or not: measured, `typeset -i b` makes
// `$(( b[2] = 1.5 ))` 1 in zsh and in ksh93 alike, so the attribute belongs to
// the name and reaches every element of it.
func (a numericAttribute) converts(v arithNum) arithNum {
	if a.isFloat {
		return floatNum(v.asFloat())
	}
	if a.isInteger {
		return intNum(v.asInt())
	}
	return v
}

// evalDecidedOperand runs the operand of `&&` or `||` whose value can no
// longer change the answer, in the shell that runs it.
//
// The axis is asked here and not at the top of evalBinary, because the two
// readings are indistinguishable unless the operand does something that
// outlives it: `$((0 && 1))` and `$((a || b))` produce the same number and
// leave the same state under either answer. Asking on the common path would
// make an unanswered vector refuse the commonest arithmetic in the language,
// which is the mistake docs/spec/semantics.md names — ask at the
// disagreement, not on the path to it.
//
// The error is returned rather than swallowed: a shell that evaluates the
// decided operand also raises what it fails on, which is how the panel shows
// that the operand is evaluated at all and not merely assigned into.
func (r *Runner) evalDecidedOperand(y syntax.ArithExpr) error {
	a := r.sem().ArithShortCircuitEvaluatesTheRightOperand
	if a == Unspecified && !arithOutlivesTheExpression(y) {
		return nil
	}
	if !r.ask(a, "the right operand of `&&` or `||` being evaluated after the answer is decided") {
		return nil
	}
	_, err := r.evalNum(y)
	return err
}

// arithOutlivesTheExpression reports whether evaluating this operand could
// leave anything behind: an assignment, or an increment or decrement, however
// deep it is written.
//
// It is deliberately a question about *effects* and not about errors. A
// division by zero inside a decided operand is a third thing the panel splits
// on — bash 3.2 raises it while assigning nothing — and no static walk can
// find it, so what this decides is only when the axis is worth asking. A
// dialect that has answered gets its answer whatever the operand holds.
func arithOutlivesTheExpression(x syntax.ArithExpr) bool {
	switch x := x.(type) {
	case *syntax.ArithAssign:
		return true
	case *syntax.ArithUnary:
		return x.Op == "++" || x.Op == "--" || arithOutlivesTheExpression(x.X)
	case *syntax.ArithBinary:
		return arithOutlivesTheExpression(x.X) || arithOutlivesTheExpression(x.Y)
	case *syntax.ArithCond:
		return arithOutlivesTheExpression(x.Cond) ||
			arithOutlivesTheExpression(x.Then) ||
			arithOutlivesTheExpression(x.Else)
	case *syntax.ArithIndex:
		return arithOutlivesTheExpression(x.Index)
	case *syntax.ArithCall:
		for _, a := range x.Args {
			if arithOutlivesTheExpression(a) {
				return true
			}
		}
	case *syntax.ArithOutput:
		return arithOutlivesTheExpression(x.X)
	}
	return false
}

func (r *Runner) evalBinary(x *syntax.ArithBinary) (arithNum, error) {
	// The short-circuiting operators usually must not evaluate their right
	// side when the answer is already known, because that side can assign —
	// and one shell in the panel evaluates it anyway. See
	// Semantics.ArithShortCircuitEvaluatesTheRightOperand: the *value* is the
	// operator's either way, so what the axis decides is only whether the
	// operand's effects happen.
	switch x.Op {
	case "&&":
		l, err := r.evalNum(x.X)
		if err != nil {
			return intNum(0), err
		}
		if l.isZero() {
			return intNum(0), r.evalDecidedOperand(x.Y)
		}
		v, err := r.evalNum(x.Y)
		return intNum(boolInt(!v.isZero())), err
	case "||":
		l, err := r.evalNum(x.X)
		if err != nil {
			return intNum(0), err
		}
		if !l.isZero() {
			return intNum(1), r.evalDecidedOperand(x.Y)
		}
		v, err := r.evalNum(x.Y)
		return intNum(boolInt(!v.isZero())), err
	}

	l, err := r.evalNum(x.X)
	if err != nil {
		return intNum(0), err
	}
	rv, err := r.evalNum(x.Y)
	if err != nil {
		return intNum(0), err
	}
	if x.Op == "," {
		// The sequence operator evaluates both and yields the right.
		return rv, nil
	}
	v, err := r.apply(x.Op, l, rv)
	if ae, ok := err.(arithError); ok && ae.token == "" {
		// apply sees values, not the tree, so the operand that caused the
		// failure is named here where the tree is still in hand. `5/y` with
		// y unset is blamed on `y` rather than on the zero it became.
		ae.token = arithToken(x.Y)
		// And the blame runs from there to the end of the expression, which
		// is the shape an evaluation failure takes in the one column that
		// quotes a token at all. Where that text begins splits by operator,
		// measured 2026-09-12 on bash 5.3.15:
		//
		//	$(( 1/((0)) ))   division by 0 (error token is "((0)) ")
		//	$(( 2**-1 ))     exponent less than 0 (error token is "1 ")
		//
		// A division names the *divisor as written*, so the parser's offset
		// is what answers it; the exponent names the last operand read, which
		// is the leaf the tree already hands back and which the sign is not
		// part of. -1 asks for the second.
		ae.blameTail, ae.from = true, -1
		if x.Op == "/" || x.Op == "%" {
			ae.from = x.YStart
		}
		err = ae
	}
	if ae, ok := err.(arithError); ok && ae.dividedByZero && r.arithValueSurvivesTheDivision &&
		r.ask(r.sem().ArithDivisionByZeroYieldsAValue,
			"arithmetic going on past a division by zero with a value") {
		// The failure is held rather than returned, so the rest of the
		// expression is evaluated with the value apply left beside it:
		// `1/0+9` is 9 in ksh93 and `8%0*2` is 16. The caller raises what is
		// held once the whole expression has been read — see
		// Runner.arithDivisionFailure, and the mode's own comment for why
		// only one caller turns this on.
		if r.arithDivisionFailure == nil {
			r.arithDivisionFailure = err
		}
		return v, nil
	}
	return v, err
}

// apply is a method because a division by zero is worded by the dialect,
// and the receiver is named `sh` because `r` is already the right operand.
//
// An operation is floating point when either operand is, and integer
// otherwise — so `3/2` is 1 even in a shell that has floats, and `3.0/2` is
// 1.5. A comparison is the exception in the other direction: it answers 0 or 1
// whatever it compared.
func (sh *Runner) apply(op string, l, r arithNum) (arithNum, error) {
	if v, ok, err := sh.compare(op, l, r); ok {
		return v, err
	}
	if op == "^^" || op == "^^=" {
		// The logical exclusive-or: 1 where exactly one operand is true, the
		// same 1-or-0 every other logical operator yields. Ahead of the
		// floating branch because truth is a property of the value and not of
		// its kind — measured, `$(( 1.5 ^^ 2.5 ))` is 0 and `$(( 1.5 ^^ 0 ))`
		// is 1 in the shell that has the operator. Both operands are
		// evaluated by the caller, since neither side can decide the answer
		// alone and there is nothing to short-circuit.
		return intNum(boolInt(l.isZero() != r.isZero())), nil
	}
	if l.floatKind() || r.floatKind() {
		return sh.applyFloat(op, l, r)
	}
	li, ri := l.asInt(), r.asInt()
	switch op {
	case "+":
		return sh.carriedInADouble(l.asFloat()+r.asFloat(), li+ri), nil
	case "-":
		return sh.carriedInADouble(l.asFloat()-r.asFloat(), li-ri), nil
	case "*":
		return sh.carriedInADouble(l.asFloat()*r.asFloat(), li*ri), nil
	case "/", "%":
		if ri == 0 {
			// The value beside the failure is what the one column that goes
			// on evaluating goes on with: zero for a division and the
			// dividend for a remainder. It is discarded wherever the error
			// is fatal, which is everywhere but one — see
			// Semantics.ArithDivisionByZeroYieldsAValue and evalBinary.
			v := intNum(0)
			if op == "%" {
				v = l
			}
			return v, arithError{
				msg:           Wording(sh.diag().DivisionByZero, "division by zero"),
				dividedByZero: true,
			}
		}
		if op == "/" {
			return sh.carried(li / ri), nil
		}
		return sh.carried(li % ri), nil
	case "<<":
		return sh.carried(li << shiftCount(ri)), nil
	case ">>":
		return sh.carried(li >> shiftCount(ri)), nil
	case "&":
		return sh.carried(li & ri), nil
	case "^":
		return sh.carried(li ^ ri), nil
	case "|":
		return sh.carried(li | ri), nil
	case "**":
		return sh.intPow(li, ri)
	}
	return intNum(0), arithError{msg: "unknown operator " + op}
}

const (
	maxInt = int(^uint(0) >> 1)
	minInt = -maxInt - 1
)

// shiftCount is the count a shift actually uses, which is the low six bits of
// the one written.
//
// Every reference shell on the panel hands the count to a C shift operator on
// a 64-bit word, and the machine those all run on takes the count modulo the
// width rather than answering zero for a count that is too large. Measured
// 2026-09-16 across bash 5.3.20, bash-as-sh, bash 3.2, zsh 5.9.2, ksh93u+
// 2012-08-01, dash 0.5.12 and BusyBox ash 1.37.0: `1<<64` is 1 in all seven,
// `1<<65` is 2, `8>>64` is 8, and `1<<-1` is the most negative value — the
// count -1 arriving as 63. Go's shift is the defined one instead and answers
// zero past the width, which is why this is written down: unanimous, so no
// axis, and the one place a shell's C heritage shows through the language
// this is written in.
func shiftCount(n int) uint { return uint(n) & 63 }

// carried is carriedInADouble for an operation that cannot overflow the word:
// the two readings are the same arithmetic and differ only where the value is
// past what a double holds exactly.
func (sh *Runner) carried(v int) arithNum { return sh.carriedInADouble(float64(v), v) }

// carriedInADouble is the value an integer result leaves the shell holding.
//
// Two readings are handed in and the axis chooses between them. `exact` is
// integer arithmetic on the machine word, wrapping at the edge, which is what
// bash, zsh, dash and BusyBox ash do. `dbl` is the same operation carried out
// in the C double ksh93 keeps every arithmetic value in — so a result past
// 2^53 loses its low bits, and one past the word does not wrap at all.
//
// They agree for every value small enough, and the axis is asked only where
// they do not. That matters: this is on the path of every integer operation in
// the shell, and an axis asked unconditionally would report itself unanswered
// on `$(( 1 + 1 ))` in a run with no dialect.
//
// The int/float split of the answer is ksh93's own and not a magnitude: the
// value is written as an integer when a saturating `(intmax_t)` cast of it
// converts back to the same double, and in floating notation when it does not.
// That is why `$(( 3037000499*3037000499 ))` is 9223372030926248960 — an
// integer, the product rounded — while `$(( 2**64 ))` is 1.84467440737096e+19,
// and why `$(( big + 1 ))` on the largest value is that value again: 2^63 casts
// to the maximum and the maximum converts back to 2^63.
func (sh *Runner) carriedInADouble(dbl float64, exact int) arithNum {
	i, fits := intFromDouble(dbl)
	if fits && i == exact {
		return intNum(exact)
	}
	if !sh.ask(sh.sem().ArithValuesAreCarriedInAFloat, "arithmetic carried in a float rather than the machine word") {
		return intNum(exact)
	}
	if fits {
		return intNum(i)
	}
	return wideNum(dbl)
}

// intFromDouble is the value a C `(intmax_t)` cast of a double leaves, and
// whether that cast converts back to the double it was given.
//
// The cast saturates rather than wrapping, which is what the hardware this was
// measured on does and what makes the maximum its own round trip. Go leaves an
// out-of-range float-to-int conversion undefined, so the ends are handled
// before the conversion rather than after it.
func intFromDouble(f float64) (int, bool) {
	switch {
	case math.IsNaN(f):
		return 0, false
	case f >= float64(maxInt):
		return maxInt, f == float64(maxInt)
	case f <= float64(minInt):
		return minInt, f == float64(minInt)
	}
	i := int(f)
	return i, float64(i) == f
}

// intPow is `**` on integers.
//
// A negative exponent cannot yield an integer, and the shells that parse the
// operator split on what to do about it: one refuses, two answer with a
// float — `2**-1` is 0.5 there — so the axis is asked, and only when the
// exponent really is negative, because `2**3` means the same thing in all of
// them. Overflow wraps, which is what both integer-arithmetic shells do and
// what squaring modulo the word size preserves; the spec records overflow as
// unportable by construction, so nothing finer is promised.
func (sh *Runner) intPow(base, exp int) (arithNum, error) {
	if exp < 0 {
		if sh.ask(sh.sem().ArithNegativeExponentIsError, "a negative exponent") {
			return intNum(0), arithError{msg: Wording(sh.diag().ArithNegativeExponent, "exponent less than 0")}
		}
		return floatNum(math.Pow(float64(base), float64(exp))), nil
	}
	v := 1
	for b, e := base, exp; e > 0; e >>= 1 {
		if e&1 == 1 {
			v *= b
		}
		b *= b
	}
	return sh.carriedInADouble(math.Pow(float64(base), float64(exp)), v), nil
}

// compare answers the operators that yield a truth rather than a number. They
// are separated because their answer is an integer whatever they compared,
// which is measured: `1.5 < 2` is 1 and not 1. in the shell that prints a
// point after a whole float.
func (sh *Runner) compare(op string, l, r arithNum) (arithNum, bool, error) {
	if !l.float && !r.float {
		switch op {
		case "<":
			return intNum(boolInt(l.i < r.i)), true, nil
		case "<=":
			return intNum(boolInt(l.i <= r.i)), true, nil
		case ">":
			return intNum(boolInt(l.i > r.i)), true, nil
		case ">=":
			return intNum(boolInt(l.i >= r.i)), true, nil
		case "==":
			return intNum(boolInt(l.i == r.i)), true, nil
		case "!=":
			return intNum(boolInt(l.i != r.i)), true, nil
		}
		return intNum(0), false, nil
	}
	a, b := l.asFloat(), r.asFloat()
	switch op {
	case "<":
		return intNum(boolInt(a < b)), true, nil
	case "<=":
		return intNum(boolInt(a <= b)), true, nil
	case ">":
		return intNum(boolInt(a > b)), true, nil
	case ">=":
		return intNum(boolInt(a >= b)), true, nil
	case "==":
		return intNum(boolInt(a == b)), true, nil
	case "!=":
		return intNum(boolInt(a != b)), true, nil
	}
	return intNum(0), false, nil
}

// applyFloat is apply where at least one operand is a float.
//
// Division by zero is not an error here: it is an infinity, which is what both
// shells with floats produce. The integer path keeps the error, because there
// is no integer to give back.
func (sh *Runner) applyFloat(op string, l, r arithNum) (arithNum, error) {
	a, b := l.asFloat(), r.asFloat()
	switch op {
	case "+":
		return floatNum(a + b), nil
	case "-":
		return floatNum(a - b), nil
	case "*":
		return floatNum(a * b), nil
	case "/":
		return floatNum(a / b), nil
	case "**":
		// Exponentiation is a float operation in both shells that have
		// floats — `9**0.5` is 3 — so no integer-only refusal arises, and a
		// negative exponent is unremarkable here: the answer was already
		// going to be a float.
		return floatNum(math.Pow(a, b)), nil
	case "%":
		// A remainder is a float operation in one of the two shells with
		// floats — `7 % 2.5` is 2 there — and the other refuses a float here
		// the way it refuses one to a bitwise operator.
		//
		// But it refuses only the **divisor**, and that is measured rather
		// than assumed. 2026-09-16, AT&T ksh93u+ 2012-08-01:
		//
		//	7 % 2.5    invalid floating point operation
		//	7 % 2.0    invalid floating point operation
		//	2.0 % 2.0  invalid floating point operation
		//	1.5 % 1    0
		//	7.0 % 2    1
		//	-1.5 % 2   -1
		//
		// So a float dividend is truncated and the remainder is an integer
		// one — `1.5 % 1` is 0 and not the 0.5 the float operation gives.
		// Every other integer-only operator refuses a float on either side
		// there, `1 << 1.5` and `1 & 1.5` included, so this is the one
		// operand in the shell that is taken rather than questioned.
		//
		// The extent belongs to the refusal rather than to an axis of its
		// own: zsh does not refuse at all, and bash and dash have no float to
		// offer, so there is no second column that could answer it
		// differently.
		if _, err := sh.integerOperand(r, op); err != nil {
			return intNum(0), err
		}
		if l.floatKind() &&
			sh.ask(sh.sem().ArithIntegerOperatorRefusesFloat, "an integer-only operator refusing a float") {
			return sh.apply(op, intNum(l.asInt()), r)
		}
		return floatNum(math.Mod(a, b)), nil
	}
	// Everything else is defined on integers only, and what a float does to
	// it is the axis: one shell refuses and the other truncates.
	li, err := sh.integerOperand(l, op)
	if err != nil {
		return intNum(0), err
	}
	ri, err := sh.integerOperand(r, op)
	if err != nil {
		return intNum(0), err
	}
	return sh.apply(op, intNum(li), intNum(ri))
}

// integerOperand is a value where only an integer will do.
//
// `1.5 & 1` and `7 % 2.5` are the cases. ksh93 refuses them and zsh truncates,
// so the axis is asked — and only when the value really is a float, because a
// shell whose numbers are all integers never reaches the question.
func (sh *Runner) integerOperand(n arithNum, op string) (int, error) {
	if !n.floatKind() {
		// A wide value is an integer past the word, not a float, so it is
		// the saturating cast and no question.
		return n.asInt(), nil
	}
	if sh.ask(sh.sem().ArithIntegerOperatorRefusesFloat, "an integer-only operator refusing a float") {
		return 0, arithError{msg: Wording(sh.diag().ArithInvalidFloatOperation, "invalid floating point operation"), token: op}
	}
	return n.asInt(), nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// arithValueOf reads a variable as a number.
//
// An unset one is zero rather than an error. A set one whose value is not a
// number is *re-evaluated as an expression* where the dialect says so:
// `x=abc; $((x+1))` finds abc, which is unset, so 0, so 1. The shells split —
// docs/spec/grammar/arithmetic.md records the divergence — so this is not a
// choice made here but the ArithNameValueRecurses ask below.
//
// The depth bound is not decoration: `x=x` would otherwise recur forever.
func (r *Runner) arithValueOf(name string) (arithNum, error) {
	if n, err, stop := r.arithRecursionExceeded(name); stop {
		return n, err
	}
	if r.arithValueDepth == 0 {
		// The name the expression itself holds, kept for the sentence above.
		// Read here rather than passed down because every frame below this
		// one is a *value* being read again, and none of them knows how it
		// was reached.
		r.arithValueTopName = name
	}
	// The chain the cycle reading walks. Pushed for every name and not only
	// for one that recurses, because a name is not known to be on a chain
	// until something below it names it again — and popped on every way out,
	// so a sibling operand starts from where this one did.
	r.arithValueNames = append(r.arithValueNames, name)
	defer func() { r.arithValueNames = r.arithValueNames[:len(r.arithValueNames)-1] }()
	value, ok := r.getVar(name)
	if v, iset, element := r.namerefReadsAnElement(name); element {
		// A reference aimed at an **element** — `typeset -n b="a[0]"` — is
		// resolved here rather than at the store, for the reason
		// namerefReadsAnElement gives: the subscript is arithmetic, and the
		// store is initialized before the table arithmetic needs. So every
		// caller that is not an expansion has to ask, and this one did not:
		// `a=(5 7); typeset -n b=a[0]; echo $(( b ))` was **0** here and is
		// 5 in bash 5.3.20 and ksh93u+ 2012-08-01 alike, measured
		// 2026-09-17 from script files under `env -i`.
		//
		// The damage is not the read. A write through the same reference
		// already lands — `(( b = 9 ))` puts 9 in `a[0]` — so every
		// arithmetic that reads *and* writes silently used 0 as the old
		// value: `(( b += 5 ))` left 5 where both shells leave 14, and
		// `(( b++ ))` never counted. Status 0 throughout.
		//
		// Replacing the read above rather than standing in front of it, so
		// the unset questions below are asked of the element exactly as they
		// are asked of a plain name.
		value, ok = v, iset
	}
	if ok {
		if f, held := r.storedFloatValue(name, value); held {
			// A float name is holding a number, and the `-E` or `-F` letter
			// decides how that number is *written* rather than what it is.
			// Measured 2026-09-25 on zsh 5.9.2: `typeset -E3
			// f=3.14159265358979` prints `3.14e+00` and answers
			// `3.14159265358979` to `$(( f ))`, and `typeset -F1 f=$same; ((
			// f = f * 2 ))` leaves `6.3` where `$(( f ))` is
			// `6.28318530717958` — the doubling was of every digit and not
			// of the one the format printed. Reading the characters instead
			// is the whole of #4475.
			return floatNum(f), nil
		}
	}
	if !ok {
		if n, err, refused := r.arithNounsetRefusal(name); refused {
			return n, err
		}
		if r.arithSubscriptDepth > 0 &&
			r.ask(r.sem().ArithSubscriptNameMustBeSet, "an unset name inside an array subscript") {
			// The same refusal, from the other place one dialect reads a
			// name as a *parameter* rather than as text that might be a
			// number: inside the brackets of a subscript. `$(( a[b] ))` is
			// `b: parameter not set` there where `$(( b ))` is zero. See
			// Semantics.ArithSubscriptNameMustBeSet.
			text := Wording(r.diag().UnboundVariable, "%s: parameter not set", name)
			return intNum(0), arithError{msg: text, token: name, complete: true}
		}
		if r.arithValueDepth > 0 && r.ask(r.sem().ArithRecursedNameMustBeSet, "an unset name reached through a value") {
			// One dialect reads a name arrived at through another name's
			// value as a *parameter reference* rather than as text that
			// might be a number, so an unset one is the same refusal its
			// `set -u` writes — with nounset off, and fatal. See
			// Semantics.ArithRecursedNameMustBeSet.
			//
			// Only below the top: a name written in the expression itself
			// is zero when it is unset in every shell in the panel.
			text := Wording(r.diag().UnboundVariable, "%s: parameter not set", name)
			return intNum(0), arithError{msg: text, token: name, complete: true}
		}
		return intNum(0), nil
	}
	return r.arithNumOfStored(value)
}

// arithNounsetRefusal is `set -u` reaching the expression: a name an
// expression read and nothing ever set, where the dialect calls that the
// option's refusal rather than the zero arithmetic otherwise gives it.
//
// Asked here rather than beside the two nounset-off refusals below it, and in
// front of them, because it is the more general rule: those two are one shell
// reading a name as a parameter in two particular places and this is every
// name in the expression once the option is on. In front, so that a shell
// answering both writes one sentence about the name rather than two — and the
// sentence is the same either way, which is what made the order invisible
// until the fatality parted them.
//
// One place rather than one per construct, and that is the whole point of the
// fix: `$(( ))`, `(( ))`, a C-style `for` header, an array subscript, `let`
// and an assignment to an integer-declared name all read their names through
// arithValueOf, so all six refuse together. See
// Semantics.ArithUnsetNameUnderNounsetIsRefused for the panel.
//
// The refusal is returned as an ordinary arithmetic failure — the sentence is
// complete, so it is printed bare wherever the expression was written, which
// is what all three refusing columns do — and the shell is stopped separately
// where the dialect says the refusal is fatal of itself. Stopping through
// fatalExpansionQuiet rather than writing the sentence here keeps one printer
// for the failure: it is the door every `set -u` refusal already uses, which
// is also where the command-string route's own status comes from, so
// `sh -c 'set -u; : $((b))'` ends at 127 as bash does while the same script
// in a file ends at 1.
func (r *Runner) arithNounsetRefusal(name string) (arithNum, error, bool) {
	if !r.nounset {
		return intNum(0), nil, false
	}
	if !r.ask(r.sem().ArithUnsetNameUnderNounsetIsRefused, "an unset name read by an expression under `set -u`") {
		return intNum(0), nil, false
	}
	if r.ask(r.sem().ArithNounsetRefusalIsFatal, "`set -u` in an expression stopping the shell wherever it is written") {
		// Through the door every other `set -u` refusal uses, which is what
		// decides the status: 1 from a script file and 127 from a `-c`
		// string in bash. Quiet, because the sentence below is written by
		// whichever site the expression was evaluated from — one printer for
		// the failure, and the site is what knows the location to write in
		// front of it.
		r.fatalExpansionQuiet()
		r.arithNounsetNamedTheParameter = true
	}
	text := Wording(r.diag().UnboundVariable, "%s: parameter not set", name)
	return intNum(0), arithError{msg: text, token: name, complete: true}, true
}

// arithRecursionExceeded is the bound on reading a stored value as an
// expression, and reports whether the read must stop here.
//
// Its own function because **two readers recurse and only one of them asked**.
// A name's value is read again through arithValueOf, which has always checked
// this; an *element's* went straight to arithNumOfStored, so a value that
// names its own element recursed with nothing counting the frames: measured on
// origin/main 5083d624d, `a=('a[0]'); echo $(( a[0] ))` is a **Go stack
// overflow that kills the shell**, where bash 5.3.20 answers `a[0]:
// expression recursion level exceeded` and ksh93u+ `a[0]: recursion too deep`,
// both reporting and carrying on.
//
// Which name the bound is reported against is itself a divergence: bash and
// ksh93 name the one it stopped on and zsh the one the expression was written
// with, which are the same name only when the value points at itself. See
// Diagnostics.ArithRecursionBlamesTheWrittenName.
//
// **What the bound counts is itself a divergence**, which is why there are two
// conditions below and not one. Three columns count frames; one follows the
// chain and looks for a name that has come back on itself, so a chain of three
// hundred distinct names is a value there and a fatal error in the other three.
// See Semantics.ArithRecursionBound.
func (r *Runner) arithRecursionExceeded(blamed string) (arithNum, error, bool) {
	// A name already on the chain is a loop; a chain past the frame count is
	// deep. Neither puts a question to the dialect, so an ordinary `$(( a+b ))`
	// — where every name is looked at and none recurses — never asks.
	loop := r.arithValueDepth > 0 && slices.Contains(r.arithValueNames, blamed)
	deep := r.arithValueDepth > 32
	if !loop && !deep {
		return intNum(0), nil, false
	}
	if r.arithRecursionBound() == ArithRecursionBoundedByACycle {
		if !loop {
			// Deep and going somewhere. The chain is followed, which is the
			// whole of the other reading.
			return intNum(0), nil, false
		}
		// The sentence names nothing in this column, so nothing is blamed:
		// `expression recursion loop detected` and no token, measured
		// 2026-09-18 against every one of the four loop shapes.
		return intNum(0), arithError{
			// The empty name is passed rather than left out: a dialect whose
			// sentence carries a verb must produce an empty subject rather
			// than fmt's own complaint about a missing argument.
			msg: Wording(r.diag().ArithRecursionLimit, "expression nested too deeply", ""),
		}, true
	}
	if !deep && r.sem().ArithRecursionBound == ArithRecursionBoundedByDepth {
		// A loop that has not yet reached the frame count. The shells that
		// count frames reach it a few frames later and blame the name they
		// stopped on, so nothing is refused here — which is what keeps
		// `a=b; b=a` blaming `b` rather than the name the cycle closed on.
		//
		// An axis with **no answer** does not take this road: it stops at the
		// first sign of either bound, so the complaint arithRecursionBound has
		// just written is written once rather than once per frame.
		return intNum(0), nil, false
	}
	if r.diag().ArithRecursionBlamesTheWrittenName && r.arithValueTopName != "" {
		blamed = r.arithValueTopName
	}
	return intNum(0), arithError{
		msg:   Wording(r.diag().ArithRecursionLimit, "expression nested too deeply: %[1]s", blamed),
		token: blamed,
	}, true
}

// arithNumOfStored reads a value a variable was holding as a number.
//
// Split out of arithValueOf so that an *element* is read the same way a plain
// name is. It was not: an element went straight to the literal parser, so
// `a=(y); y=5; echo $(( a[0] ))` refused where all three shells with arrays
// answer 5, and an element holding a word that is no name at all refused with
// a different sentence from the identical scalar. One operand rule, asked
// once — the storage it came out of is not what decides how it reads.
func (r *Runner) arithNumOfStored(value string) (arithNum, error) {
	if strings.TrimSpace(value) == "" {
		return intNum(0), nil
	}
	value = r.decimalLeadingNumeral(value)
	if n, err := r.parseArithStored(strings.TrimSpace(value)); err == nil {
		return n, nil
	}
	return r.arithValueAsExpression(value)
}

// arithValueAsExpression reads a stored value that is no numeral by parsing it
// again as an expression, where the dialect does that.
//
// `v=1+1; $(( v * 3 ))` is 6 in bash, ksh93 and zsh, and dash alone refuses
// it — the same split ArithNameValueRecurses already records, which is why
// this asks that axis rather than a second one beside it. A name-shaped value
// is the case the axis was written for and is not a case at all here: `x=y`
// parses as the expression `y`, so the ordinary walk looks `y` up and the
// recursion, the unset-name question and the depth bound are all the ones a
// name written in the expression itself gets. Reading a name and reading an
// expression were two helpers with one rule between them, and the one that
// only knew names refused `1+1`.
//
// The value is parsed as it stands, without expansion: a `$` in it is an
// ordinary character, which is why `q=5; v='$q'` is a syntax error and not 5
// in all three shells that re-read at all.
func (r *Runner) arithValueAsExpression(value string) (arithNum, error) {
	text := strings.TrimSpace(value)
	p := syntax.NewParser("", r.dialect())
	// A stored value has been through its expansions already, so a `$` in it
	// is a character of the result — which is what the doc above says and
	// what the reader that waits for expansions could not be told. It
	// mattered for the subscript inside one: `m[k]=5; key=k; f='m[$key]';
	// $(( f ))` is 5 in bash 5.3.20, bash 3.2.57, zsh 5.9.2 and ksh93u+, and
	// was a silent 0 here because a nil tree with no complaint evaluates to
	// nothing at all (#3303).
	tree := p.ParseArithExpanded(value, syntax.Pos{})
	err := unmarkArithFailure(p.Err())
	if err == nil && tree == nil {
		// No tree and no complaint means the text holds an expansion, which
		// the parser leaves for its caller to substitute first. A stored
		// value has already been through that once and does not go through
		// it again, so what is left is a `$` standing in an expression as an
		// ordinary character — an operand failure, which is what the shells
		// that re-read at all report for it.
		err = &syntax.Error{Kind: syntax.ErrArithOperand, Expr: text, Token: text}
	}
	if err != nil {
		if r.sem().ArithNameValueRecurses == Yes {
			// The value became an expression and that expression would not
			// parse, which is the failure a written one earns, worded the
			// same way and blaming the value rather than the name it came
			// out of: `v="3 4"; $(( v ))` names `3 4`.
			return intNum(0), arithError{msg: r.expressionFailure(text, err), complete: true}
		}
		// Nowhere for it to be an expression, so it is simply not a number.
		// No ask: every dialect refuses this text and only the sentence
		// differs, so a question here would be one asked where the panel
		// agrees.
		return intNum(0), arithError{msg: r.wordInvalidNumber(text), token: text, badNumeral: true, complete: true}
	}
	if !r.ask(r.sem().ArithNameValueRecurses, "re-reading a stored value as an expression") {
		return intNum(0), arithError{msg: r.wordInvalidNumber(text), token: text, badNumeral: true, complete: true}
	}
	r.arithValueDepth++
	defer func() { r.arithValueDepth-- }()
	n, err := r.evalNum(tree)
	if err != nil {
		// The complaint names the *value*, because that is the expression
		// that failed: `v=1/0; $(( v ))` is `1/0: division by 0` and not
		// `v: division by 0`, measured on bash 5.3.15 and ksh93u+ alike.
		// Already-complete failures are handed back as they stand, which is
		// what arithFailure does with them.
		return intNum(0), arithError{msg: r.arithFailure(text, err), complete: true}
	}
	return n, nil
}

// decimalLeadingNumeral answers a stored value with its leading zeros taken
// off, where the dialect reads one that way — see
// Semantics.ArithStoredValueReadsALeadingZeroAsDecimal. Everything else is
// returned unchanged.
//
// A rewrite rather than a number, because the value may be a whole expression
// and only what stands in front of it is read this way: measured 2026-09-11 on
// ksh93u+, `k=010+1` is 11 where the identical literal `$((010+1))` is 9.
//
// Asked only where the two readings can differ: the value has to *begin* with
// a zero that something follows, with nothing before it.
//
//	k=010      10   the digits, in decimal
//	k=0010     10   however many zeros
//	k=09        9   an invalid octal digit is just a digit
//	k=010+1    11   the leading run only; `k=1+010` is 9
//	k=010#5     5   the numeral is the base, and in decimal
//	k=-010     -8   a sign is not part of it, so nothing is rewritten
//	k=" 010"    8   nor is anything in front of it
//	k=0abc      5   with abc=5: the zeros go in front of a name too
//	k=0b101     9   with b101=9, which is why this is not a rule about digits
//	k=0x10     16   one zero in front of an x is a hex prefix and survives
//	k=00x10     7   with x10=7: two zeros are not a prefix, so both go
//
// The last two rows are the whole of what hexPrefixSurvives is for, and they
// are the one thing this reader and a *condition operand's* disagree about —
// see conditionLeadingNumeral.
func (r *Runner) decimalLeadingNumeral(value string) string {
	return r.leadingZerosOff(value, true)
}

// conditionLeadingNumeral is decimalLeadingNumeral at the other site: the
// operand of a word-spelled comparison, where nothing survives the zeros.
//
// Measured 2026-09-12 on ksh93u+, which is the only column that takes any of
// this — the reading is the same one, asked the same way, and only the hex
// prefix parts company:
//
//	                      [ … -eq ]   k=…; $(( k ))
//	0x10, with x10=7          7            16
//	0x10, with x10 unset      0            16
//	0xg, with xg=9            9            arithmetic syntax error
//	00x10, with x10=7         7             7
//	010                      10            10
//
// So a condition operand's `0x` is not a prefix at all: `[[ 0x10 -eq 16 ]]`
// is false there while `[[ 1+0x10 -eq 17 ]]` holds, because the second has
// nothing in front of the zero for the rewrite to reach (#1627).
func (r *Runner) conditionLeadingNumeral(value string) string {
	return r.leadingZerosOff(value, false)
}

// leadingZerosOff is the reading both sites share. hexPrefixSurvives keeps a
// single leading zero in front of an `x` or `X`, which is what tells a stored
// value's reader from a condition operand's.
func (r *Runner) leadingZerosOff(value string, hexPrefixSurvives bool) string {
	if r.sem().ArithLeadingZeroIsOctal != Yes {
		// Nothing made the zero octal, so the two readings already agree and
		// there is no choice to put to the dialect.
		return value
	}
	n := leadingZeroRun(value, hexPrefixSurvives)
	if n == 0 {
		return value
	}
	if !r.ask(r.sem().ArithStoredValueReadsALeadingZeroAsDecimal,
		"a zero-padded number read out of a variable") {
		return value
	}
	// A value that is nothing but zeros keeps them — leadingZeroRun says so
	// by answering nothing to strip — so what is left here is never empty.
	return value[n:]
}

// leadingZeroRun is how many leading `0` bytes the value opens with, when
// something follows them — the one shape the two readings answer differently.
// Zero means there is nothing to rewrite.
//
// hexPrefixSurvives excludes exactly one zero standing in front of an `x` or
// an `X`, which is a radix prefix to the reader that keeps it. Two zeros are
// not: `00x10` is the name `x10` in every reader measured.
func leadingZeroRun(value string, hexPrefixSurvives bool) int {
	n := 0
	for n < len(value) && value[n] == '0' {
		n++
	}
	if n == 0 || n == len(value) {
		return 0
	}
	if hexPrefixSurvives && n == 1 && (value[1] == 'x' || value[1] == 'X') {
		return 0
	}
	return n
}

// parseArithNum reads a numeral written in the expression itself.
//
// See readArithNum: the two readers part over a numeral too large for a
// double, and only there.
func (r *Runner) parseArithNum(s, tail string) (arithNum, error) {
	return r.readArithNum(s, tail, true)
}

// parseArithStored reads a numeral that came out of a variable.
//
// The counterpart of parseArithNum, and the reason readArithNum takes the
// site at all: ksh93's two readers give an overflowed numeral zeros of
// opposite sign. See Semantics.ArithFloatOverflowIsZero.
func (r *Runner) parseArithStored(s string) (arithNum, error) {
	// No tail: a numeral that stood in a variable was written nowhere, so the
	// one diagnostic that quotes an expression's tail has nothing but the
	// digits — which is what the shell that writes it does. Measured
	// 2026-09-16: `n=9223372036854775808; $(( n ))` quotes the digits alone.
	return r.readArithNum(s, "", false)
}

// readArithNum reads a literal, which may be a float where the dialect has
// them.
//
// The axis is asked only when the text is float-shaped. An expression of whole
// numbers means the same thing in every shell in the panel, so `3/2` needs no
// dialect and `3.0/2` does.
//
// written says the text stood in the expression rather than in a variable the
// expression named, which decides the sign of the zero an overflow comes to
// where an overflow comes to zero at all.
func (r *Runner) readArithNum(s, tail string, written bool) (arithNum, error) {
	s = strings.TrimSpace(s)
	if r.lang().ArithDigitSeparator {
		// The separator is removed and then the ordinary rules apply to what
		// is left, which is the whole of that rule — see
		// syntax.Dialect.ArithDigitSeparator. Here rather than only in the
		// parser because a value a *variable* was holding reaches this with
		// no parser having seen it: `x=1_0; $(( x ))` is 10 on the shell that
		// has the separator. The dialect answers both sites for the reason
		// the float question below does: one question, asked where it is
		// needed, rather than two fields that could disagree.
		s = strings.ReplaceAll(s, "_", "")
	}
	if f, ok := hexFloatNumeral(s); ok && r.lang().ArithHexFloat {
		// `0x1p4` is 16 and `0x1.8` is 1.5 in the one column that reads the
		// spelling C has for a float written in hexadecimal. The other five
		// refuse it, each in its own words, which is what makes this an axis
		// and not a reader everyone should have had.
		return floatNum(f), nil
	}
	if !floatShaped(s) {
		n, err := r.parseNum(s)
		if err != nil {
			var ae arithError
			if errors.As(err, &ae) && ae.pastTheWord {
				if r.lang().ArithFloat &&
					r.ask(r.sem().ArithValuesAreCarriedInAFloat, "arithmetic carried in a float rather than the machine word") {
					// A numeral with an explicit radix is read in the
					// **unsigned** word first, and only becomes the double
					// when that overflows. `$(( 0xffffffffffffffff ))` is -1
					// and `$(( 01777777777777777777777 ))` is -1, where a
					// plain decimal of the same magnitude is
					// 1.84467440737096e+19 — so the radix is the whole of
					// what parts them (#3257).
					//
					// The value goes back through carriedInADouble like every
					// other integer, which is what makes `0x8000000000000001`
					// -9223372036854775808 rather than -9223372036854775807:
					// the unsigned word holds it exactly and the double this
					// shell stores it in does not.
					if v, ok := unsignedWordNumeral(ae.digits, ae.numBase); ok && ae.numBase != 10 {
						if negativeNumeral(s) {
							v = -v
						}
						return r.carriedInADouble(float64(v), int(v)), nil
					}
					// Past the unsigned word too, or a plain decimal, which
					// never takes that route at all. What is left is C's
					// `strtod` over the whole numeral as it was written —
					// `$(( 10000000000000000000 ))` is 1e+19 and
					// `$(( 0xffffffffffffffffff ))` is 4.72236648286965e+21.
					return floatNum(floatNumeral(s)), nil
				}
				if !written && r.ask(r.sem().ArithStoredNumeralPastTheWordIsRefused,
					"a stored numeral past the machine word") {
					// dash parts its two number readers here and nothing
					// else does: the same digits saturate where they stand
					// in the expression and are refused where a variable
					// held them. Asked before the reading below, so the one
					// column that says something about a truncation does not
					// say it for a numeral that is about to be refused.
					return intNum(0), ae
				}
				// The other three readings, none of which is a refusal: the
				// word goes round, or part of the numeral is read and said
				// so, or the largest value stands. Refusing outright was this
				// shell's own fourth answer and no column has it (#3202).
				if v, ok := r.numeralPastTheWord(ae.digits, ae.numBase, tail); ok {
					return intNum(int(v)), nil
				}
				return intNum(0), arithError{
					msg:   r.unanswered("an integer numeral past the machine word"),
					token: s, badNumeral: true, complete: true,
				}
			}
			return intNum(n), err
		}
		// The numeral goes through the same carriage every result does, so a
		// written-down 9007199254740993 is the same value as one arrived at
		// by arithmetic. See carriedInADouble.
		return r.carriedInADouble(float64(n), n), nil
	}
	if !r.lang().ArithFloat {
		// The dialect has no floats, so this is not a number at all. It is
		// the *grammar* that answers — the parser would not have produced a
		// float literal here either — which is why this reads the dialect
		// rather than an axis: one question, asked in the two places that
		// need it, rather than two fields that could disagree.
		msg := r.wordInvalidNumber(s)
		if w := r.diag().DigitTooGreatForBase; w != "" {
			// The dialect that calls every unreadable literal the same
			// thing says it here too: `1e3` fails with the octal digit's
			// own sentence.
			msg = w
		}
		return intNum(0), arithError{msg: msg, token: s, badNumeral: true}
	}
	f, err := strconv.ParseFloat(s, 64)
	if errors.Is(err, strconv.ErrRange) {
		// The numeral is well formed and too large: ParseFloat has already
		// read it and saturated to an infinity, and handed back the value
		// *and* the error. Discarding both is what refused `$((1e400))`,
		// which both float columns answer. An underflow is not this — Go
		// reports no error for one, and both columns answer zero anyway.
		if r.ask(r.sem().ArithFloatOverflowIsZero, "a float numeral too large for a double") {
			return floatNum(overflowZero(written)), nil
		}
		return floatNum(f), nil
	}
	if err != nil {
		return intNum(0), arithError{msg: r.wordInvalidNumber(s), token: s, badNumeral: true}
	}
	return floatNum(f), nil
}

// overflowZero is the zero a numeral too large for a double comes to, in the
// dialect where it comes to a zero at all.
//
// The sign is measured and is not decoration: `$((1e400))` writes `-0` and
// `$((-1e400))` writes `0`, which is one negative zero with the unary minus
// applied to it, and a value read out of a variable is the positive one. See
// Semantics.ArithFloatOverflowIsZero for the table.
func overflowZero(written bool) float64 {
	if written {
		return math.Copysign(0, -1)
	}
	return 0
}

// unsignedWordNumeral reads a digit run in the **unsigned** machine word,
// reporting false where it does not fit rather than letting it go round.
//
// The refusal is the point, and it is what parts this from wrappedNumeral one
// file along. Two shells read a numeral past the signed word in the unsigned
// one; only one of them stops there. `$(( 01777777777777777777777 ))` is 2^64-1
// and is -1 in bash and in ksh93 alike, and `$(( 02000000000000000000000 ))` is
// 2^64 and goes round to 0 in bash while ksh93 abandons the word and reads the
// numeral as a double instead — 2e+21, because the reader it falls back to has
// never heard of octal. Measured 2026-09-16 against AT&T ksh93u+ 2012-08-01.
func unsignedWordNumeral(digits string, base int) (int64, bool) {
	if digits == "" {
		return 0, false
	}
	var v uint64
	for i := 0; i < len(digits); i++ {
		d, known := baseDigitValue(digits[i], base)
		if !known || d >= base {
			return 0, false
		}
		nv := v*uint64(base) + uint64(d)
		if nv/uint64(base) != v || nv < uint64(d) {
			// The multiply or the add carried past the word. Checked rather
			// than inferred from the result shrinking: a base that is not a
			// power of two can wrap to a value larger than the one before it.
			return 0, false
		}
		v = nv
	}
	return int64(v), true
}

// negativeNumeral reports a numeral written with a leading minus, which
// parseNum strips before the conversion and never gets to apply when the
// conversion fails.
func negativeNumeral(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && s[0] == '-'
}

// floatNumeral is an integer numeral the shell has given up reading as an
// integer, read as the double it comes to.
//
// It is C's `strtod` and deliberately nothing more, because the one shell that
// reaches it falls back to exactly that function. So a hexadecimal numeral is
// the hexadecimal float — Go reads one once it is given the binary exponent C
// lets it leave off — and everything else is **decimal**, whatever a leading
// zero or a `base#` prefix would have meant to the integer reader that just
// failed.
//
// The octal row is the one that says so out loud, and it is measured:
// `$(( 02000000000000000000000 ))` in ksh93u+ is `2e+21` and not
// 1.84467440737096e+19, so the fallback read the whole string in base ten with
// its leading zero along for the ride. Reading it as octal here is what this
// shell used to do (#3257).
func floatNumeral(s string) float64 {
	s = strings.TrimSpace(s)
	if f, ok := hexFloatNumeral(s); ok {
		return f
	}
	body, neg := s, false
	if body != "" && (body[0] == '-' || body[0] == '+') {
		neg = body[0] == '-'
		body = body[1:]
	}
	if strings.HasPrefix(body, "0x") || strings.HasPrefix(body, "0X") {
		// A hexadecimal integer, which hexFloatNumeral above declines
		// because it has neither a point nor an exponent. `strtod` reads it
		// as the hexadecimal float with an exponent of zero.
		f, err := strconv.ParseFloat(body+"p0", 64)
		if err != nil {
			return 0
		}
		if neg {
			return -f
		}
		return f
	}
	f, err := strconv.ParseFloat(body, 64)
	if err != nil {
		return 0
	}
	if neg {
		return -f
	}
	return f
}

// hexFloatNumeral reads the hexadecimal float spelling C has, and reports
// whether the literal was one at all.
//
// Measured 2026-09-16 against AT&T ksh93u+ 2012-08-01, which is the only panel
// column that reads it — bash 5.3, bash-as-sh, bash 3.2, zsh 5.9.2, dash
// 0.5.12 and BusyBox ash 1.37.0 all refuse every row below:
//
//	0x1p4      16      a binary exponent, which is what makes it a float
//	0x1P4      16      either case of the exponent letter
//	0x1p-1     0.5     and a signed one
//	0x1p+2     4
//	0xffp0     255     an exponent of zero is still the float spelling
//	0x1.8p1    3       a point as well
//	0x1.8      1.5     or a point alone, with no exponent at all
//	0x1.p1     2       the digits after the point may be missing
//	0x1e5      485     but `e` is a hex *digit* here, so this is an integer
//
// Two shapes are refused and both are about a missing digit rather than a
// missing exponent: `0x.8p1` and `0xp4`, which have no digit between the `0x`
// and the point or the `p`. An exponent whose digits are missing is not
// refused — `0x1p`, `0x1p+` and `0x1.8p` are 1, 1 and 1.5 — so the letter is
// consumed and the absent exponent read as zero.
//
// Go's reader wants the exponent this spelling may leave off, so it is put
// back before the literal is handed over. That is the whole of the difference.
func hexFloatNumeral(s string) (float64, bool) {
	body := s
	neg := false
	if body != "" && (body[0] == '-' || body[0] == '+') {
		neg = body[0] == '-'
		body = body[1:]
	}
	if len(body) < 3 || body[0] != '0' || (body[1] != 'x' && body[1] != 'X') {
		return 0, false
	}
	digits := body[2:]
	if !isHexDigit(digits[0]) {
		// `0x.8p1` and `0xp4` are refused, so the mantissa wants a digit of
		// its own before anything else may follow.
		return 0, false
	}
	e := strings.IndexAny(digits, "pP")
	if e < 0 && !strings.Contains(digits, ".") {
		// An ordinary hexadecimal integer, `e` and all.
		return 0, false
	}
	switch {
	case e < 0:
		body += "p0"
	case e == len(digits)-1:
		body += "0"
	case digits[e+1] == '+' || digits[e+1] == '-':
		if e+2 == len(digits) {
			body += "0"
		}
	}
	f, err := strconv.ParseFloat(body, 64)
	if err != nil {
		return 0, false
	}
	if neg {
		return -f, true
	}
	return f, true
}

// floatShaped reports whether a literal can only be a float.
//
// A point or an exponent, and not a hex literal — `0x1e5` is an integer whose
// digits happen to include an `e`, and `16#1f` is one whose base separator is
// not a point.
func floatShaped(s string) bool {
	if s == "" || strings.ContainsAny(s, "#") || strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return false
	}
	if strings.Contains(s, ".") {
		return true
	}
	i := strings.IndexAny(s, "eE")
	// An exponent needs digits before it, or it is a name: `e5` is a
	// variable and `1e5` is a number.
	return i > 0 && strings.IndexFunc(s[:i], func(c rune) bool { return c < '0' || c > '9' }) < 0
}

// formatNum writes a value the way the dialect writes one.
//
// Integers are the same everywhere. Floats are not: the two shells that have
// them disagree on how many digits to show and on whether a whole one keeps
// its point, so both are the dialect's to answer.
func (r *Runner) formatNum(n arithNum) string {
	if !n.float {
		return itoa(n.i)
	}
	// An infinity and a NaN are named rather than formatted, and each shell
	// names them its own way.
	switch {
	case math.IsInf(n.f, 1):
		return Wording(r.diag().ArithInfinity, "+Inf")
	case math.IsInf(n.f, -1):
		return "-" + Wording(r.diag().ArithInfinity, "Inf")
	case math.IsNaN(n.f):
		return Wording(r.diag().ArithNotANumber, "NaN")
	}
	digits := r.diag().ArithFloatDigits
	if digits == 0 {
		digits = 17
	}
	out := strconv.FormatFloat(n.f, 'g', digits, 64)
	if i, fits := intFromDouble(n.f); fits && itoa(i) != out &&
		r.ask(r.sem().ArithValuesAreCarriedInAFloat, "arithmetic carried in a float rather than the machine word") {
		// The shell whose every arithmetic value is a C double writes one as
		// an **integer** whenever a saturating `(intmax_t)` cast of it
		// converts back to the same double, and in floating notation when it
		// does not. That is the rule carriedInADouble already applies to an
		// integer result, and it is the same rule here because it was never
		// about where the value came from: `$(( 1e19/3 ))` in ksh93u+ is
		// 3333333333333333504 and `$(( 1e19 ))` is 1e+19, from a division of
		// two floats and a float literal — neither of them an integer
		// operation at all (#3257).
		//
		// Asked only where the two readings disagree, which is why the `%g`
		// text is produced first and compared: a float that is already
		// written as its own integer — `$(( 1.5 + 1.5 ))` is `3` in every
		// column that has floats — is not a question, and an axis asked
		// there would report itself unanswered for a shell that never
		// carries anything in a double.
		return itoa(i)
	}
	if r.diag().ArithFloatKeepsPoint && !strings.ContainsAny(out, ".eEnif") {
		// A whole float still reads as one: 4 becomes `4.`. Skipped when the
		// text already carries a point, an exponent, or is an infinity or a
		// NaN, none of which could be mistaken for an integer.
		out += "."
	}
	return out
}

func isNameLike(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || isLetter(c) || (i > 0 && isDigit(c)) {
			continue
		}
		return false
	}
	return true
}

// parseNum reads a literal, which the parser deliberately kept as written.
//
// Whether a leading zero means octal is a dialect question — zsh reads
// `0100` as one hundred where everything else reads sixty-four — so the
// answer is given here, where the dialect is, rather than baked into the tree.
func (r *Runner) parseNum(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, arithError{msg: "empty number"}
	}
	neg := false
	if s[0] == '-' || s[0] == '+' {
		neg = s[0] == '-'
		s = s[1:]
	}

	var n int64
	var err error
	// Why the literal is not a number, which is worded separately from the
	// fact that it isn't: bash calls a bad octal digit "value too great for
	// base" and reserves its generic wording for operands that are not
	// literals at all.
	// badDigit says the literal parsed as far as its base and then met a
	// digit that base does not allow. bash words that separately from an
	// operand that is not a literal at all, and wraps it where it reports
	// the other bare.
	// digits is what the conversion actually read, and base what it read them
	// in. Both are kept so that a failure can be blamed the way bash blames
	// one: it parts a digit the base cannot reach from a byte that is no
	// digit at all, and only the pair says which happened.
	digits, base := s, 10
	switch {
	case r.spellsANamedBase(s):
		text, rest, _ := strings.Cut(s, "#")
		b, _ := strconv.Atoi(text)
		if b == 0 && r.ask(r.sem().ArithBaseZeroReadsTheDigitsAsWritten,
			"a base of zero") {
			// Zero is a base one dialect reads *through*: the digits after
			// the `#` are read as an ordinary constant, so `0#5` is 5 and
			// `0#0x10` is 16. Read again from the top rather than in base
			// ten, because the radix prefix counts there too.
			return r.parseNum(rest)
		}
		if b < 2 || b > 64 {
			// Below two is no base at all and above 64 is past the alphabet.
			// Worded the same way as a base the dialect stops short of,
			// because it is the same refusal: bash writes `invalid
			// arithmetic base` for `1#0` and ksh93 its one sentence.
			return 0, arithError{msg: Wording(r.diag().ArithInvalidBase,
				"invalid base: %[1]s", strconv.Itoa(b)), token: s, badNumeral: true}
		}
		if b > 36 && !r.ask(r.sem().ArithBaseAbove36, "a base above 36") {
			if r.unspecified {
				return 0, arithError{msg: r.unanswered("a base above 36")}
			}
			// One dialect stops at 36 and says so, naming the base — as a
			// number rather than as written, which is what it prints for a
			// padded one: `064#10` is `invalid base (must be 2 to 36
			// inclusive): 64` there.
			return 0, arithError{msg: Wording(r.diag().ArithInvalidBase,
				"invalid base: %[1]s", strconv.Itoa(b)), token: s, badNumeral: true}
		}
		digits, base = rest, b
		var bad bool
		if n, bad = parseBaseDigits(digits, base); bad {
			err = strconv.ErrSyntax
		}
	case strings.HasPrefix(s, "0x"), strings.HasPrefix(s, "0X"):
		digits, base = s[2:], 16
		n, err = r.parseRadixDigits(digits, base)
	case r.lang().ArithBinaryLiteral &&
		(strings.HasPrefix(s, "0b") || strings.HasPrefix(s, "0B")):
		digits, base = s[2:], 2
		n, err = r.parseRadixDigits(digits, base)
	case len(s) > 1 && s[0] == '0' && !strings.ContainsAny(s, "xX") && r.octalLeadingZero():
		digits, base = s[1:], 8
		n, err = strconv.ParseInt(digits, 8, 64)
		if errors.Is(err, strconv.ErrSyntax) &&
			!r.ask(r.sem().ArithInvalidOctalDigitIsError, "an invalid octal digit being an error") {
			// ksh93 is octal *and* tolerant: `08` is 8 there, not a
			// failure. Asked only once the octal read has actually failed,
			// so a dialect that never sees a bad digit is never questioned.
			//
			// A *digit* the base cannot use, and not a numeral too large for
			// the word. The two used to be one condition, and the second
			// reading threw the octal away: `01777777777777777777777` is
			// 2^64-1 written in octal and was re-read in base ten, so the
			// numeral this shell then had was 1.77777777777778e+21 where
			// ksh93 answers -1 (#3257).
			digits, base = s, 10
			n, err = strconv.ParseInt(digits, 10, 64)
		}
	default:
		n, err = strconv.ParseInt(digits, base, 64)
	}
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			// Well formed and too large. The reader that refuses it is
			// alone on the panel, so the numeral is handed back as a
			// failure the caller may still answer — see evalNumeral.
			return 0, arithError{
				msg: r.wordInvalidNumber(s), token: s,
				digits: digits, numBase: base,
				badNumeral: true, complete: true, pastTheWord: true,
			}
		}
		if w := r.diag().DigitTooGreatForBase; w != "" {
			// Three dialects word every unreadable literal through the same
			// wrapper, and one of them parts two diagnoses inside it: a
			// digit the base cannot reach — `08`, `2#12`, and `1@2` whose
			// `@` is digit 62 — from a byte that is no digit anywhere, which
			// is the `#` left behind when a leading zero made the text an
			// octal constant. See Diagnostics.ArithByteIsNoDigit.
			if _, isDigit, found := firstByteTheBaseCannotUse(digits, base); found && !isDigit {
				if n := r.diag().ArithByteIsNoDigit; n != "" {
					w = n
				}
			}
			return 0, arithError{msg: w, token: s, badNumeral: true}
		}
		return 0, arithError{msg: r.wordInvalidNumber(s), token: s, badNumeral: true, complete: true}
	}
	if neg {
		n = -n
	}
	return int(n), nil
}

// parseBaseDigits reads digits in a base up to 64. The second result reports
// a digit the base cannot use, which each dialect words its own way.
func parseBaseDigits(digits string, base int) (int64, bool) {
	if digits == "" {
		return 0, true
	}
	var n int64
	for i := 0; i < len(digits); i++ {
		v, known := baseDigitValue(digits[i], base)
		if !known || v >= base {
			return 0, true
		}
		n = n*int64(base) + int64(v)
	}
	return n, false
}

// baseDigitValue is the base-64 alphabet, in one place: 0-9, then letters —
// one case as good as the other through 36, and apart above it, where a-z is
// 10..35, A-Z 36..61, `@` 62 and `_` 63.
//
// The second result says the byte is a digit *somewhere* in that alphabet,
// even where this base cannot reach it. That is the distinction a complaint
// turns on and the reason the two questions share one function: a digit the
// base does not have and a byte that is no digit at all are two diagnoses,
// and an alphabet written down twice is the way they stop agreeing.
func baseDigitValue(c byte, base int) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'Z':
		if base <= 36 {
			return int(c-'A') + 10, true
		}
		return int(c-'A') + 36, true
	case c == '@':
		return 62, true
	case c == '_':
		return 63, true
	}
	return 0, false
}

// firstByteTheBaseCannotUse finds what stopped a conversion: the byte, whether
// the alphabet knows it as a digit at all, and whether there was one.
func firstByteTheBaseCannotUse(digits string, base int) (byte, bool, bool) {
	for i := 0; i < len(digits); i++ {
		v, known := baseDigitValue(digits[i], base)
		if !known || v >= base {
			return digits[i], known, true
		}
	}
	return 0, false, false
}

// spellsANamedBase reports whether the text is the `base#digits` form *as this
// dialect spells it*, rather than a numeral that happens to hold a `#`.
//
// Where it is not, the text falls through to the ordinary numeral reading and
// the `#` is simply a byte no base can use — which is not a fallback but
// bash's whole rule: a leading zero opens an octal constant there, so
// `010#5` is the octal `010` with `#5` behind it and fails as a number.
func (r *Runner) spellsANamedBase(s string) bool {
	text, _, ok := strings.Cut(s, "#")
	if !ok || text == "" || !allDecimalDigits(text) {
		// Only decimal digits name a base in any shell in the panel:
		// `0x10#5` is a hex literal with a `#` after it, not base sixteen of
		// something.
		return false
	}
	if text[0] == '0' && !r.ask(r.sem().ArithBaseMayHaveALeadingZero,
		"a base written with a leading zero") {
		return false
	}
	if len(text) > 2 && r.ask(r.sem().ArithBaseIsAtMostTwoDigits,
		"a base longer than two characters") {
		return false
	}
	return true
}

func allDecimalDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// parseRadixDigits reads the digits after a radix prefix, where an empty run
// is a question rather than a failure: `$(( 0x ))` is zero in bash and zsh and
// refused in ksh93 and dash — see Semantics.ArithEmptyRadixDigitsAreZero.
func (r *Runner) parseRadixDigits(digits string, base int) (int64, error) {
	if digits == "" {
		if r.ask(r.sem().ArithEmptyRadixDigitsAreZero, "a radix prefix with no digits after it") {
			return 0, nil
		}
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseInt(digits, base, 64)
}

// octalLeadingZero is the dialect answer, and the quietest divergence
// measured: `0100` is sixty-four everywhere but zsh, where it is one hundred.
func (r *Runner) octalLeadingZero() bool {
	return r.ask(r.sem().ArithLeadingZeroIsOctal, "a leading zero meaning octal")
}

// arithCmd runs `(( expr ))` as a command.
//
// It exits 0 when the expression is non-zero, which is the reverse of the
// usual convention and is unanimous across the panel.
func (r *Runner) arithCmd(ctx context.Context, c *syntax.ArithCmdClause) error {
	return r.withRedirs(ctx, c.Redirs, func() error {
		r.unspecified = false
		// The expression's text is read again here, so what a substitution
		// in it reports is placed from the construct's line rather than from
		// the text's own first. See Runner.inArithCommandText (#3810).
		putBackLine := r.inArithCommandText(c.Pos())
		tree, text, perr := r.arithTreeOver(c.Parsed, c.Expr, arithTextWritten)
		putBackLine()
		// Traced from the expanded text and after the expansion, which is
		// where the shells put it: `(( $(echo 1) ))` traces the substitution
		// first and then `((  1  ))`. Before the parse, so an expression that
		// will not parse is still reported as having been reached — a trace
		// that skips what failed is the gap #2126 is about.
		r.traceArithCommand(text, r.diag().TraceArithCommand)
		if perr != nil {
			r.diagf("%s\n", r.diag().arithConstructFailure("((", r.diag().ParseFailure(perr)))
			r.status = r.arithCmdFailed(r.diag().StatusForParseError(perr))
			return nil
		}
		r.arithCommand++
		outerConstruct := r.arithConstruct
		r.arithConstruct = "(("
		v, err := r.evalArithTruth(tree)
		r.arithConstruct = outerConstruct
		r.arithCommand--
		if r.unspecified {
			r.status = 2
			return nil
		}
		if errors.Is(err, errReadonlyRefusedInACommand) {
			r.refusedInACommand = false
			r.status = r.arithCmdFailed(1)
			return nil
		}
		if err != nil {
			if r.badSubscript {
				// A **subscript's** failure is not the construct's, and no
				// column words it as one: see
				// Semantics.BadSubscriptEscapesAnArithmeticCommand, where
				// `(( b c ))` is the control that keeps the prefix.
				r.status = r.arithCommandBadSubscript(r.arithFailure(text, err))
				return nil
			}
			if r.arithNounsetNamedTheParameter {
				// `set -u` naming a name the expression read is not the
				// construct's failure and no column words it as one:
				// measured 2026-09-18, `set -u; (( b ))` is `b: unbound
				// variable` in bash 5.3.20 where `(( 1+ ))` is `((: 1+: …`.
				// The shell is already stopping with a status of its own, so
				// arithCmdFailed is not consulted either — its answer for
				// bash is the non-fatal 1, which is what a `-c` string's 127
				// would have been overwritten with (#3574).
				r.diagf("%s\n", r.arithFailure(text, err))
				return nil
			}
			// The expression is named here as it is everywhere else an
			// expression fails. It was not, and a loop doing arithmetic
			// reported `division by 0` with nothing to say which iteration
			// or which expression had done it — where the expansion route
			// for the identical failure quoted it back (#1985).
			r.diagf("%s\n", r.diag().arithConstructFailure("((", r.arithFailure(text, err)))
			r.status = r.arithCmdFailed(1)
			return nil
		}
		r.status = boolInt(!v)
		r.arithZeroLeft = !v
		return nil
	})
}

// arithCommandBadSubscript is `(( a[b c] ))` — a `(( ))` whose failure came
// from a **subscript** rather than from the expression around it.
//
// Two things part it from the neighbor below, and the first is unanimous.
// **The sentence names no construct**: measured 2026-09-17, bash 5.3.20
// writes `b c: arithmetic syntax error …` here and `((: b c : …` for
// `(( b c ))`, which is the construct's own arithmetic and is the control;
// zsh and ksh93 write the bare sentence for both. We wrote `((: ` for both,
// so a computed subscript was blamed on the brackets it sat in.
//
// **And the give-up is the subscript's in one column.** bash gives up the
// input line wherever a bad subscript is written — `$(( a[b c] ))`, the same
// subscript one construct over, already goes through
// Runner.giveUpForABadSubscript — and gave up nothing here, so `(( a[b c] ));
// echo "same=$?"` printed a `same=` bash never reaches. The other two let the
// construct catch it and answer with its own status and fatality, which is
// what arithCmdFailed already does for them: zsh's 2 with the line running on
// and ksh93's ended input are both that. See
// Semantics.BadSubscriptEscapesAnArithmeticCommand (#3507).
//
// The C-style `for` header words its parts through the construct at the same
// two calls and is left alone: it is a different construct with three
// expressions and its own naming rule, and nothing here was measured about
// it.
func (r *Runner) arithCommandBadSubscript(sentence string) int {
	if r.badSubscriptInAnArithmeticConstruct(sentence) || r.unspecified {
		return r.status
	}
	return r.arithCmdFailed(1)
}

// badSubscriptInAnArithmeticConstruct is the half both arithmetic constructs
// share: the sentence with no construct in front of it, and the give-up in
// the column that gives a bad subscript up wherever one is written. It
// reports whether that give-up was taken, leaving the caller to let its own
// construct answer where it was not.
//
// One door for `(( ))` and for the C-style `for` header, because the same
// measurement covers both and a second copy that answered one of them is how
// this repository keeps re-finding the same bug. Measured 2026-09-17, bash
// 5.3.20, a script file: `for (( i=a[b c]; i<1; i++ ))` writes `b c:
// arithmetic syntax error …` and gives up the line, exactly as `(( a[b c] ))`
// does, where `for (( i=b c; … ))` — the header's own arithmetic — is `((:
// i=b c: …` with the line still running. zsh and ksh93 end the input at the
// header either way, which is what they already did here.
func (r *Runner) badSubscriptInAnArithmeticConstruct(sentence string) bool {
	r.diagf("%s\n", sentence)
	if !r.ask(r.sem().BadSubscriptEscapesAnArithmeticCommand,
		"a bad subscript inside `(( ))` being given up as the subscript's failure") {
		return false
	}
	r.giveUpForABadSubscript()
	return true
}

// arithCmdFailed is what `(( ))` leaves behind once it has said what went
// wrong, given the status the failure would otherwise carry.
//
// One question for both ways an expression can fail, because the shell that
// answers it differently answers the same for both: `(( 1+ ))`, `(( 1 2 ))`
// and `(( 8#9 ))` never reach the evaluator and `(( 1/0 ))` and a call to a
// math function nothing defines do, and all five are 2 in zsh 5.9.2 and 1 in
// bash 5.3. A status split across the two branches would have been two axes
// with one answer each and no measurement separating them.
//
// The dialect's answer stands in front of the general one on the parse branch
// rather than beside it: `(( ))` is a construct whose failure has a status of
// its own, and taking the syntax status there gave zsh 1 where it leaves 2
// (#1625).
func (r *Runner) arithCmdFailed(otherwise int) int {
	if r.ask(r.sem().ArithCommandErrorIsFatal, "a `(( ))` that could not be evaluated abandoning the input") {
		// The same shape ConditionArithmeticErrorIsFatal has, and through the
		// same door: the status is decided below either way, and what the
		// dialect adds is that there is no next line to read it.
		r.abandonOverArithmetic()
	}
	if r.ask(r.sem().ArithCommandErrorStatusIsTwo, "the status a failed `(( ))` leaves") {
		return 2
	}
	return otherwise
}

// abandonOverArithmetic gives up the input over an expression the dialect
// calls fatal, for the two constructs that ask that question.
//
// An *error* rather than a request to stop, which is the half that decides how
// far the give-up reaches. A boundary reading a file of its own catches an
// error and carries on past it, and both constructs stop at that boundary:
// measured 2026-09-11, ksh93's `. ./s.sh; echo after` prints `after` over a
// file holding `(( 1+ ))` and over one holding `[[ 1+ -eq 0 ]]`, and so does
// zsh for the second. Marked only as a stop, the give-up cost the whole
// script in both — which is the same error one level down as the one
// interp/source.go is named for.
//
// Not fatalQuiet, which is the other door: that one decides the status as
// well, and both of these constructs have a status of their own that the
// caller has already worked out.
func (r *Runner) abandonOverArithmetic() {
	r.ctl, r.abandon, r.errexitStopped = controlExit, abandonError, false
}

// wordInvalidNumber words "this is not a number" the way the dialect does.
func (r *Runner) wordInvalidNumber(text string) string {
	return Wording(r.diag().InvalidNumber, "invalid number: %s", text)
}

// arithTree is the expression to evaluate, given what the parser managed and
// the text it came from.
//
// A tree the parser built is used as it stands. A nil one means the text had
// an expansion in it, so it is not an expression until that has happened —
// substituted first, read second, which is the order every shell in the panel
// uses and the only order that makes `$(( $x$y ))` with x=`1+` and y=`2`
// come to 3.
func (r *Runner) arithTree(tree syntax.ArithExpr, text string) (syntax.ArithExpr, error) {
	out, _, err := r.arithTreeOver(tree, text, arithTextArrived)
	return out, err
}

// arithTreeRead reads an expression out of text that has **already** been
// expanded, which is not the job arithTree does.
//
// A subscript and a substring's range are words, and the caller expands them
// before anything arithmetic happens — so by the time the text arrives here
// the substitutions written in it have been performed once already. Handing
// it to arithTree expanded it a second time, and the second pass ran what the
// first had only produced: `k='$(cmd)'; a[$k]=V` executed cmd and assigned
// the element its output named, in every dialect, where no shell on the panel
// runs anything (#3047).
//
// So the text is read as the expression it already is. That costs nothing any
// shell offers. Measured 2026-09-15 from a script file, and unanimous in bash
// 5.3.20, bash 3.2.57, zsh 5.9.2 and ksh93u+ 2012-08-01: a `$( )`, a
// backquoted run, a `$(( ))` and a bare `$i` left in an expanded subscript are
// all refused as an operand, while a bare *name* still resolves — `k='i'` and
// `k='1+1'` each name an element in all four. Resolving a name is the
// evaluator's job; it is not a second round of expansion, and that is the
// whole of the distinction this function draws.
func (r *Runner) arithTreeRead(text string) (syntax.ArithExpr, error) {
	p := syntax.NewParser("", r.dialect())
	out := p.ParseArithExpanded(text, syntax.Pos{})
	if err := p.Err(); err != nil {
		// Worded about the text a script wrote: the marks are this
		// implementation's bookkeeping and a refusal carrying one prints a
		// stray NUL into a log. See stripArithValueMarks.
		return nil, unmarkArithFailure(err)
	}
	return out, nil
}

// arithTreeOver is arithTree with the text the parser was actually handed, for
// the one caller that has to look at where in it the failure was.
//
// The expansion happens once, here, and the text is handed back rather than
// recomputed: expanding it a second time to find an offset would run a command
// substitution on the right-hand side twice, which is the mistake #1915 was.
func (r *Runner) arithTreeOver(tree syntax.ArithExpr, text string, origin arithTextOrigin) (syntax.ArithExpr, string, error) {
	if tree != nil && !r.arithPrecedenceOptionInCharge() {
		return tree, text, nil
	}
	expanded := r.expandArithText(text, origin)
	p := syntax.NewParser("", r.dialect())
	// Read as the *result* it now is. A `$` still standing after the
	// expansion came out of a value, and begins no operand: measured
	// 2026-09-16 from a script file, `x=2; e='1+$x'; $(( $e ))` is `$x :
	// arithmetic syntax error: operand expected` in bash 5.3.20, the
	// operand sentence in zsh 5.9.2 and `arithmetic syntax error` in
	// ksh93u+. Handing it back to the reader that waits for expansions
	// left no tree and no complaint either, which evaluated to a silent
	// **0** — the worst of the three answers, and the one this shell gave
	// (#3303).
	out := p.ParseArithExpanded(expanded, syntax.Pos{})
	// The marks go no further than the reader they were put on for. Every
	// caller of this uses the text it gets back to *show* something — a
	// trace, a refusal, the value a reader stopped at — and a dialect
	// without subscripts never scans one at all, so its expression keeps
	// the marks all the way to the diagnostic: measured, our `dash` wrote a
	// NUL either side of a value's quote into `arithmetic expression:
	// expecting EOF`. See stripArithValueMarks.
	shown := r.shownArithText(expanded)
	if err := p.Err(); err != nil {
		return nil, shown, r.shownArithFailure(err)
	}
	return out, shown, nil
}

// arithTextOrigin says whether text handed to [Runner.arithTreeOver] is the
// source text of an arithmetic construct or a result that reached it already
// expanded, which is the one thing the double-quote removal turns on.
//
// The removal is a rule about **text a script wrote inside `(( … ))`** and
// about nothing else — see [syntax.Parser.ParseArithSubscript] for the rows —
// and the reader cannot tell the two apart once the text is a string. The
// parse-time reader is told the same thing by its own `dequote`; this is that
// question asked again for the expression that had to be expanded first.
type arithTextOrigin bool

const (
	// arithTextArrived: the text reached the evaluator already expanded, so
	// a quote in it is a character of a result. `let 'x = 1"0"'` and
	// `x='1"0"'; (( y = x ))` are the shape, and bash refuses both.
	arithTextArrived arithTextOrigin = false
	// arithTextWritten: the text is what a script wrote between `(( ))`,
	// `$(( ))` or a C-style `for` header's semicolons.
	arithTextWritten arithTextOrigin = true
)

// shownArithText is the expanded expression as this dialect quotes it back.
//
// The marks come off either way, since they are this implementation's
// bookkeeping; the question is whether the bytes they stood in front of are
// written with a backslash. See Diagnostics.ArithValueShownEscaped.
func (r *Runner) shownArithText(text string) string {
	if strings.IndexByte(text, syntax.ArithValueMark) < 0 {
		return text
	}
	if r.diag().ArithValueShownEscaped {
		return syntax.EscapeArithValue(text)
	}
	return syntax.UnmarkArithValue(text)
}

// shownArithFailure is unmarkArithFailure for the same reading: the error's
// extent and token are slices of the marked text the parser was handed, so
// they are shown the way the expression beside them is.
func (r *Runner) shownArithFailure(err error) error {
	se, ok := err.(*syntax.Error)
	if !ok {
		return err
	}
	out := *se
	out.Expr = r.shownArithText(out.Expr)
	out.Token = r.shownArithText(out.Token)
	out.Msg = r.shownArithText(out.Msg)
	return &out
}

// unmarkArithFailure is a parse failure worded about the text a script wrote
// rather than the marked one it was read from: the error's extent and token
// are slices of what the parser was handed. See stripArithValueMarks.
func unmarkArithFailure(err error) error {
	se, ok := err.(*syntax.Error)
	if !ok {
		return err
	}
	out := *se
	out.Expr = stripArithValueMarks(out.Expr)
	out.Token = stripArithValueMarks(out.Token)
	out.Msg = stripArithValueMarks(out.Msg)
	return &out
}

// arithPrecedenceOptionInCharge reports whether a dialect's run-time option is
// deciding where the arithmetic operators bind, in which case the tree the
// file's own read built is not the tree this expression has now.
//
// Measured 2026-09-15 on zsh 5.9.2, which is the shell with the option:
// `setopt c_precedences` changes what `$(( 1 << 2 + 1 ))` answers on a line
// the parser has already read, and changes it inside a function whose body
// was read before the option was touched. So an expression is read again when
// it runs, which is the same thing that already happens to one with a `$` in
// it — see [Runner.expandArithText] — and this is the second reason for it.
//
// It is the *option being in charge* rather than the order differing, so a
// `setopt c_precedences` in a shell whose dialect already binds C's way still
// re-reads. Comparing the two orders instead would make the cost depend on
// which dialect a script happens to run under, and the answer is the same
// either way; nothing outside the one shell with the option ever asks.
func (r *Runner) arithPrecedenceOptionInCharge() bool { return r.arithPrecedenceMoved }

// expandArithText substitutes into an arithmetic expression before it is read.
//
// The inside of `$(( ))` is expanded the way a double-quoted string is —
// parameters, command substitutions and nested arithmetic — and only then is
// the result an expression. `$(( $x$y ))` with x=`1+` and y=`2` is 3 in every
// shell in the panel, which is a fact about *when* the substitution happens
// and cannot be reproduced by a tree built from the text as written.
//
// It is the same scan a here-document body gets, and for the same reason: in
// both, a quote is an ordinary character and only the expansions matter.
func (r *Runner) expandArithText(text string, origin arithTextOrigin) string {
	// The double quotes a script wrote come out as the text is joined, which
	// is a step later than it reads: the quotation is a quoting context while
	// the expansions in it are performed and is gone from the result. Both
	// halves are measured — `$(( 1"0" + $i ))` is 11 in bash 5.3.20 with
	// `i=1`, and `(( m["'$kq'"] = 42 ))` stores under `'q'` there, the
	// double quotation having decided that the apostrophes stop nothing.
	// The parse-time reader runs the same removal over an expression with
	// nothing to expand; without this one an expression that had to be
	// expanded first never had it run, and `(( "assoc[$key]++" ))` was
	// refused as an operand where bash increments the element (#4255).
	dequote := origin == arithTextWritten &&
		r.lang().ArithDoubleQuote == syntax.ArithDoubleQuoteRemoved
	if !strings.ContainsAny(text, "$`") {
		// Nothing to expand, so the parse-time reader has already run the
		// removal over this text and there is no second round to run.
		return text
	}
	// The bytes a value puts *inside brackets the source wrote* are marked,
	// so the bracket scanner cannot read them back as subscript syntax. The
	// depth is counted over the literal spans alone, which is the whole of
	// the distinction: `m[$key]` has a source bracket around the expansion
	// and `v=a[1]; $(( $v ))` has none, and the two are measured to part —
	// bash reads the second back as a subscript and answers the element,
	// and does not read the first back. See syntax.ArithValueMark.
	scan, balanced := syntax.ArithBracketScan{}, true
	// The depth is counted through quotations wherever the parser's own scan
	// reads them, so the two readings of the same brackets cannot part.
	quoted := r.lang().ArithSubscriptQuoting
	// The axis is asked at most once and only where a subscript's expansion
	// really produced a bracket: under either answer `$(( a[$i] ))` is the
	// same expression, so a dialect that has not chosen has nothing to be
	// refused over.
	asked, protect := false, false
	// Read before anything is expanded, because an expansion an apostrophe
	// stops must not be performed at all. See Runner.stoppedArithSpans.
	spans, read := r.arithSpans(text)
	if !read {
		return text
	}
	// Where each subscript the *source* wrote lands in the output, collected
	// here because nothing in the finished text says which brackets were the
	// script's: an arrived bracket is the same byte. One reader wants it —
	// see Runner.truncateSubscriptsAtAQuotedExpansion — and it is recorded
	// on the way past rather than searched for afterwards.
	var subs []sourceSubscript
	// at is where the next part lands in the output, open where the subscript
	// being scanned began, quoteOpen where the apostrophe it is inside began,
	// and runs the quoted runs that have performed an expansion so far.
	at, open, quoteOpen := 0, -1, -1
	var runs []int
	out, _, _ := r.expandSpansWith(spans, func(literal bool, part string) (wrote string) {
		// The *written* length, since a marked part is longer than the one
		// that came in and every later extent is measured from the output.
		defer func() { at += len(wrote) }()
		if literal {
			// Quoting and all: a `]` the source wrote inside a quotation
			// closes no subscript, so it must not close one for the depth
			// either — and the state carries across the spans, because the
			// quotation a span opens can hold the next expansion. The
			// parser draws the same boundary with the same type.
			//
			// The extents are taken from what this span *writes* rather
			// than from where the byte stood, since a removed quote shifts
			// everything after it.
			var b strings.Builder
			b.Grow(len(part))
			for i := 0; i < len(part); i++ {
				was := scan.Quote()
				switch c := part[i]; {
				case quoted && (was != 0 || scan.Depth > 0) && scan.Content(c):
					// Inside a subscript the source opened, or inside a
					// quotation that opened in one. The flag says a
					// quotation *inside* a subscript holds its brackets,
					// and a quotation the script wrote before any bracket
					// holds nothing: measured 2026-09-22 on bash 5.3.20
					// with `declare -A m; k='a]b'; m[$k]=1`, the refusal
					// `(( 'm[$k]' ))` earns names `'m[a\]b]'` — the
					// expansion escaped, so the apostrophe did not stop
					// the `m[` from opening a subscript. Reading it as one
					// left the expansion unmarked and the same refusal
					// named `'m[a]b]'`, which says the key ended where it
					// did not (#4255). See syntax.Dialect.ArithSubscriptQuoting.
					if was == 0 && scan.Quote() == '\'' {
						quoteOpen = at + b.Len()
					}
				case c == '[':
					scan.Depth++
					if scan.Depth == 1 {
						open, runs = at+b.Len()+1, nil
					}
				case c == ']':
					scan.Depth--
					if scan.Depth < 0 {
						balanced = false
					}
					if scan.Depth == 0 && open >= 0 {
						subs = append(subs, sourceSubscript{open, at + b.Len(), runs})
						open, runs = -1, nil
					}
				}
				if dequote && part[i] == '"' && scan.Depth == 0 {
					// Dropped only now that the scan has read it, so the
					// quotation still holds the brackets it was written
					// around.
					//
					// Outside a subscript and nowhere else. A quotation
					// *inside* one is removed by the reading that subscript
					// gets — as a word's quoting where the brackets hold a
					// key, which is what keeps an apostrophe inside it a
					// character rather than a quotation: measured
					// 2026-09-20, `declare -A m; kq=q; (( m["'$kq'"] = 42
					// ))` stores under `'q'` in bash 5.3.20. Taking the
					// byte out here left `'q'` behind for the key's own
					// removal to strip, and the element landed under `q`.
					continue
				}
				b.WriteByte(part[i])
			}
			return b.String()
		}
		if scan.Depth <= 0 {
			// Outside any bracket the source wrote, so a bracket in here is
			// the value's own and the subscript it opens arrived whole. Its
			// quoting is the one thing that needs saying; the bracket itself
			// must keep delimiting and a `$` must keep waiting, which is why
			// nothing else here is touched. See
			// Runner.markArrivedSubscriptQuoting.
			return r.markArrivedSubscriptQuoting(part)
		}
		if scan.Quote() == '\'' && quoteOpen >= open && open >= 0 &&
			(len(runs) == 0 || runs[len(runs)-1] != quoteOpen) {
			// An expansion performed inside an apostrophe run the source
			// wrote, which is what one dialect ends the key at. Recorded
			// rather than looked for afterwards: an expansion whose result
			// holds no syntax character is marked nowhere, so the finished
			// text cannot say a run performed one.
			runs = append(runs, quoteOpen)
		}
		if !strings.ContainsAny(part, arithValueMarked) {
			return part
		}
		if !asked {
			asked = true
			protect = !r.ask(r.sem().ArithSubscriptRereadsItsExpandedText,
				"a subscript's expanded text being read again as subscript syntax")
		}
		if !protect {
			return part
		}
		return markArithValue(part)
	}, r.stoppedArithSpans(text, spans))
	if scan.Depth == 0 && balanced && !scan.Unclosed() {
		// A key that ends at the run which performed an expansion in it,
		// where the dialect says so. Over the subscripts a script wrote and
		// nothing else, which is what the extents above are for.
		out = r.truncateSubscriptsAtAQuotedExpansion(out, subs)
	}
	if scan.Depth != 0 || !balanced || scan.Unclosed() {
		// The source never closed the bracket it opened, so there is no
		// bracket of the script's for a value's to be distinguished from —
		// and the shell that draws the distinction stops drawing it here
		// too. Measured 2026-09-13: `a=(9 8 7); k='1]'; $(( a[$k ))` is 8 in
		// bash 5.3.15, the value's `]` closing a subscript the script left
		// open. Stripped rather than never applied, because whether the
		// brackets balance is only known once the whole text exists.
		return stripArithValueMarks(out)
	}
	return out
}

// markedSubscriptWord expands a word once and marks what landed inside a
// bracket the *script* wrote, so the bracket scanner behind an arithmetic
// reader can still tell the script's brackets from a value's.
//
// The one place that distinction can be drawn: once a word is joined, an
// arrived bracket is the same byte as a written one. [Runner.expandArithText]
// draws it over raw text and this draws it over a word's spans, and the two
// are the same rule — see syntax.ArithValueMark.
//
// protects says which spans count as content rather than as syntax, and it
// is the whole of what the two callers disagree about: a condition's operand
// protects every span that is not plain unquoted text, and a substring's
// range protects a narrower set. A span that is not protected is scanned for
// brackets like any other text, so its `]` still closes a subscript.
func (r *Runner) markedSubscriptWord(w *syntax.Word, protects func(syntax.Span) bool) string {
	var scan syntax.ArithBracketScan
	advance := func(text string) {
		for i := 0; i < len(text); i++ {
			switch b := text[i]; {
			case scan.Content(b):
			case b == '[':
				scan.Depth++
			case b == ']':
				scan.Depth--
			}
		}
	}
	return r.wordTextNoSplit(w, func(sp syntax.Span, text string) string {
		if sp.Kind == syntax.Literal && sp.Quoting == syntax.Unquoted {
			advance(text)
			return text
		}
		if scan.Depth <= 0 {
			// Not inside brackets the script wrote, so there are no brackets
			// of the script's for a value's to be told apart from — and the
			// value's are then the only ones there are. Measured 2026-09-16:
			// `m[k]=5; key=k; e='m[$key]'; [[ $e -eq 5 ]]` holds in bash
			// 5.3.20, the whole subscript having come out of the value, where
			// `[[ a[$k] -eq 9 ]]` with `k='x]'` reads the value's bracket as
			// part of the key (#3303).
			return markArithValueNul(text)
		}
		if !protects(sp) {
			// Text the caller reads as syntax rather than as content, so its
			// own brackets delimit exactly as an unquoted one's do.
			if sp.Kind == syntax.Literal {
				advance(text)
			}
			return markArithValueNul(text)
		}
		return markArithValue(text)
	})
}

// markArithValue puts a mark in front of each byte of an expansion's result
// that the bracket scanner would otherwise read as syntax.
//
// The brackets and the quotation marks a scan would open on, and the mark
// itself so that a NUL a value really carried is still one byte of data when
// the marks come off.
func markArithValue(part string) string {
	var b strings.Builder
	for i := 0; i < len(part); i++ {
		if strings.IndexByte(arithValueMarked, part[i]) >= 0 {
			b.WriteByte(syntax.ArithValueMark)
		}
		b.WriteByte(part[i])
	}
	return b.String()
}

// markArithValueNul marks a NUL a value carried, and nothing else.
//
// The bytes markArithValue covers are two different kinds of thing, and only
// one of them is about brackets. A `[`, a `]`, a quote, a `$` and a backtick
// are marked because the *scanner* would read them as syntax, so a span
// outside brackets the script wrote has none to be told apart from and is
// rightly left alone. The mark byte is not one of those: it is marked because
// the encoding has to be reversible, and syntax.ArithValueMark says so —
// "a mark in front of a mark is a NUL that was data, so a bare one can only
// be this". That invariant holds over the whole text or it holds nowhere.
//
// It did not. A NUL a value carried arrived bare wherever the depth gate sent
// the span back unmarked, and unmarkArithValue then took it **and the byte
// behind it**, so a condition compared `ab` where the value was `a\0b` — and
// only where there was a byte behind it, which is why a NUL at the very end
// of a value survived and looked like the encoding working. Measured
// 2026-09-25: `v=$'a\0b'` has `${#v}` 3 and `[[ $v == ab ]]` held (#4516).
//
// Separate from markArithValue rather than a flag on it, because the two
// answer different questions and a reader deciding which to call is deciding
// whether brackets are in play — which is exactly the distinction the depth
// gate above draws.
func markArithValueNul(part string) string {
	if strings.IndexByte(part, syntax.ArithValueMark) < 0 {
		return part
	}
	var b strings.Builder
	b.Grow(len(part) + 1)
	for i := 0; i < len(part); i++ {
		if part[i] == syntax.ArithValueMark {
			b.WriteByte(syntax.ArithValueMark)
		}
		b.WriteByte(part[i])
	}
	return b.String()
}

// arithValueMarked is the alphabet the marking covers: the bytes a scanner
// would read as syntax, and the mark itself so that a NUL a value really
// carried is still one byte of data when the marks come off.
//
// The brackets, because a `]` a value carries closes no subscript the source
// opened. The three quoting characters for the same reason one stage on: a
// subscript's brackets are scanned through quotations, and quote removal is
// then performed over what they held, so a value that carries a `'` would
// otherwise open a quotation nobody wrote and lose its own two characters
// out of the key. Measured 2026-09-16, `typeset -A a; a["'q'"]=21; a[q]=22;
// k="'q'"` makes `$(( a[$k] ))` 21 in bash 5.3.20 and ksh93u+ 2012-08-01,
// where the same two characters written in the source name `q`.
//
// The `$` and the backtick for the third reason on the same list: a subscript
// that still holds an expansion has it performed, and a `$` a value carried
// is not one. Measured 2026-09-16 from a script file, `a=(9 8 7); i=1;
// k='$i'`: `$(( a[$k] ))` is `$i: arithmetic syntax error: operand expected`
// in bash 5.3.20 — the `$i` the value carried begins nothing — where
// `e='a[$i]'; $(( $e ))` is 8, the brackets having come out of a value too
// and nothing being marked (#3303, #3047).
const arithValueMarked = "[]'\"\\$`\x00"

// stripArithValueMarks takes the marks off text that is about to be *shown*.
//
// A diagnostic names the expression a script wrote, and a mark is this
// implementation's bookkeeping rather than anything the script contains: a
// refusal carrying one would print a stray NUL into a log. The subscript
// scanner takes its own off, so this is for the text that never became one.
func stripArithValueMarks(text string) string {
	if strings.IndexByte(text, syntax.ArithValueMark) < 0 {
		return text
	}
	return syntax.UnmarkArithValue(text)
}

// arithSubscriptRead is the subscript text its readers read.
//
// A subscript that still holds an expansion has it performed, once, here —
// which is where the panel performs it and nowhere else in the expression.
// Measured 2026-09-16 from a script file with standard input on /dev/null,
// `typeset -A m; m[k]=5; key=k; a=(10 20 30); i=1`:
//
//	                          bash 5.3.20  bash 3.2  zsh 5.9.2  ksh93u+
//	e='m[$key]'; $(( $e ))              5         5          5        5
//	f='a[$i]';   $(( $f ))             20        20         10       20
//	$(( 1+$x ))  through a value    refused   refused    refused  refused
//
// So it is the *subscript* that is read again and not the expression: a `$`
// left anywhere else is an operand failure in every column. Unanimous among
// the four with arrays, so it is core and not an axis; dash and BusyBox ash
// have no arrays to ask.
//
// Once. The counter #3252 built reads 1 on `$(( $e ))` with a command
// substitution in the subscript, in bash and here — running it *is* the side
// effect, so nothing can hide behind it.
//
// Not where a *value* put the `$` there. That is #3047's rule and the other
// half of this one: with `k='$i'`, `$(( a[$k] ))` is `$i: arithmetic syntax
// error: operand expected` in bash 5.3.20, bash 3.2.57, ksh93u+ and zsh —
// none of the four reads the `i` behind it — because the brackets were the
// script's and only what they held came out of a value. The marks are how
// that is known, and a subscript carrying both a marked expansion and an
// unmarked one is read as it stands: half an expansion is an answer no column
// gives.
//
// reading says which of the two consumers is waiting, because they want
// different text out of the same expansion. See subscriptReading.
func (r *Runner) arithSubscriptRead(marked string, reading subscriptReading) string {
	if !syntax.UnmarkedExpansion(marked) || syntax.MarkedExpansion(marked) {
		return marked
	}
	text := syntax.UnmarkArithValue(marked)
	if reading == subscriptAsExpression {
		return r.expandArithText(text, arithTextArrived)
	}
	return r.arithSubscriptKeyText(text)
}

// subscriptReading is what a subscript's text is about to become, which is
// the one thing [Runner.arithSubscriptRead] cannot work out for itself.
//
// The two readings want the *same* expansion performed and its result marked
// differently, and there is no third. See [Runner.arithSubscriptKeyText] for
// the measurement that parts them.
type subscriptReading bool

const (
	// subscriptAsExpression: the text is handed to the parser and read as
	// arithmetic, which is what an indexed name's brackets hold.
	subscriptAsExpression subscriptReading = false
	// subscriptAsKey: the text becomes an associative array's key, which is
	// a string and is never read as syntax again.
	subscriptAsKey subscriptReading = true
)

// arithSubscriptKeyText performs the expansion a re-read subscript still
// holds, for the reading that makes it a key, and marks what it produced.
//
// The marking is the whole point of it not being [Runner.expandArithText].
// Everything this is handed is *already* between a subscript's brackets — the
// text is the subscript — so there is no depth to count and no bracket of the
// script's to find: every byte an expansion produces here is a byte of the
// key, exactly as one produced inside brackets the source wrote is. Without a
// mark, [subscriptQuoteRemoval] cannot tell an apostrophe a value carried from
// one the re-read text spelled, and takes it off.
//
// Measured 2026-09-20, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> f.sh` over
// a script file with standard input on the null device, against
// `typeset -A m; k="q'r'z"; m[$k]=4`:
//
//	                          bash 5.3.20  ksh93u+  zsh 5.9.2
//	e='m[$k]';    $(( $e ))             4        4          4
//	g="m[q'r'z]"; $(( $g ))             0        4          4
//
// Unanimous on the first row, so it is the core's and no axis. The second row
// is the control that says this is marking rather than a stopped removal: the
// same two apostrophes, in the same brackets, reached by the same re-read —
// and bash loses the element there, because those apostrophes are characters
// of `g`'s own text rather than the product of an expansion performed at
// subscript-read time. bash parts from the other two on that row over
// Semantics.SubscriptIsAQuotingContext, which is already answered and is not
// touched here.
//
// **Only this reading**, and that is measured rather than tidiness. The other
// one hands its text back to the parser, where a mark is what stops a value's
// bracket closing a subscript — and that is
// Semantics.ArithSubscriptRereadsItsExpandedText, which zsh answers **yes**:
// measured the same day, `a=(9 8 7); kb='1]'; e='a[$kb]'; $(( $e ))` is 9 in
// zsh 5.9.2, the value's `]` closing the brackets. Marking there would
// override an answered axis in the one column that holds the other value.
// Nothing overrides anything here, because quote removal is this reading's
// only consumer and zsh removes no quoting from a subscript at all.
func (r *Runner) arithSubscriptKeyText(text string) string {
	if !strings.ContainsAny(text, "$`") {
		return text
	}
	if quotationStopsASubscriptsExpansion(text) &&
		r.ask(r.sem().SubscriptIsAQuotingContext,
			"an array subscript being a quoting context") {
		return r.arithSubscriptKeyQuoted(text)
	}
	return r.arithSubscriptKeyScan(text)
}

// arithSubscriptKeyScan is that expansion with every quotation an ordinary
// character, which is the reading of a dialect whose subscript is no quoting
// context. See [Runner.arithSubscriptKeyQuoted] for the other one.
func (r *Runner) arithSubscriptKeyScan(text string) string {
	if !strings.ContainsAny(text, "$`") {
		return text
	}
	out, _, _ := r.expandRawSpansWith(text, func(literal bool, part string) string {
		if literal {
			return part
		}
		return markArithValue(part)
	})
	return out
}

// arithAssocKey is the key an associative array's subscript names when the
// subscript was written inside an arithmetic expression.
//
// The same question Runner.assocKey answers for `${m[k]}`, asked through the
// same axis, and it has to be asked twice because the two subscripts reach
// this shell in different shapes. A parameter expansion's is a syntax.Word
// with its quoting recorded per span; an arithmetic expression's is *text*,
// because the expression has already been expanded once by the time it is
// parsed — so what arrives here is a subscript with its remaining quote
// characters still in it, and nothing downstream would take them out.
//
// Measured 2026-09-16 with `declare -A a; a[q]=7`, each probe from a script
// file with standard input on /dev/null:
//
//	                      bash 5.3.20  ksh93u+  zsh 5.9.2
//	(( r = a[q] ))                  7        7          7
//	(( r = a['q'] ))                7        7          0
//	(( r = a[\q] ))                 7        7          0
//	(( a['k'] = 5 ))             [k]=5    [k]=5    ['k']=5
//
// Which is Semantics.SubscriptIsAQuotingContext exactly — yes in bash and
// ksh93, no in zsh — and this shell already answers it correctly one
// spelling over, where `${a['q']}` is 7 here and unset in zsh. So the bug was
// not a missing answer but an unasked question: the arithmetic route looked
// the key up as written and found nothing, which is the silent half of a
// wrong answer. `(( r = a['q'] ))` came to 0.
//
// A double-quoted subscript was already right in both columns before this,
// and by a different mechanism: the expansion that runs over an arithmetic
// expression before it is parsed takes the quotes out under the bash dialect
// and leaves them in under zsh's, so `a["q"]` arrives here as `q` in the one
// that should find the element and as `"q"` in the one that should not. That
// is why this asks the axis on *whether quoting was removed* rather than on
// whether the text changed — the two columns reach the right answer from
// opposite sides, and a comparison against the original text would have
// stopped asking in the column where the question is already settled.
//
// Asked only where removal changes the text, so a key with no quote and no
// backslash in it — nearly every key a script writes — never demands a
// dialect for the question.
func (r *Runner) arithAssocKey(sub string) string {
	bare, quoted := subscriptQuoteRemoval(sub)
	if !quoted {
		return syntax.UnmarkArithValue(sub)
	}
	if r.ask(r.sem().SubscriptIsAQuotingContext,
		"an array subscript written inside arithmetic being a quoting context") {
		return bare
	}
	return syntax.UnmarkArithValue(sub)
}

// subscriptQuoteRemoval performs quote removal on an arithmetic subscript and
// reports whether it took anything out.
//
// The boolean rather than a comparison at the caller, because "took something
// out" and "the text changed" are not the same claim: `a[\q]` and `a['q']`
// both come to `q`, and a subscript whose quoting happens to remove to itself
// would still be a quoted one. Nothing in the panel distinguishes them today,
// and a caller that asked `bare != sub` would silently stop asking the axis
// the day one did.
//
// An unterminated quotation removes nothing. The expression will not parse
// past it in any case, and text that ran off the end is not a key anybody
// wrote on purpose.
func subscriptQuoteRemoval(sub string) (string, bool) {
	if !strings.ContainsAny(sub, arithValueMarked) {
		return sub, false
	}
	var b strings.Builder
	b.Grow(len(sub))
	// Whether a quotation of the *script's* was taken off, which is the
	// question the caller asks the axis about. Marks come off whatever
	// happens — they are this implementation's bookkeeping and never part of
	// a key — so "the text changed" would answer the wrong question here for
	// a second reason.
	removed := false
	for i := 0; i < len(sub); i++ {
		switch c := sub[i]; c {
		case syntax.ArithValueMark:
			// A byte a value carried: a character of the key, whatever it
			// spells, and the mark itself is not.
			if i+1 == len(sub) {
				return sub, false
			}
			i++
			b.WriteByte(sub[i])
		case '\\':
			if i+1 == len(sub) {
				return sub, false
			}
			i++
			if sub[i] == syntax.ArithValueMark {
				if i+1 == len(sub) {
					return sub, false
				}
				i++
			}
			b.WriteByte(sub[i])
			removed = true
		case '\'', '"':
			end := indexUnmarked(sub[i+1:], c)
			if end < 0 {
				return sub, false
			}
			b.WriteString(syntax.UnmarkArithValue(sub[i+1 : i+1+end]))
			i += end + 1
			removed = true
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), removed
}

// indexUnmarked is strings.IndexByte over the bytes a *script* wrote: the one
// behind a mark came out of a value and closes no quotation. -1 when there is
// none, which is the unterminated quotation subscriptQuoteRemoval leaves
// alone.
func indexUnmarked(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case syntax.ArithValueMark:
			i++
		case c:
			return i
		}
	}
	return -1
}
