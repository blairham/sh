// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// stoppedArithSpans names the expansions an apostrophe **written in the
// source, inside a subscript** stops, so they are never performed.
//
// It hands back a lookup by span index for [Runner.expandSpansWith], and nil
// where nothing is stopped — which is every expression without an apostrophe
// between brackets, and every dialect that performs what one holds. See
// Semantics.WrittenSubscriptQuotationStopsItsExpansion for the rows.
//
// The decision has to be made from the spans and before any of them has run,
// because an expansion that is stopped must not have happened: a `$(cmd)` in
// there is a command nobody ran, and a hook handed the *result* is a hook
// called after the only thing that mattered. That is the whole reason
// expandSpansWith exists.
//
// Only an apostrophe, and only inside brackets. A double quotation's contents
// are performed in every column — that is the control the axis rests on — and
// an apostrophe outside a subscript is not this question: `(( '1' ))` is a
// refusal everywhere rather than a quoted operand. The bracket depth and the
// quotation are read with syntax.ArithBracketScan, which is the scan the
// parser will make over the same bytes, so the two readings of the same
// brackets cannot part.
func (r *Runner) stoppedArithSpans(text string, spans []syntax.Span) func(int) (string, bool) {
	if !strings.Contains(text, "'") || !strings.Contains(text, "[") {
		return nil
	}
	var scan syntax.ArithBracketScan
	quoted := r.dialect().ArithSubscriptQuoting
	var stopped map[int]string
	for i, s := range spans {
		start, end := int(s.Pos.Offset), len(text)
		if i+1 < len(spans) {
			end = int(spans[i+1].Pos.Offset)
		}
		if start < 0 || end > len(text) || start > end {
			// The spans do not line up with the text they were read from,
			// which is nothing this reading can recover: answer as though
			// nothing were stopped rather than slicing at a guess.
			return nil
		}
		if s.Kind == syntax.Literal {
			for j := start; j < end; j++ {
				switch b := text[j]; {
				case quoted && scan.Content(b):
				case b == '[':
					scan.Depth++
				case b == ']':
					scan.Depth--
				}
			}
			continue
		}
		if scan.Depth <= 0 || scan.Quote() != '\'' {
			continue
		}
		if stopped == nil {
			stopped = map[int]string{}
		}
		// Written out **marked**, which is the same claim the apostrophes
		// around it make: these bytes are characters of the key now, and the
		// `$` among them begins nothing on any later reading. The apostrophes
		// themselves stay unmarked, because taking them off is quote
		// removal's to do and the dialect that stops the expansion removes
		// them — so `'$kq'` comes through as `'$kq'` with its middle sealed,
		// and the key is the three characters `$kq`.
		stopped[i] = markArithValue(text[start:end])
	}
	if stopped == nil {
		return nil
	}
	if !r.ask(r.sem().WrittenSubscriptQuotationStopsItsExpansion,
		"an apostrophe written inside a subscript stopping the expansion it holds") {
		return nil
	}
	return func(i int) (string, bool) {
		out, ok := stopped[i]
		return out, ok
	}
}
