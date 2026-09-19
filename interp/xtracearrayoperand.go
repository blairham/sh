// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// Where `set -x` writes a declaration utility's **array literal** operand, and
// what it writes there.
//
// `typeset a=(1 2)` is one command carrying one assignment, and this shell
// wrote the command and not the assignment: `+ typeset a` in every column,
// with the value the script handed over appearing nowhere (#3567).
//
// Measured 2026-09-19, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> x.sh` over
// a script file with `set -x` on line 1, stdin from `/dev/null`:
//
//	written                	bash 5.3.20               	ksh93u+ 2012-08-01        	zsh 5.9.2
//	`typeset b=(3 4)`      	`+ b=('3' '4')` / `+ typeset b`	`+ b=( 3 4 )` / `+ typeset b`	`+ typeset b=( 3 4 )`
//	`typeset -A m=([k]=v)` 	`+ m=(['k']='v')` / `+ typeset -A m`	`+ m[k]=v` / `+ typeset -A m`	one line
//	`typeset w=()`         	`+ w=()` / `+ typeset w`  	`+ typeset w`             	`+ typeset w=( )`
//	`typeset u+=(1 2)`     	`+ u+=('1' '2')` / `+ typeset u`	`+ u+=( 1 2 )` / `+ typeset u+`	`+ typeset u+=( 1 2 )`
//
// dash 0.5.12 and BusyBox ash 1.37.0 have no array literal at all, so the
// question cannot be put to them; bash 3.2.57 answers as 5.3.20 does.
//
// Three facts come out of that, and each is separately measured:
//
//  1. **The literal is expanded before the command's own line is written**,
//     in all three. `x='p q'; typeset b=("$x" r $(echo z))` traces the
//     substitution first and then `b=( 'p q' r z )` in ksh93 and zsh and
//     `b=('p q' 'r' 'z')` in bash — the values, never the words the script
//     wrote. That is the opposite of a *bare* literal in bash, which is
//     traced as written and expanded afterwards
//     (Semantics.TraceArrayLiteralShowsTheExpandedElements), so the operand
//     position is its own answer rather than that axis reaching further.
//
//  2. **Where the resulting assignment goes** splits the panel, and not the
//     way Diagnostics.TraceDeclarationOperand does. bash leaves a *scalar*
//     operand on the command line and takes an array one off it, so the two
//     fields are two answers in one shell and cannot be one field. See
//     Diagnostics.TraceDeclarationArrayOperand.
//
//  3. **Whether every element is quoted** splits it again. bash writes
//     `('3' '4')` where ksh93 and zsh write `( 3 4 )` — and the same bash
//     writes a tab inside the quotes rather than reaching for the `$'…'` its
//     own TraceQuoting uses elsewhere, while spelling an embedded quote
//     `'it'\''s'` and a lone one `\'` exactly as that value does. So it is
//     *whether* the rule fires and not which rule. See
//     Diagnostics.TraceArrayOperandQuotesEveryElement.
//
// What the **store** does is unchanged and has to be: `typeset -A m=([k]=v)`
// reads `[k]` as a key only because `typeset -A` has already declared the name
// a table, so the elements are expanded here and handed to the assignment that
// runs after the utility, exactly as Runner.prepareTracedAssign hands a bare
// literal's elements to its own store. Nothing is expanded twice.

// arrayOperand is one `name=( … )` written after a declaration utility's own
// word, together with where the bare name it leaves behind stands in argv.
//
// The position is recorded rather than searched for, for the reason
// Runner.declarationOperands is: two operands may name the same thing and an
// expanded word cannot say which of them it came from.
type arrayOperand struct {
	assign *syntax.Assign
	at     int
	// expanded is the element list, expanded once ahead of the trace and
	// consumed by the store that runs after the utility. Nil where nothing
	// asked for it — an untraced command — or where the expansion failed, in
	// which case the assignment is refused anyway.
	expanded *expandedAssign
}

