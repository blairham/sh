// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A here-document's delimiter is subject to quote removal and to nothing
// else, so a `$` written in one is an ordinary character: `<<$d` waits for a
// line reading `$d`, and the body under it is still expanded.
//
// Measured 2026-09-14 against bash 5.3.15, ksh93u+, zsh 5.9.2 and dash, each
// running the same script from a file. All four print the expanded body and
// then carry on past the `$d` line, so this is a correction rather than an
// axis.
//
// The delimiter used to be the word's spans *joined*, which is the text with
// each construct's own delimiters already removed — `$d` arrived as `d`, the
// line that was written as the delimiter never matched, and everything under
// it was swallowed as the body (#2745). That is the expensive half: one
// mis-read delimiter costs the rest of the file.
func TestADelimiterIsSubjectToQuoteRemovalAndNothingElse(t *testing.T) {
	for _, tc := range []struct{ name, written, delim string }{
		{"a bare parameter stays as written", "$d", "$d"},
		{"a braced one too", "${d}", "${d}"},
		{"one attached to text", "a$d", "a$d"},
		{"a command substitution is its own text", "$(echo X)", "$(echo X)"},
		{"a backquoted one is too", "`echo X`", "`echo X`"},
		{"arithmetic is text as well", "$((1+1))", "$((1+1))"},
		{"double quotes remove and keep the dollar", `"$d"`, "$d"},
		{"single quotes the same", `'$d'`, "$d"},
		{"a backslash quotes the dollar on its own", `\$d`, "$d"},
		{"a plain delimiter is unchanged", "EOF", "EOF"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "cat <<" + tc.written + "\nbody\n" + tc.delim + "\n"
			p := syntax.NewParser(src, syntax.Core())
			f := p.Parse()
			if err := p.Err(); err != nil {
				t.Fatalf("parse %q: %v", src, err)
			}
			r := theRedirect(t, f)
			if got := r.Word.Literal(); got != tc.delim {
				t.Errorf("delimiter = %q, want %q", got, tc.delim)
			}
			// The delimiter arrived, so the body is the one line above it
			// and not the rest of the input.
			if got := r.Heredoc.Literal(); got != "body\n" {
				t.Errorf("body = %q, want %q", got, "body\n")
			}
		})
	}
}

// Quoting is not expansion, and the two are told apart by what quote removal
// *did*: a delimiter that lost characters to it was quoted, and a body under
// a quoted delimiter is literal throughout. So `<<$d` leaves the body
// expandable where `<<"$d"` does not, even though both delimiters are `$d`.
func TestWhetherTheBodyIsLiteralFollowsTheQuotingAndNotTheDollar(t *testing.T) {
	for _, tc := range []struct {
		name, written, delim string
		literal              bool
	}{
		{"an unquoted dollar leaves the body expandable", "$d", "$d", false},
		{"a substitution in the delimiter does too", "$(echo X)", "$(echo X)", false},
		{"double quotes make it literal", `"$d"`, "$d", true},
		{"so does a backslash", `\$d`, "$d", true},
		{"and single quotes", `'EOF'`, "EOF", true},
		{"a plain delimiter leaves it expandable", "EOF", "EOF", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "cat <<" + tc.written + "\n$HOME\n" + tc.delim + "\n"
			p := syntax.NewParser(src, syntax.Core())
			f := p.Parse()
			if err := p.Err(); err != nil {
				t.Fatalf("parse %q: %v", src, err)
			}
			r := theRedirect(t, f)
			// A literal body is marked single-quoted, which is what stops
			// the interpreter reading it again.
			got := r.Heredoc.Spans[0].Quoting == syntax.SingleQuoted
			if got != tc.literal {
				t.Errorf("body literal = %v, want %v", got, tc.literal)
			}
		})
	}
}

// `$'` and `$"` open quoted runs rather than substitutions, so they are quote
// removal's business and still apply inside a delimiter: `<<$'a'` is
// delimited by `a`. Measured on bash 5.3.15 and zsh 5.9.2, 2026-09-14.
func TestADollarQuotedRunInADelimiterIsStillQuoting(t *testing.T) {
	d := syntax.Core()
	d.DollarSingleQuote = true
	d.DollarDoubleQuote = true
	for _, tc := range []struct{ name, written, delim string }{
		{"dollar-single", `$'a'`, "a"},
		{"dollar-double inside text", `E$"x"F`, "ExF"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "cat <<" + tc.written + "\nbody\n" + tc.delim + "\n"
			p := syntax.NewParser(src, d)
			f := p.Parse()
			if err := p.Err(); err != nil {
				t.Fatalf("parse %q: %v", src, err)
			}
			if got := theRedirect(t, f).Word.Literal(); got != tc.delim {
				t.Errorf("delimiter = %q, want %q", got, tc.delim)
			}
		})
	}
}

func theRedirect(t *testing.T, f *syntax.File) *syntax.Redirect {
	t.Helper()
	sc := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd)
	if len(sc.Redirs) != 1 || sc.Redirs[0].Heredoc == nil {
		t.Fatalf("no here-document")
	}
	return sc.Redirs[0]
}
