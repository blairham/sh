// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// heredocDelimiter is the text a here-document ends at, which is the
// delimiter's **value** and not always the bytes it was written as.
//
// `cat <<$'E\tOF'` ends at a line reading `E<TAB>OF` in every reference shell
// that has `$'…'`; the raw spelling never appears in the script again. Word
// alone cannot answer it, because the escape set is a column of semantics axes
// and nothing here may hold one — see [Dialect.HeredocDelimiterValue], which is
// where the caller's decoder arrives.
//
// Span by span rather than over Word.Literal, because a delimiter may be part
// quoted and part not: `E$'\t'OF` is three spans and only the middle one is
// decoded. Literal's own fast paths are kept for the shape that is nearly every
// delimiter there is — one span, nothing to decode.
func (l *Lexer) heredocDelimiter(w *Word) string {
	decode := l.dialect.HeredocDelimiterValue
	if w == nil || decode == nil {
		return w.Literal()
	}
	var decodes bool
	for _, s := range w.Spans {
		if s.Quoting == DollarSingleQuoted {
			decodes = true
			break
		}
	}
	if !decodes {
		return w.Literal()
	}
	var b []byte
	for _, s := range w.Spans {
		if s.Quoting == DollarSingleQuoted {
			b = append(b, decode(s.Value)...)
			continue
		}
		b = append(b, s.Value...)
	}
	return string(b)
}
