// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// literalIs and appendLiteral answer what Literal answers without building
// it, so they are held to Literal over words of one span and of several,
// against the word's own text, a prefix of it, a longer string, and the
// reserved words the parser actually asks about (#6381).
func TestLiteralIsAgreesWithLiteral(t *testing.T) {
	t.Parallel()
	words := []string{
		"if", "fi", "then", "n=$((n+1))", `"$a0"`, "a${b}c", "x'y'z", `\if`,
		"i'f'", "$x", "do", "''", "a$b", "{", "}", "in", "end", "typeset",
	}
	for _, src := range words {
		for _, d := range []Dialect{Core(), POSIX()} {
			tok := NewLexer(src, d).Next()
			if tok.Kind != TokWord {
				t.Fatalf("%q lexed as %v, want a word", src, tok.Kind)
			}
			lit := tok.Literal()
			if got := string(tok.appendLiteral(nil)); got != lit {
				t.Errorf("%q: appendLiteral = %q, Literal = %q", src, got, lit)
			}
			probes := append([]string{lit, lit + "x", ""}, words...)
			if lit != "" {
				probes = append(probes, lit[:len(lit)-1], lit[1:])
			}
			for _, s := range probes {
				if got, want := tok.literalIs(s), lit == s; got != want {
					t.Errorf("%q: literalIs(%q) = %v, Literal is %q", src, s, got, lit)
				}
			}
		}
	}
}
