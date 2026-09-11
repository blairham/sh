// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// ExpandParametersOnly expands the parameters in raw text, and refuses text
// whose expansion would have to run a command.
//
// It exists for the completer, which is the one caller that expands text
// nobody asked to execute. `Expand` is the general entry point and it runs
// command substitutions, so a completer reaching for it would run somebody's
// command on a keystroke — `echo $(rm -rf ~)/<Tab>` is a line a person may
// well type and never mean to run. bash draws the boundary in the same place
// and it is measured: `echo $HOME/docum<Tab>` completes and
// `echo $(echo documents)/tar<Tab>` rings the bell, so the refusal is the
// answer rather than a limitation of it.
//
// The refusal is structural rather than a scan of the text, because a
// substitution can be nested where a scan would have to know the grammar to
// find it: `${undefined:-$(cmd)}` runs the command, and so does `${x[$(cmd)]}`
// and `${${(f)$(cmd)}}`. Every word a parameter expansion carries is walked —
// its subscript, its inner expansion, and both of its operands — so the answer
// is "no command runs" and not "no command was spotted".
//
// Arithmetic is refused with the rest. It cannot be dismissed as safe — `$((
// $(cmd) ))` is arithmetic holding a substitution, and ArithExpr is a tree
// this would have to walk as well — and refusing it costs nothing, because
// what a refusal produces is the completion this shell already gives for such
// a word: none.
//
// The bool is "this was expandable", not "this expanded to something". Empty
// text is expandable and expands to nothing, which is a directory portion that
// means the working directory rather than one that cannot be read.
func (r *Runner) ExpandParametersOnly(text string) (string, bool) {
	if text == "" {
		return "", true
	}
	// Text that ran out inside an expansion is not expandable, which is the
	// same answer this gives a substitution: a completer's job is to say
	// nothing rather than to guess at what the unfinished text meant. No
	// diagnostic either — a keystroke is not a line of script — so this asks
	// the lexer itself rather than going through rawSpans.
	spans, err := syntax.HeredocSpans(text, r.dialect())
	if err != nil {
		return "", false
	}
	if !expandableSpans(r.parseSpans(spans)) {
		return "", false
	}
	return r.expandRawText(text), true
}

// expandableSpans reports whether a word can be expanded without running
// anything. See ExpandParametersOnly, which carries the reasoning.
func expandableSpans(spans []syntax.Span) bool {
	for _, s := range spans {
		switch s.Kind {
		case syntax.Literal:
		case syntax.ParamExp:
			if !expandableParam(s.Param) {
				return false
			}
		default:
			// CommandSubst, ArithSubst and the two process substitutions.
			return false
		}
	}
	return true
}

// expandableParam walks the words a parameter expansion carries, each of
// which is expanded in its own right and each of which may hold a
// substitution the outer span's kind does not mention.
//
// A nil ParamExpr is a span the parser has not filled in, which is a span
// this cannot vouch for.
func expandableParam(p *syntax.ParamExpr) bool {
	if p == nil {
		return false
	}
	for _, w := range []*syntax.Word{p.Index, p.Inner, p.Arg, p.Arg2, p.Arg2Enclosed} {
		if w != nil && !expandableSpans(w.Spans) {
			return false
		}
	}
	return true
}
