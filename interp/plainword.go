// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

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
