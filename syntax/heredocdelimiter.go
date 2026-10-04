// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "strings"

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
	if w != nil && l.dialect.HeredocDelimiterKeepsAQuotedExpansion {
		if d, ok := l.delimiterKeepingQuotedExpansions(w); ok {
			return d
		}
	}
	decode := l.dialect.HeredocDelimiterValue
	if w == nil || decode == nil {
		// Nil is the caller having no decoder, and then the delimiter is its
		// bytes. Spelled as an early return rather than as an identity
		// function so that the shape with nothing to do costs nothing: it is
		// every delimiter in every script a formatter reads.
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

// delimiterKeepingQuotedExpansions is the delimiter read from its written
// text, keeping a double-quoted part that holds an expansion as written. See
// [Dialect.HeredocDelimiterKeepsAQuotedExpansion].
//
// It answers only where such a part is there to keep: everything else —
// nearly every delimiter — goes the ordinary way, and a delimiter holding
// `$'…'` is left to the decoder the caller supplies.
func (l *Lexer) delimiterKeepingQuotedExpansions(w *Word) (string, bool) {
	from, to := int(w.Start.Offset), int(w.Stop.Offset)
	if from < 0 || to > len(l.src) || from >= to {
		return "", false
	}
	raw := l.src[from:to]
	var b []byte
	kept := false
	for i := 0; i < len(raw); i++ {
		switch c := raw[i]; c {
		case '\\':
			if i+1 < len(raw) {
				i++
				b = append(b, raw[i])
			}
		case '\'':
			end := i + 1
			for end < len(raw) && raw[end] != '\'' {
				end++
			}
			b = append(b, raw[i+1:min(end, len(raw))]...)
			i = end
		case '$':
			if i+1 < len(raw) && raw[i+1] == '\'' {
				return "", false
			}
			b = append(b, c)
		case '"':
			end, inner, expands := i+1, []byte(nil), false
			for end < len(raw) && raw[end] != '"' {
				switch raw[end] {
				case '\\':
					if end+1 < len(raw) && strings.IndexByte("\\\"$`", raw[end+1]) >= 0 {
						end++
					}
				case '`':
					expands = true
				case '$':
					if end+1 < len(raw) && startsAnExpansion(raw[end+1]) {
						expands = true
					}
				}
				inner = append(inner, raw[end])
				end++
			}
			if expands {
				b = append(b, raw[i:min(end+1, len(raw))]...)
				kept = true
			} else {
				b = append(b, inner...)
			}
			i = end
		default:
			b = append(b, c)
		}
	}
	return string(b), kept
}

// startsAnExpansion reports whether the byte after a `$` makes it one.
func startsAnExpansion(c byte) bool {
	return c == '{' || c == '(' || isBareParam(c)
}
