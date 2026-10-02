// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// An operand of a declaration that runs as a **builtin**, rather than as the
// reserved word, is an ordinary word: split, matched, and read for a name
// after both. zsh's `ksh_typeset` keeps one shape of it whole — a word that
// writes an `=` — so that `typeset x=$(cmd)` declares one name holding the
// whole output where it would otherwise declare one name per word.
//
// Measured 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`), each line
// read by `eval` after `disable -r typeset`, so that the builtin is what runs
// (#5157):
//
//	                                   unsetopt kshtypeset    setopt kshtypeset
//	typeset x=$(echo a b)              x=a, b declared        x='a b'
//	typeset $nm=$(echo c d)            z=c, d declared        z='c d'
//	typeset "w"=… / w\=… / "x="… / y"="…   split                 whole
//	typeset $(echo z)=$(echo g h)      z=g, h declared        z='g h'
//	typeset q=r$(echo s t)=u           q=rs, t=u              q='rs t=u'
//	typeset $(echo a b)c=d             a, bc=d                a, bc=d
//	typeset m$(echo n o)=p             mn, o=p                mn, o=p
//	typeset $(echo m=1 n=2)            m=1, n=2               m=1, n=2
//	typeset k[1]=$(echo l m)           matched as `k[1]=l`    matched as `k[1]=l m`
//
// So the `=` has to be one the word *writes* — quoted or not, but not one an
// expansion produced — and what decides is whether the text in front of it
// comes to one field: the last three rows hold an `=` and split anyway,
// because a substitution in front of it did. The word is still matched
// against the filesystem, the one thing the option leaves alone.
//
// The same seven builtins take it — typeset, local, export, readonly,
// declare, integer and float, the last two reporting a bad math expression
// over the whole word rather than declaring a second name — and so does a
// declaration reached through `builtin`, with the reserved word still on.
// The reserved word itself is not touched by the option: `typeset "z"=$(echo
// c d)` splits in both states, because the parser decided its operands.
// Every emulation leaves the option off.

// OperandsKeepTheirAssignments reports zsh's `ksh_typeset`.
func (r *Runner) OperandsKeepTheirAssignments() bool { return r.operandsKeepTheirAssignments }

// SetOperandsKeepTheirAssignments moves it.
func (r *Runner) SetOperandsKeepTheirAssignments(on bool) { r.operandsKeepTheirAssignments = on }

// builtinDeclarationOperand expands the i-th word of a command whose argv so
// far names a declaration builtin, where the option keeps the word whole. The
// second result is false where it does not apply, and nothing has been
// expanded.
func (r *Runner) builtinDeclarationOperand(c *syntax.SimpleCmd, argv []string, i int, w *syntax.Word) ([]string, bool) {
	if !r.operandsKeepTheirAssignments || i == 0 || len(argv) == 0 {
		return nil, false
	}
	k := 0
	if argv[0] == "builtin" && len(argv) > 1 {
		k = 1
	}
	if !r.declares(argv[k]) || (k == 0 && r.declarationCommand(c, argv)) {
		// Not a declaration, or one the reserved word is running, whose
		// operands the parse already decided.
		return nil, false
	}
	span, off, ok := writtenEquals(w)
	if !ok {
		return nil, false
	}
	name := *w
	name.Spans = append(append([]syntax.Span{}, w.Spans[:span]...), syntax.Span{
		Kind: syntax.Literal, Value: w.Spans[span].Value[:off],
		Quoting: w.Spans[span].Quoting, Pos: w.Spans[span].Pos,
	})
	value := *w
	value.Spans = append([]syntax.Span{{
		Kind: syntax.Literal, Value: w.Spans[span].Value[off+1:],
		Quoting: w.Spans[span].Quoting, Pos: w.Spans[span].Pos,
	}}, w.Spans[span+1:]...)
	head := r.expandWordEscaped(&name)
	if len(head) != 1 {
		// Something in front of the `=` split, so the word is split as it
		// would have been: the last field in front joins the first behind.
		rest := r.expandWordEscaped(&value)
		if len(rest) == 0 {
			rest = []string{""}
		}
		if len(head) == 0 {
			head = []string{""}
		}
		joined := append([]string{}, head[:len(head)-1]...)
		joined = append(joined, head[len(head)-1]+"="+rest[0])
		return append(joined, rest[1:]...), true
	}
	return []string{head[0] + "=" + r.expandAssignValueMarked(&value)}, true
}

// writtenEquals finds the first `=` a word writes itself — in a literal span,
// quoted or not — and answers where it is.
func writtenEquals(w *syntax.Word) (span, off int, ok bool) {
	for i, s := range w.Spans {
		if s.Kind != syntax.Literal {
			continue
		}
		for j := 0; j < len(s.Value); j++ {
			if s.Value[j] == '=' {
				return i, j, true
			}
		}
	}
	return 0, 0, false
}
