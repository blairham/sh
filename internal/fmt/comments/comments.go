// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package comments recovers the comments the substrate's parser discards.
//
// The rules are docs/comments.md, measured across the five-shell panel: a
// comment is exactly a '#' standing where a token could begin, outside every
// word and here-document extent, running up to and not through the newline.
// Given a parse tree with exact extents, subtraction of those extents from
// the source is therefore a complete recovery — no lexer support needed.
package comments

import (
	"sort"

	"github.com/blairham/sh/syntax"

	"github.com/blairham/sh/internal/fmt/walk"
)

// Comment is one recovered comment.
type Comment struct {
	// Pos locates the '#' in the source the tree was parsed from.
	Pos syntax.Pos
	// Text is the comment as written, '#' included, newline excluded.
	Text string
}

// Recover returns every comment in src, in source order. f must be the tree
// src parsed to; extents from any other input are meaningless here.
func Recover(src string, f *syntax.File) []Comment {
	ex := excluded(f)
	var out []Comment
	line, lineStart := 1, 0
	xi := 0
	for i := 0; i < len(src); i++ {
		for xi < len(ex) && ex[xi].to <= i {
			xi++
		}
		c := src[i]
		if c == '\n' {
			line++
			lineStart = i + 1
			continue
		}
		if xi < len(ex) && i >= ex[xi].from {
			continue // inside a word or heredoc extent
		}
		if c != '#' || !opensComment(src, i) {
			continue
		}
		end := i
		for end < len(src) && src[end] != '\n' {
			end++
		}
		out = append(out, Comment{
			Pos:  syntax.Pos{Offset: int32(i), Line: int32(line), Col: int32(i - lineStart + 1)},
			Text: src[i:end],
		})
		i = end - 1 // resume at the newline so the line count stays right
	}
	return out
}

// opensComment applies rule 1 of docs/comments.md: a '#' opens a comment only
// where a new token could begin. Mid-word it is a literal — and a mid-word
// '#' with no word extent over it exists: a function's name is stored as a
// string, so `a#b() { :; }` puts one right here.
func opensComment(src string, i int) bool {
	if i == 0 {
		return true
	}
	switch src[i-1] {
	case ' ', '\t', '\n', ';', '&', '|', '(', ')':
		return true
	}
	return false
}

type span struct{ from, to int }

// excluded collects the source extents a '#' can sit inside without being a
// comment: every word (quoting, expansions and here-document bodies live
// inside word extents), every assignment (its name and subscript are strings,
// not words), and the opaque constructs whose interiors are kept as text —
// `[[ ]]`, `(( ))`, and an arithmetic for's header.
func excluded(f *syntax.File) []span {
	var ex []span
	add := func(from, to int) {
		if to > from {
			ex = append(ex, span{from, to})
		}
	}
	walk.Nodes(f, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.Word:
			add(int(x.Start.Offset), int(x.Stop.Offset))
		case *syntax.Assign:
			add(int(x.Start.Offset), int(x.Stop.Offset))
			return false
		case *syntax.TestClause:
			add(int(x.Start.Offset), int(x.Stop.Offset))
			// Fall through to redirections via the walker: they sit
			// outside [Start, Stop) and carry words of their own.
		case *syntax.ArithCmdClause:
			add(int(x.Start.Offset), int(x.Stop.Offset))
		case *syntax.ForArithClause:
			add(int(x.Start.Offset), int(x.Start.Offset)+len(x.Header))
		}
		return true
	})
	sort.Slice(ex, func(i, j int) bool { return ex[i].from < ex[j].from })
	// Merge overlaps so the scan's cursor never has to back up.
	merged := ex[:0]
	for _, s := range ex {
		if n := len(merged); n > 0 && s.from <= merged[n-1].to {
			if s.to > merged[n-1].to {
				merged[n-1].to = s.to
			}
			continue
		}
		merged = append(merged, s)
	}
	return merged
}
