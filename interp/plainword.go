// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// A word that is nothing but plain text, and a field that is.
//
// Most words on a command line are written text with nothing in them for any
// stage of expansion to act on: `[`, `-lt`, `10000`, `]`, a command name, an
// option. Each of them went through the whole pipeline anyway — the tilde and
// equals passes, the span loop, the field builder, and then the glob stage
// with its dozen dialect questions — to come back as itself, which was most of
// what a `[ $i -lt N ]` cost (#6353). These two say, from the bytes alone,
// that nothing would have happened, so the pipeline is not run.
//
// The test is a closed alphabet rather than a list of the metacharacters,
// because the metacharacters are a union over every dialect and every option
// a script can turn on — zsh's `#`, `^` and `~` under extendedglob, ksh's `@`
// and `!` in front of a group, `<1-5>`, a `%?` job spec, an `=cmd` — and a
// list that missed one would be a silent wrong answer. Every byte admitted
// here is text in every column of the panel, wherever it stands in a word:
// letters, digits, and `_ . / : , + - ]`. A `]` with no `[` in front of it
// opens nothing. The one `[` admitted is the field that is nothing else,
// which every dialect leaves alone because it is the name of the test
// builtin; see Runner.glob, which says the same.

// plainField reports whether pathname expansion would hand this field back
// unchanged: a field made of bytes no dialect reads as pattern syntax, with
// no mark of the expander's in it.
func plainField(f string) bool {
	if f == "[" {
		return true
	}
	if f == "" {
		return false
	}
	for i := 0; i < len(f); i++ {
		c := f[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		case c == '_', c == '.', c == '/', c == ':', c == ',', c == '+', c == '-', c == ']':
		default:
			return false
		}
	}
	return true
}

// plainLiteralWord is the word's one field, where the word is a single
// unquoted literal of plain text and nothing this runner is in the middle of
// could make the expansion of it anything else.
//
// The runner states it checks are the ones under which a literal's text is
// not simply its field: an operand of `${v-word}` that splits its written
// text, `shfileexpansion` reading an `=` first (no `=` is admitted, but the
// option reorders the passes and is not worth reasoning about), and a shell
// already on its way out, where the word expands to nothing.
func (r *Runner) plainLiteralWord(w *syntax.Word) (string, bool) {
	if len(w.Spans) != 1 || r.splitWordLiterals.on || r.shFileExpansion || r.ctl == controlExit {
		return "", false
	}
	s := w.Spans[0]
	if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted || !plainField(s.Value) {
		return "", false
	}
	return s.Value, true
}

// plainLiteralFields is the expansion of a word plainLiteralWord accepted,
// given the one field it came to. What the full road leaves behind it is left
// here too: the word's edges are none, and a brace name armed for it is spent.
func (r *Runner) plainLiteralFields(field []string) []string {
	r.braceName = false
	r.lastWordEdges = wordEdges{}
	return field
}

// plainParamWord is the word's one field, where the word is a single unquoted
// `$name` (or `${name}`) of an ordinary scalar whose value is plain text with
// no separator of IFS's in it.
//
// The front of the issue after the plain literal: an unquoted `$i` was more
// than half of what `: $i` cost, nearly none of it reading the variable (#6353).
// The road it took asks some forty questions of the word and the runner —
// tildes, `=cmd`, braces, list-shaped parameters, references, flags, held
// sources, the field builder, the glob stage — and for a scalar holding a
// number every one of them answers "nothing to do".
//
// Which of those questions could have a different answer is settled before the
// variable is read, so a word this declines is handed to the full road having
// read nothing: the name is a written identifier with nothing else in the
// expansion, and it is not a reference, a produced parameter, an array of any
// kind, a parameter waiting to arrive, or a name a discipline watches. After
// the read, a value that is unset, empty, holds a byte that is not plain text
// or holds one of IFS's bytes is also handed back — reading an ordinary scalar
// twice changes nothing — and the full road answers it from the start.
func (r *Runner) plainParamWord(w *syntax.Word) (string, bool) {
	v, ok := r.plainParamValue(w)
	if !ok {
		return "", false
	}
	if ifs, _ := r.ifs(); containsAnyOf(v, ifs) {
		return "", false
	}
	return v, true
}

// plainParamShortcut switches plainParamValue on. A variable so a test can
// run the same script with the full road answering every word and require the
// same output — see TestAPlainParameterWordAnswersAsTheFullRoadDoes.
var plainParamShortcut = true

// plainParamValue is plainParamWord without the question about IFS, which is
// the whole of it where the word is not split: an assignment's value.
func (r *Runner) plainParamValue(w *syntax.Word) (string, bool) {
	if !plainParamShortcut || w == nil || len(w.Spans) != 1 || r.splitWordLiterals.on || r.shFileExpansion || r.ctl == controlExit ||
		r.disciplined != nil {
		return "", false
	}
	s := &w.Spans[0]
	if s.Kind != syntax.ParamExp || s.Quoting != syntax.Unquoted || !bareNameOnly(s.Param) {
		return "", false
	}
	name := s.Param.Name
	if !r.ordinaryScalarName(name) {
		return "", false
	}
	v, set := r.getVar(name)
	if !set || !plainField(v) || strings.ContainsAny(v, "[]") {
		return "", false
	}
	return v, true
}

// bareNameOnly reports whether a parameter expansion is a name and nothing
// else: no subscript, operator, flag, modifier, nesting or indirection, and a
// name that is an identifier rather than a special or positional parameter.
func bareNameOnly(e *syntax.ParamExpr) bool {
	if e == nil || e.Index != nil || e.BareIndexText != nil || e.BareIndexUnclosed ||
		e.Length || e.Indirect || e.Prefix != 0 || e.Member != "" || e.Inner != nil ||
		e.Op != syntax.ParamNone || e.Arg != nil || e.Arg2 != nil || e.OperandUnreadable != nil ||
		e.RawTail != "" || e.Transform != 0 || e.HasFlags || e.QuoteModifier != 0 ||
		e.IndexFlags != nil || e.IndexRange != nil || e.IndexDots != nil || len(e.Leading) > 0 ||
		e.TildeFlags != 0 || e.SplitFlags != 0 || e.RcExpandFlags != 0 || e.SetTest ||
		e.Bad || e.BadTransform || e.RefusedAtExpansion != nil || e.InsideASubscript {
		return false
	}
	return identifierName(e.Name)
}

// identifierName reports whether name is a shell identifier: a letter or an
// underscore, then letters, digits and underscores.
func identifierName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', c == '_':
		case '0' <= c && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// ordinaryScalarName reports whether reading name is nothing but reading the
// variable store: the name is not a reference, a produced parameter, a
// parameter that has not yet arrived, or any kind of array, each of which the
// full road answers differently from a read of the store or reads with a side
// effect that must not happen twice.
func (r *Runner) ordinaryScalarName(name string) bool {
	// The filter answers for the deferred table, so a name never registered
	// is not looked up in it; see specialnames.go.
	if name == r.pipeStatusName || (r.specialNames.mayHold(name) && r.deferredParams[name]) {
		return false
	}
	if _, ok := r.nameref[name]; ok {
		return false
	}
	// No array, table or producer of any kind: the cell's slots other than
	// the stored scalar are all empty, which is one lookup where it was six.
	return !r.cellHolds(name, slotArray|slotAssoc|slotProducer|slotArrayProducer|slotAssocProducer)
}
