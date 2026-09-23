// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// ProcSubstInSubscriptAsText returns w with every process substitution that
// stands inside an **array subscript** read as the characters it was written
// with, and w itself where there is none.
//
// Inside a subscript a script wrote, `<(` and `>(` are the arithmetic `<` and a
// grouping rather than a substitution. Measured 2026-09-22 from a script file
// under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh`, standard input on the
// null device, bash 5.3.20, with `a=(1 2 3)` and `declare -A A`:
//
//	${a[0<(1)]}          2 — the subscript is `0<1`, which is 1
//	${a[1>(2)]}          1 — and `1>2`, which is 0
//	A[1<(2)]=v           the key is the five characters `1<(2)`
//	[[ a[1<(2)] -lt 9 ]] holds, and nothing is run
//	$(( a[1<(2)] ))      2
//
// and the two controls that say it is the *subscript* rather than the construct
// or the brackets:
//
//	[[ -e <(echo x) ]]   a substitution, at the front of the same operand
//	echo a[1<(2)]        `a[1/dev/fd/63]` — brackets in an ordinary command
//	                     word are no subscript, so the substitution is made
//
// Performing it instead answered a script from a descriptor number divided
// into a subscript: `${a[1<(2)]}` was `1/dev/fd/63: division by 0` and handed
// back nothing where the element is `2`, with a diagnostic naming a path no
// line of the script mentions (#4253).
//
// isSubscript says the word **is** a subscript, so every span in it stands
// inside one. Where it is false the word is scanned for the brackets it writes
// itself, which is what a condition's arithmetic operand needs: one word may
// hold a subscript and the next may open with a substitution, and only the
// brackets part them.
//
// The substitution is replaced rather than refused, because the text is still
// an expression — `0<(1)` reads as `0<1`, parentheses and all — and a reader
// that saw nothing there would take the subscript for `0`.
func ProcSubstInSubscriptAsText(w *Word, isSubscript bool) *Word {
	if w == nil || !wordHoldsProcSubst(w) {
		return w
	}
	var scan ArithBracketScan
	if isSubscript {
		// A subscript word never holds the brackets that opened it, so the
		// depth is stated rather than counted up to.
		scan.Depth = 1
	}
	out := *w
	out.Spans = make([]Span, len(w.Spans))
	copy(out.Spans, w.Spans)
	// advance counts the brackets over text the source wrote, which is the
	// same count interp's own scan of an expanded subscript makes. See
	// ArithValueMark.
	advance := func(text string) {
		for i := 0; i < len(text); i++ {
			switch b := text[i]; {
			case scan.Content(b):
			case b == '[':
				scan.Depth++
			case b == ']':
				if scan.Depth > 0 {
					scan.Depth--
				}
			}
		}
	}
	for i, sp := range out.Spans {
		switch {
		case sp.Kind == Literal && sp.Quoting == Unquoted:
			advance(sp.Value)
		case isProcSubstSpan(sp.Kind) && scan.Depth > 0:
			text := procSubstSourceText(sp)
			out.Spans[i] = Span{
				Kind: Literal, Quoting: Unquoted, Pos: sp.Pos, Value: text,
			}
			// Scanned like the literal text it now is, so a bracket the body
			// happens to hold counts the way a written one does.
			advance(text)
		}
	}
	return &out
}

// wordHoldsProcSubst reports whether any span of w is a process substitution,
// asked first so that the overwhelming majority of subscripts are handed back
// untouched and nothing is copied.
func wordHoldsProcSubst(w *Word) bool {
	for _, sp := range w.Spans {
		if isProcSubstSpan(sp.Kind) {
			return true
		}
	}
	return false
}

// isProcSubstSpan reports whether a span kind is one of the three process
// substitution spellings.
func isProcSubstSpan(k SpanKind) bool {
	return k == ProcSubstIn || k == ProcSubstOut || k == ProcSubstFile
}

// procSubstSourceText is a process substitution span written back out, which
// is its opener, its body and the closing paren.
//
// One function for the two readers that need it — the formatter, which writes
// the program back, and the subscript above, which reads the same characters as
// an expression — because two spellings of "what this span was written with"
// are two chances to disagree about one.
func procSubstSourceText(sp Span) string {
	switch sp.Kind {
	case ProcSubstOut:
		return ">(" + sp.Value + ")"
	case ProcSubstFile:
		return "=(" + sp.Value + ")"
	}
	return "<(" + sp.Value + ")"
}