// expandArrayOperands expands each array-literal operand's elements once,
// before the command's own trace line is written.
//
// Asked only while tracing, which is the same arrangement
// Runner.prepareTracedAssign has for a bare literal: nothing here changes what
// the assignment stores, only when the value it stores was computed. The wider
// question of *when* an untraced operand is expanded is a separate measured
// difference — bash and ksh93 expand it before the command's redirections are
// opened and this shell expands it after, which `typeset a=($(echo hi >&2))
// 2>/dev/null` shows — and is not this issue's.
func (r *Runner) expandArrayOperands() {
	if !r.tracing() {
		return
	}
	for i := range r.arrayOperands {
		op := &r.arrayOperands[i]
		a := op.assign
		if a.Members != nil || len(a.Elems) == 0 {
			// A compound body writes the member assignments it performs and
			// has no element list of its own, and an empty literal has
			// nothing to expand.
			continue
		}
		parsed, ok := r.literalElems(a.Elems,
			r.literalReadsSubscripts(a.Name, a.Elems, a.Append))
		if !ok {
			// The element list failed — an unmatched pattern where the
			// dialect calls that an error, a division by zero. The store
			// abandons the assignment for the same reason, so there is
			// nothing to write a line about.
			continue
		}
		op.expanded = &expandedAssign{assign: a, elems: parsed, elemsSet: true}
	}
}

// expandedArrayOperand is the element list already expanded for this operand,
// or nil where the store is to expand it itself.
func (r *Runner) expandedArrayOperand(a *syntax.Assign) *expandedAssign {
	for _, op := range r.arrayOperands {
		if op.assign == a {
			return op.expanded
		}
	}
	return nil
}

// traceArrayOperandsBefore is the assignment lines this dialect writes ahead of
// a declaration utility's own line for its array-literal operands.
//
// Behind the scalar operands' lines, which is the order the one column that
// writes both writes them in: `typeset x=1 a=(1 2)` is `+ x=1`, `+ a=( 1 2 )`
// and then `+ typeset x a` on ksh93u+, measured. That is the order the script
// wrote them in as well, and the reverse spelling is not modeled — the bare
// name of an array operand is appended to argv rather than standing where it
// was written, so there is no position here to sort the two lists by.
func (r *Runner) traceArrayOperandsBefore(d Diagnostics) []string {
	if d.TraceDeclarationArrayOperand != TraceOperandSplitBefore {
		return nil
	}
	var lines []string
	for _, op := range r.arrayOperands {
		lines = append(lines, r.traceArrayOperandLines(op, d)...)
	}
	return lines
}

// traceArrayOperandLines is what one array-literal operand writes: the element
// assignments it performs in the column that spells it that way, and one
// assignment line everywhere else.
//
// A compound body writes nothing here, and an *empty* one is a compound as
// much as a populated one is — syntax.Assign.Members is an empty slice rather
// than nil for `typeset w=()` in the dialect that has the construct, which is
// what makes `+ typeset w` alone the right answer there and `+ w=()` the right
// answer where the same three characters are an empty array. Its members are
// declarations of their own and trace themselves as they are performed, which
// is after the utility has run where ksh93 writes them in front of it; that row
// is a second ordering and is not this one.
func (r *Runner) traceArrayOperandLines(op arrayOperand, d Diagnostics) []string {
	a := op.assign
	if a.Members != nil {
		return nil
	}
	if lines, ok := r.traceLiteralAsElementWrites(a, op.expanded, d); ok {
		return lines
	}
	if len(a.Elems) == 0 && r.emptyListIsACompound() {
		// Nothing between the parentheses, in the dialect where that pair is
		// an empty compound body rather than an empty array — so nothing was
		// assigned and there is no line to write. The letters beside it do
		// not move the reading of the empty pair: measured 2026-09-19 on
		// ksh93u+ 2012-08-01, `typeset -A s=()` and `typeset -a t=()` are
		// `+ typeset -A s` and `+ typeset -a t` alone, which is also what the
		// bare `mm=()` on a declared table writes there — nothing.
		return nil
	}
	return []string{r.traceArrayOperandAssignment(a, op.expanded, d)}
}

// traceArrayOperandAssignment is the operand written as the assignment it
// performs: the target as the script spelled it, and the expanded list.
func (r *Runner) traceArrayOperandAssignment(a *syntax.Assign, e *expandedAssign, d Diagnostics) string {
	var b strings.Builder
	b.WriteString(a.Name)
	if a.Append {
		b.WriteString("+")
	}
	b.WriteString("=")
	b.WriteString(traceArrayOperandLiteral(a.Elems, expandedElemsOf(e), d))
	return b.String()
}

