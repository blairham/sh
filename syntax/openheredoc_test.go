// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// `OpenQuote` says a here-document is what the input ran out inside, and
// spells both delimiters `<<` because what the next physical line *begins
// inside* is the same either way. `OpenHeredocExpands` is the other half:
// whether that document's body is shell text.
//
// A reader taking a line at a time needs both. A body under an unquoted
// delimiter has its expansions done and its line continuations resolved
// before anything else sees the line; under a quoted one it does not, and a
// reader recording what it read has to know which happened to it (#4103).
//
// The inner rows are the ones a flag on the outer lexer alone would get
// wrong: the document belongs to a program between parentheses, whose read
// is a lexer of its own.
func TestOpenHeredocExpandsSaysWhetherTheBodyIsShellText(t *testing.T) {
	for _, tc := range []struct {
		name    string
		src     string
		open    string
		expands bool
	}{
		{"an unquoted delimiter", "cat <<EOD\nx\n", "<<", true},
		{"a single-quoted delimiter", "cat <<'EOD'\nx\n", "<<", false},
		{"a double-quoted delimiter", "cat <<\"EOD\"\nx\n", "<<", false},
		{"a backslash in the delimiter", "cat <<\\EOD\nx\n", "<<", false},
		{"a stripping operator keeps the answer", "cat <<-EOD\nx\n", "<<", true},
		{"one inside a substitution", "echo $(cat <<EOD\nx\n", "<<", true},
		{"a quoted one inside a substitution", "echo $(cat <<'EOD'\nx\n", "<<", false},
		{"a quote is not a here-document", "echo \"a\n", `"`, false},
		{"nothing open is not one either", "echo one\n", "", false},
		{"a document that closed is not open", "cat <<EOD\nx\nEOD\necho \\\n", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := syntax.NewParser(tc.src, syntax.Dialect{})
			p.Parse()
			if got := p.OpenQuote(); got != tc.open {
				t.Errorf("OpenQuote = %q, want %q", got, tc.open)
			}
			if got := p.OpenHeredocExpands(); got != tc.expands {
				t.Errorf("OpenHeredocExpands = %v, want %v", got, tc.expands)
			}
		})
	}
}
