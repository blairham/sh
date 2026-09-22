// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// The operand of a `${ … }` is cut into its parts by a scan of its own, and
// that scan has to know `$'…'` is one construct too (#4169).
//
// #3896 taught the scan for the closing *brace* the rule and left the two
// scans behind it reading a plain `'…'` run, where a backslash is ordinary:
// the one that finds the `/` between a pattern and its replacement, and the
// one that looks for a quote nothing closes. So the run of an escaped quote
// ended at that quote, the one behind it opened a second run, and the
// separator disappeared into it. A replacement whose pattern is an escaped
// quote came back holding the whole operand and a stray quote span behind it,
// which matched nothing where every reference replaces.
//
// The rows are about [Dialect.DollarSingleQuote] and name no shell. What the
// reference columns print is in dialect/bash/dollarsingleinbracequoted_test.go
// and its three siblings.
func TestADollarSingleQuoteInABraceOperandIsOneConstruct(t *testing.T) {
	t.Parallel()
	on := Core()
	on.DollarSingleQuote = true
	for _, tc := range []struct {
		name string
		src  string
		arg  string // the pattern operand, joined
		arg2 string // the replacement operand, joined
	}{
		// The case the issue was filed on: the separator stands behind the
		// run's closing quote and has to be found there.
		{"an escaped quote in the pattern", `echo ${v/$'\''/x}`, `\'`, "x"},
		{"an escaped quote in the replacement", `echo ${v/x/$'\''}`, "x", `\'`},
		{"one on each side", `echo ${v/$'\''/$'\''}`, `\'`, `\'`},
		// A separator *inside* the run belongs to the run, which is the
		// same sentence read from the other end.
		{"a separator inside the run", `echo ${v/$'a/b'/x}`, `a/b`, "x"},
		// The control that says what moved was the escape: the same byte
		// spelled without one was cut correctly before any of this.
		{"the same byte without an escape", `echo ${v/$'\x27'/x}`, `\x27`, "x"},
		// And an ordinary quoted run, which was already right.
		{"a plain quoted run", `echo ${v/'a'/x}`, "a", "x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(tc.src, on)
			if err != nil {
				t.Fatalf("%s: %v", tc.src, err)
			}
			e := paramExpIn(t, f, tc.src)
			if got := joinSpans(e.Arg); got != tc.arg {
				t.Errorf("%s: pattern %q, want %q", tc.src, got, tc.arg)
			}
			if got := joinSpans(e.Arg2); got != tc.arg2 {
				t.Errorf("%s: replacement %q, want %q", tc.src, got, tc.arg2)
			}
		})
	}
}

// And whether that construct survives the double quotes around a *word*
// operand, which is the one place the panel splits — and is
// [Dialect.QuoteProtectsTheClosingBrace]'s answer rather than a flag of its
// own. Where a quote quotes, the `$'…'` run is the quoting.
//
// The pattern operand is read on its own terms under either answer, so it is
// not what this separates; the rows above cover it.
func TestAQuotedWordOperandReadsADollarSingleOnlyWhereAQuoteQuotes(t *testing.T) {
	t.Parallel()
	const src = `echo "${u:-$'\t'}"`
	for _, tc := range []struct {
		name  string
		quote BraceQuotePolicy
		want  Quoting
	}{
		{"a quote quotes every operand", BraceQuoteProtectsEveryOperand, DollarSingleQuoted},
		{"only a pattern's", BraceQuoteProtectsAPatternOnly, DoubleQuoted},
		{"none of them", BraceQuoteProtectsNothing, DoubleQuoted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := Core()
			d.DollarSingleQuote = true
			d.QuoteProtectsTheClosingBrace = tc.quote
			f, err := Parse(src, d)
			if err != nil {
				t.Fatalf("%s: %v", src, err)
			}
			e := paramExpIn(t, f, src)
			if e.Arg == nil || len(e.Arg.Spans) == 0 {
				t.Fatalf("%s: no operand", src)
			}
			if got := e.Arg.Spans[0].Quoting; got != tc.want {
				t.Errorf("%s: operand span quoting %v, want %v", src, got, tc.want)
			}
		})
	}
	// A plain double-quoted run is never this question, whatever the axis
	// says: the suite file that found all of this has such a line and it
	// must stay the characters it was written as.
	d := Core()
	d.DollarSingleQuote = true
	d.QuoteProtectsTheClosingBrace = BraceQuoteProtectsEveryOperand
	f, err := Parse(`echo "$'a\tb'"`, d)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd).Args[1].Spans {
		if s.Quoting == DollarSingleQuoted {
			t.Errorf(`"$'a\tb'" read the run as a construct, want the text`)
		}
	}
}

func paramExpIn(t *testing.T, f *File, src string) *ParamExpr {
	t.Helper()
	for _, s := range f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd).Args[1].Spans {
		if s.Kind == ParamExp && s.Param != nil {
			return s.Param
		}
	}
	t.Fatalf("%s: no parsed ParamExp span", src)
	return nil
}

func joinSpans(w *Word) string {
	if w == nil {
		return ""
	}
	var b []byte
	for _, s := range w.Spans {
		b = append(b, s.Value...)
	}
	return string(b)
}