// traceArrayOperandLiteral renders the parentheses and what is between them.
//
// From the expanded elements rather than from the words, which is the whole of
// fact 1 above — and which is why a `[sub]=value` element is written from its
// two halves here where Runner.traceAssign writes the word as the script typed
// it. `i=1; typeset a=([i+1]=v)` is `['i+1']='v'` in bash and `a[i+1]=v` in
// ksh93, so the subscript is the text it expanded to and never the number it
// would evaluate to; `k=kk; typeset -A m=([$k]=v)` is `['kk']` and `m[kk]` in
// the same two, which is what says it expanded at all.
func traceArrayOperandLiteral(elems []*syntax.Word, parsed []literalElem, d Diagnostics) string {
	var words []string
	for i, w := range elems {
		if parsed == nil {
			words = append(words, syntax.PrintWord(w))
			continue
		}
		el := parsed[i]
		if el.subscripted {
			op := "="
			if el.appendValue {
				op = "+="
			}
			words = append(words, "["+traceArrayOperandElement(el.sub, d)+"]"+
				op+traceArrayOperandElement(el.value, d))
			continue
		}
		// However many fields a bare element expanded to, for the reason
		// traceArrayLiteral gives: the count is part of what the line says.
		for _, f := range el.fields {
			words = append(words, traceArrayOperandElement(f, d))
		}
	}
	return wrapArrayLiteral(strings.Join(words, " "), d.TraceArrayLiteral)
}

// traceArrayOperandElement renders one expanded element.
//
// The column that quotes every one of them reaches for the single-quoted
// spelling whether or not the ordinary rule would fire, and — measured on bash
// 5.3.20 — writes a control character inside those quotes rather than reaching
// for the `$'…'` its TraceQuoting uses for the same byte in an argument. An
// embedded quote is still `'it'\”s'` and a lone one still `\'`, so what the
// field moves is whether the rule fires and not which rule it is.
func traceArrayOperandElement(s string, d Diagnostics) string {
	if !d.TraceArrayOperandQuotesEveryElement {
		return traceQuote(s, d.TraceQuoting, d.TraceMetacharacters)
	}
	if s == "" {
		return "''"
	}
	if s == "'" {
		return `\'`
	}
	return traceShellQuote(s, d.TraceQuoting == QuoteShellLazy)
}

// traceArrayOperandWord is what stands in argv's place for an array operand,
// and whether this position holds one at all.
//
// Two shapes, from the one field: the column that splits the assignment off
// leaves the name behind, and the column that does not writes the whole
// assignment where the name stands.
//
// **Whether the append marker goes with the name or with the assignment** is a
// third answer, and it follows the *scalar* field rather than this one.
// Measured 2026-09-19 over `typeset u+=(1 2)`: ksh93u+ writes `+ u+=( 1 2 )`
// and then `+ typeset u+`, and bash 5.3.20 writes `+ u+=('1' '2')` and then
// `+ typeset u`. The column that keeps it is the column that splits a scalar
// operand too — there the leftover word is the operand cut at its `=`, `+` and
// all, which is what Runner.traceOperandCommandWord does for `typeset x+=q`
// — where bash's leftover is the name the parser handed the utility.
func (r *Runner) traceArrayOperandWord(i int, d Diagnostics) (string, bool) {
	for _, op := range r.arrayOperands {
		if op.at != i {
			continue
		}
		if d.TraceDeclarationArrayOperand == TraceOperandSplitBefore {
			if op.assign.Append && d.TraceDeclarationOperand == TraceOperandSplitBefore {
				return op.assign.Name + "+", true
			}
			return op.assign.Name, true
		}
		if op.assign.Members != nil {
			return op.assign.Name, true
		}
		return r.traceArrayOperandAssignment(op.assign, op.expanded, d), true
	}
	return "", false
}

// emptyListIsACompound reports whether `name=()` is read as a compound
// variable's empty body rather than as an empty array.
//
// The parser answers it for a body with something in it — syntax.Assign.Members
// is that reading — and cannot for an empty one, where the two spellings are
// the same three characters and a declaration's own letters have already
// settled which store the value goes to. So the question is put to the grammar
// that has the construct at all, which is the same place the parser asks it.
func (r *Runner) emptyListIsACompound() bool {
	return len(r.dialect().CompoundVariableDeclarators) > 0
}
