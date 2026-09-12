// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// braceScanExtent is where a `${ … }` written in src ends, said as the text
// the enclosing word holds after it. It is the whole of what
// QuoteProtectsTheClosingBrace decides: a quote that protects makes the
// expansion run to the second brace and leaves nothing behind, and one that
// does not ends it at the first and leaves the rest as literal characters.
func braceScanExtent(t *testing.T, src string, d Dialect) (body, tail string) {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	for _, w := range sc.Args {
		seen := false
		var after strings.Builder
		for _, s := range w.Spans {
			switch {
			case (s.Kind == ParamExp || (s.Kind == CommandSubst && s.CurrentShell)) && !seen:
				seen, body = true, s.Value
			case seen:
				after.WriteString(s.Value)
			}
		}
		if seen {
			return body, after.String()
		}
	}
	t.Fatalf("no braced expansion in %q", src)
	return "", ""
}

// A single quote written inside a double-quoted `${ … }` protects the closing
// brace in some operands and not others, and which is a grammar question with
// three answers rather than two. See Dialect.QuoteProtectsTheClosingBrace.
func TestQuoteProtectsTheClosingBrace(t *testing.T) {
	word := `echo "[${v-'a}b'}]"`
	pattern := `echo "[${v#'a}b'}]"`

	for _, tc := range []struct {
		name       string
		policy     BraceQuotePolicy
		src        string
		body, tail string
	}{
		// The core reading: a pattern's quotes are its own, and a word
		// operand's belong to the double quote around the whole expansion,
		// where a single quote quotes nothing.
		{"a pattern only, in a word operand", BraceQuoteProtectsAPatternOnly, word, `v-'a`, `b'}]`},
		{"a pattern only, in a pattern operand", BraceQuoteProtectsAPatternOnly, pattern, `v#'a}b'`, `]`},
		// Every operand: the quote runs the scan past the brace wherever it
		// is written.
		{"every operand, in a word operand", BraceQuoteProtectsEveryOperand, word, `v-'a}b'`, `]`},
		{"every operand, in a pattern operand", BraceQuoteProtectsEveryOperand, pattern, `v#'a}b'`, `]`},
		// Nothing: the expansion always ends at the first `}`.
		{"nothing, in a word operand", BraceQuoteProtectsNothing, word, `v-'a`, `b'}]`},
		{"nothing, in a pattern operand", BraceQuoteProtectsNothing, pattern, `v#'a`, `b'}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := Core()
			d.QuoteProtectsTheClosingBrace = tc.policy
			body, tail := braceScanExtent(t, tc.src, d)
			if body != tc.body || tail != tc.tail {
				t.Errorf("body %q tail %q, want body %q tail %q", body, tail, tc.body, tc.tail)
			}
		})
	}
}

// The three readings the flag is deliberately not consulted for, each
// unanimous across the panel. Without these the flag reads as "a quote in an
// expansion", which is wider than what was measured.
func TestQuoteProtectsTheClosingBraceBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name       string
		src        string
		body, tail string
	}{
		// Unquoted, every column protects, in both kinds of operand.
		{"unquoted, a word operand", `echo [${v-'a}b'}]`, `v-'a}b'`, `]`},
		{"unquoted, a pattern operand", `echo [${v#'a}b'}]`, `v#'a}b'`, `]`},
		// A double quote protects in every column and in both kinds.
		{"a double quote in a word operand", `echo "[${v-"a}b"}]"`, `v-"a}b"`, `]`},
		{"a double quote in a pattern operand", `echo "[${v#"a}b"}]"`, `v#"a}b"`, `]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, p := range []BraceQuotePolicy{
				BraceQuoteProtectsAPatternOnly, BraceQuoteProtectsNothing, BraceQuoteProtectsEveryOperand,
			} {
				d := Core()
				d.QuoteProtectsTheClosingBrace = p
				body, tail := braceScanExtent(t, tc.src, d)
				if body != tc.body || tail != tc.tail {
					t.Errorf("%v: body %q tail %q, want body %q tail %q", p, body, tail, tc.body, tc.tail)
				}
			}
		})
	}
}

// A dialect without `${x/pat/rep}` has no `/` operator, so nothing there
// introduces a pattern for a quote to be honored in.
func TestASlashIsAPatternOperatorOnlyWhereTheFormExists(t *testing.T) {
	d := Core()
	if !d.ParamSubstitution {
		t.Fatal("the core is expected to have the replacement form")
	}
	if body, tail := braceScanExtent(t, `echo "[${v/'a}b'/z}]"`, d); body != `v/'a}b'/z` || tail != `]` {
		t.Errorf("with the form: body %q tail %q, want the quote honored", body, tail)
	}
	d.ParamSubstitution = false
	if body, tail := braceScanExtent(t, `echo "[${v/'a}b'/z}]"`, d); body != `v/'a` || tail != `b'/z}]` {
		t.Errorf("without the form: body %q tail %q, want the quote ordinary", body, tail)
	}
}

// One expansion written inside another's operand: the innermost one decides
// what a quote written in it does, measured six columns to one.
func TestTheInnermostOperandDecidesWhatAQuoteDoes(t *testing.T) {
	d := Core()
	d.NestedParamExpansion = true
	// The quote stands in the inner expansion's *pattern* operand while the
	// inner expansion stands in the outer's *word* operand. Asking the outer
	// answers `w#'W` and leaves `'}}]` behind.
	if body, tail := braceScanExtent(t, `echo "[${v-${w#'W}'}}]"`, d); body != `v-${w#'W}'}` || tail != `]` {
		t.Errorf("body %q tail %q, want the inner pattern operand to govern", body, tail)
	}
}

// A here-document body carries the double-quoted reading, which is the same
// rule that substitutes a parameter written in one. Measured: bash 5.3 and
// 3.2 write `[Vx}y]` there and the other five `[Vx}yb'}]`, exactly as they do
// inside `""`.
func TestAHereDocumentBodyTakesTheQuotedReading(t *testing.T) {
	for _, tc := range []struct {
		policy     BraceQuotePolicy
		body, tail string
	}{
		{BraceQuoteProtectsAPatternOnly, `v-'a`, "b'}]\n"},
		{BraceQuoteProtectsEveryOperand, `v-'a}b'`, "]\n"},
		{BraceQuoteProtectsNothing, `v-'a`, "b'}]\n"},
	} {
		d := Core()
		d.QuoteProtectsTheClosingBrace = tc.policy
		spans, err := HeredocSpans("[${v-'a}b'}]\n", d)
		if err != nil {
			t.Fatalf("%v: %v", tc.policy, err)
		}
		var body string
		var after strings.Builder
		seen := false
		for _, s := range spans {
			switch {
			case s.Kind == ParamExp && !seen:
				seen, body = true, s.Value
			case seen:
				after.WriteString(s.Value)
			}
		}
		if body != tc.body || after.String() != tc.tail {
			t.Errorf("%v: body %q tail %q, want body %q tail %q", tc.policy, body, after.String(), tc.body, tc.tail)
		}
	}
}

// The `${ cmd;}` command form keeps its own rule, in every reading of the
// flag: its body is a program, so its quotes are that program's for the same
// reason its braces are. Measured `printf '[%s]' "${ echo '}' ;}"`, which is
// `[}]` in the two panel columns that have the construct.
func TestTheCommandFormsQuotesAreItsProgramsInEveryReading(t *testing.T) {
	for _, p := range []BraceQuotePolicy{
		BraceQuoteProtectsAPatternOnly, BraceQuoteProtectsNothing, BraceQuoteProtectsEveryOperand,
	} {
		d := Core()
		d.CurrentShellSubstitution = true
		d.QuoteProtectsTheClosingBrace = p
		body, tail := braceScanExtent(t, `echo "[${ echo '}' ;}]"`, d)
		if body != ` echo '}' ;` || tail != `]` {
			t.Errorf("%v: body %q tail %q, want the program read whole", p, body, tail)
		}
	}
}

// A subscript belongs to the name, so the operator a quote is judged against
// is what follows the `]` rather than the `[`. Measured
// `a=(p a}b); printf '[%s]' "${a[1]#'a}'}"`, which is `[b]` in every column
// with the construct but zsh.
func TestASubscriptIsPartOfTheNameNotTheOperand(t *testing.T) {
	d := Core()
	d.QuoteProtectsTheClosingBrace = BraceQuoteProtectsAPatternOnly
	if body, tail := braceScanExtent(t, `echo "[${a[1]#'a}b'}]"`, d); body != `a[1]#'a}b'` || tail != `]` {
		t.Errorf("body %q tail %q, want the operator read after the subscript", body, tail)
	}
	// The word operand through a subscript is the other side of the same
	// reading, so a fix that skipped to the end of the body would pass the
	// row above and fail this one.
	if body, tail := braceScanExtent(t, `echo "[${a[1]-'a}b'}]"`, d); body != `a[1]-'a` || tail != `b'}]` {
		t.Errorf("body %q tail %q, want the quote ordinary in a word operand", body, tail)
	}
}
