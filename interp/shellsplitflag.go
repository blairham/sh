// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// The `${(Z:opts:)v}` expansion flag: split a value the way the shell would
// split a command line, with option letters between a pair of delimiters.
//
// Measured on zsh 5.9.2, the only shell in the panel whose grammar has the
// flag at all. The other five read `${(Z…)v}` as a bad substitution or refuse
// it while reading, which is the same three-way split every one of these
// flags follows.
//
// The split itself is [syntax.ShellWords], which is the lexer. A scanner
// written here instead was the obvious shape and is the shape this repository
// keeps having to undo — scanGroupSpans kept its own copy of scanWord's case
// list and lost every expansion inside a pattern group (#1331), and three
// raw scans each wrote their own comment rule and two of them did not have
// one (#1397). `(Z)` means "split like the shell"; the shell's splitting is
// the lexer; a copy of it would agree on `a b` and then part company over
// `a"b c"d`, `$(f x)`, `a#b` and every other place a word boundary is not a
// blank.

// shellSplitOpts is the `Z` flag's argument, and whether the flag does
// anything at all.
//
// An empty option list turns the flag *off* rather than splitting with no
// options set, which is measured and is the opposite of the reading a name
// like "the same split, with options" invites: on `a  b`, `${(Z::)v}` is the
// value unchanged with both spaces, where `${(z)v}` is `a b`. So the letter
// being present is not the question — the argument being non-empty is.
func shellSplitOpts(e *syntax.ParamExpr) (string, bool) {
	if !strings.ContainsRune(e.Flags, 'Z') {
		return "", false
	}
	return e.ShellSplitOpts, e.ShellSplitOpts != ""
}

// shellSplitActive reports whether this group splits its value into shell
// words: the same question shellSplitOpts answers, asked where the options
// themselves are not wanted.
func shellSplitActive(e *syntax.ParamExpr) bool {
	_, ok := shellSplitOpts(e)
	return ok
}

// splitShellWords is one word's worth of the `Z` split.
//
// The option letters, measured one at a time and in combination:
//
//	         `a # hi<LF>b`   what the letters do
//	(none)   a  #  hi  ;  b  no comments at all, and a newline is its own word
//	c        a  '# hi'  ;  b the comment is one word, `#` and all
//	C        a  ;  b         the comment is dropped
//	n        a  #  hi  b     the newline is ordinary whitespace
//	Cn       a  b            which is what a plugin manager's formatter writes
//
// The `#` rows are the ones worth reading twice. Without `c` or `C` there is
// no comment rule, so `a # hi` is three words and `a #hi` is two — the same
// answer an interactive shell without INTERACTIVE_COMMENTS gives, which is
// what "split like the shell would" means for a *value* somebody typed.
//
// A newline that is not blank comes back as `;` rather than as a newline:
// measured, `$'a\n\n\nb'` is five words and the three middle ones are each a
// single `;`.
//
// A word with no shell words in it — blanks, or a comment that `C` dropped —
// contributes *no* field, and the one empty field a wholly empty result comes
// to is added once at the end rather than once per word. The difference is
// visible and is measured: with `a=(” x)`, `"${(@Z+n+)a}"` is the single
// field `x` on the shell that has the flag, where a per-word empty would make
// it two — and `v=""; set -- "${(Z+n+)v}"` is still one parameter, the
// unquoted spelling none, the empty field being dropped at the edge rather
// than never made. See splitShellWordsAll.
func (r *Runner) splitShellWords(w string, opts string) []string {
	return syntax.ShellWords(w, r.dialect(), syntax.ShellSplit{
		Comments:       shellSplitComments(opts),
		NewlineIsBlank: strings.ContainsRune(opts, 'n'),
	})
}

// splitShellWordsAll is the `Z` split over every word the pipeline has so far.
//
// The empty result is the whole of why this is a function rather than a loop
// at the call site: nothing at all becomes one empty field, and that is a
// statement about the *expansion* and not about any one word in it. Measured
// on zsh 5.9.2 — `a=(); set -- "${(@Z+n+)a}"` is one parameter and
// `"${(@)a}"` on the same array is none, so the split is what makes the
// field rather than the array having had one.
func (r *Runner) splitShellWordsAll(words []string, opts string) []string {
	split := make([]string, 0, len(words))
	for _, w := range words {
		split = append(split, r.splitShellWords(w, opts)...)
	}
	if len(split) == 0 {
		return []string{""}
	}
	return split
}

// shellSplitComments turns the option letters into the lexer's comment rule.
//
// `c` wins over `C` where both are written, and it is not the last letter
// that decides: measured on `a # hi`, `${(Z+cC+)v}` and `${(Z+Cc+)v}` are
// both `a` and `# hi` — the comment kept, which is what `c` alone answers,
// where `C` alone answers the single word `a`.
func shellSplitComments(opts string) syntax.CommentMode {
	switch {
	case strings.ContainsRune(opts, 'c'):
		return syntax.CommentsKept
	case strings.ContainsRune(opts, 'C'):
		return syntax.CommentsSkipped
	}
	return syntax.CommentsOrdinaryText
}
