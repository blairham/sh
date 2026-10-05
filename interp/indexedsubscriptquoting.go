// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// indexedSubscriptKeepsItsQuoting reports whether an indexed array's
// subscript, written with a quote or a backslash in it, is handed to the
// arithmetic the way the inside of `$(( ))` is: expanded as a double-quoted
// string, so an apostrophe and a backslash before an ordinary character stay
// characters of the expression. See
// Semantics.IndexedSubscriptKeepsItsQuoting.
//
// Asked only where the written subscript holds one of those characters and
// the name is not an associative array — an indexed one, a scalar, or one not
// set yet, which a store makes indexed — so the common path asks nothing: a
// subscript with no quoting in it comes to the same text either way.
func (r *Runner) indexedSubscriptKeepsItsQuoting(written, name string) bool {
	if name == "" || !strings.ContainsAny(written, `'\`) {
		return false
	}
	if assoc := r.assocDeclared(r.throughNameref(name)); assoc {
		// A key, which is SubscriptIsAQuotingContext's question.
		return false
	}
	return r.ask(r.sem().IndexedSubscriptKeepsItsQuoting,
		"an indexed array's subscript keeping its quoting for the arithmetic")
}

// removeSubscriptQuoting takes the quoting off a subscript's text the way a
// word's quote removal would — what an apostrophe pair holds is kept as it
// stands, a double quotation keeps what it holds less the backslash before
// `$`, a backquote, `"`, `\` and a newline, and an unquoted backslash keeps
// the character after it — for the dialect that reads an indexed subscript
// without its quoting. See Semantics.IndexedSubscriptKeepsItsQuoting.
func removeSubscriptQuoting(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		switch c := text[i]; c {
		case '\'':
			j := strings.IndexByte(text[i+1:], '\'')
			if j < 0 {
				b.WriteString(text[i:])
				return b.String()
			}
			b.WriteString(text[i+1 : i+1+j])
			i += j + 1
		case '"':
			for i++; i < len(text) && text[i] != '"'; i++ {
				if text[i] == '\\' && i+1 < len(text) && strings.IndexByte("$`\"\\\n", text[i+1]) >= 0 {
					i++
				}
				b.WriteByte(text[i])
			}
		case '\\':
			if i+1 < len(text) {
				i++
				b.WriteByte(text[i])
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
