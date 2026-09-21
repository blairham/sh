// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// A compound assignment's word does not end at its closing parenthesis
// everywhere. Where it does not, text written after the `)` stays part of the
// same word — and the word is then not an array assignment at all, because an
// assignment's value cannot hold an unquoted parenthesis.
//
// Measured 2026-09-21, one `-c` per row under `env -i PATH=/usr/bin:/bin
// LC_ALL=C`, with a shell function standing in for the builtin so the words
// it is handed are visible:
//
//	declare() { printf "<%s>" "$@"; echo; }
//	declare a=(1 2)x        bash 5.3.20  <a=(1 2)x>
//	declare a=(1 2)x y      bash 5.3.20  <a=(1 2)x><y>
//	declare a=(1 2)x=z      bash 5.3.20  <a=(1 2)x=z>
//	a=(1 2)x                bash 5.3.20  declare -- a="(1 2)x" — a scalar
//
// and the same lines in ksh93u+ 2012-08-01 and zsh 5.9.2 store the array and
// then look for a command called `x`. bash 3.2.57 answers as 5.3.20 does. So
// the panel splits and this is a grammar flag rather than a rule.
//
// **The word is rebuilt rather than taken from the source.** bash normalizes
// what it folds: `a=(1    2)x`, `a=( 1 2 )x` and a newline between the
// elements all reach the utility as `a=(1 2)x`, so the elements are written
// out again with one blank between them — the same shape
// [interp.Runner] rejoins a `name=( … )` operand into for the utilities that
// read one themselves.
//
// The elements keep their own spans, which is what makes the fold a *word*
// and not a string: `v=Q; declare a=($v 2)x` is `<a=(Q 2)x>` and
// `declare a=(1 "2 3")x` is `<a=(1 2 3)x>`, so the expansions still expand
// and the quotes are still removed where they stand.
func (p *Parser) foldCompoundAssignmentPastItsParenthesis(a *Assign, open, closing Pos) {
	if !p.dialect.CompoundAssignmentWordRunsPastItsParenthesis || p.err != nil {
		return
	}
	if !a.IsArray || len(a.Members) > 0 {
		return
	}
	if p.tok.Kind != TokWord || !p.touchesPrevious(a.Stop) {
		return
	}
	spans := make([]Span, 0, len(a.Elems)*2+3)
	spans = append(spans, Span{Value: "(", Pos: open})
	for i, el := range a.Elems {
		if el.Word == nil {
			// A nested literal or a compound variable's body, neither of
			// which the one dialect that folds here has. Left as the array
			// it was read as rather than folded into text nothing measured.
			return
		}
		if i > 0 {
			spans = append(spans, Span{Value: " ", Pos: el.Word.Pos()})
		}
		spans = append(spans, el.Word.Spans...)
	}
	spans = append(spans, Span{Value: ")", Pos: closing})
	spans = append(spans, p.tok.Spans...)
	end := p.tok.End
	a.IsArray, a.Elems = false, nil
	a.Value = p.newWord(spans, open, end)
	a.Stop = end
	p.next()
}
