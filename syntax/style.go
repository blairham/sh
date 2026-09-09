// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// Laying source back out, as opposed to printing a tree back as source.
//
// [Layout] is the arrangement a *canonicalizer* uses: `type f` and `export -f`
// need the tree said again, and what comes back is the tree's spelling rather
// than the author's. A formatter is the other job. It rewrites a file the
// author will read next, so it may not respell anything — and it therefore
// works from the source text, emitting every token by its extent and owning
// only the space between them.
//
// So the two do not share a type. What they share is the shape of the
// arrangement: a set of named questions, asked centrally, answered by each
// shell in its own package. docs/spec/style.md holds the measurements behind
// every answer.

// Style is how a formatter lays a script out — the questions the grammar does
// not ask, answered per dialect by `dialect/<shell>.Style()`.
//
// Nothing here names a shell, and nothing here decides what a token says. A
// field belongs in this struct only if a dialect could answer it differently
// or a named style guide put it up for decision; docs/spec/style.md records
// which fields currently have one answer across all four dialects, and on what
// evidence.
type Style struct {
	// Indent is one level of indentation. Two spaces everywhere measured:
	// the zsh distribution and a third-party plugin tree agree at 92% and
	// 97%, bash leans 60/36 and the Google Shell Style Guide settles it,
	// and this tree's .editorconfig says the same.
	//
	// A `<<-` here-document body is unaffected however this reads, because
	// bodies are emitted verbatim — which is also what keeps the tabs that
	// make `<<-` strip anything at all.
	Indent string

	// ThenOnHeaderLine and DoOnHeaderLine put the keyword that opens a body
	// on the line of its header — `if cond; then`, `for x in …; do` — rather
	// than on a line of its own.
	//
	// Two fields and not one because a shell may answer them differently;
	// none measured does. The header's line wins by 13:1 or better in every
	// dialect with a sample worth counting.
	ThenOnHeaderLine bool
	DoOnHeaderLine   bool

	// BraceShortForm says what becomes of a body the author wrote with
	// braces where the dialect also has the keyword spelling.
	//
	// This is the one field the dialects disagree on. zsh alone spells
	// `if cond { … }`, `for f ( a b c ) { … }`, `while cond { … }` and
	// `repeat n { … }`, and those parse to the same tree as the keyword
	// form — no AST field separates them. A formatter printing from the
	// tree alone therefore rewrites one spelling into the other silently,
	// across some five hundred lines of a real zsh tree.
	//
	// Where no other dialect can parse a brace body at all, the answer
	// costs nothing; zsh's is the one that matters.
	BraceShortForm ShortForm

	// AlignTrailingComments lines up the `#` of consecutive trailing
	// comments at one indent, one space past the run's longest code line. A
	// run of one keeps a single space.
	//
	// Computed rather than inherited from the author's spaces, so the column
	// is consistent by construction — which is the difference between
	// aligning and preserving padding.
	AlignTrailingComments bool

	// MaxBlankLines is how many consecutive blank lines survive between
	// items. gofmt's rule, adopted: one, and none directly under an opener.
	MaxBlankLines int
}

// ShortForm is what a formatter does with a brace-spelled body.
type ShortForm int

const (
	// PreserveShortForm writes the body back with the spelling the author
	// used, read from the source rather than from the tree.
	PreserveShortForm ShortForm = iota
	// ExpandShortForm writes the keyword spelling however the body was
	// written. The portable form, and the only reachable answer in a
	// dialect that cannot parse the other one.
	ExpandShortForm
)

func (s ShortForm) String() string {
	if s == ExpandShortForm {
		return "expand"
	}
	return "preserve"
}

// CoreStyle is the arrangement for the core language — the common denominator,
// and what a file that declares no dialect is laid out with.
//
// Every dialect preset starts here and overrides what it was measured to
// answer differently, which today is one field in one shell. Starting from the
// core rather than from a sibling is the same rule the other three vectors
// follow, and for the reason `dialect/doc.go` gives: a preset that inherits
// from a sibling inherits its future mistakes.
func CoreStyle() Style {
	return Style{
		Indent:                "  ",
		ThenOnHeaderLine:      true,
		DoOnHeaderLine:        true,
		BraceShortForm:        ExpandShortForm,
		AlignTrailingComments: true,
		MaxBlankLines:         1,
	}
}
