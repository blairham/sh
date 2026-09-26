// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// wordValue is the one literal a single-argument command was given, joined
// from its spans so that a word made of several runs is read whole.
func wordValue(t *testing.T, src string, d Dialect) string {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	args := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd).Args
	if len(args) != 2 {
		t.Fatalf("%s: %d words, want 2", src, len(args))
	}
	var out string
	for _, s := range args[1].Spans {
		out += s.Value
	}
	return out
}

// [Dialect.DoubledQuoteInSingleQuotesIsALiteralQuote] makes a doubled `'`
// inside a single-quoted run stand for one literal quote, with the run
// carrying on. The reading is one byte of lookahead and nothing else, so what
// decides is the **count** of quotes: `””` is open, a pair, close.
//
// The rows are about the flag and name no shell. What the reference shell
// prints for each is in dialect/zsh/rcquotes_test.go.
func TestADoubledQuoteInsideSingleQuotesIsOneQuote(t *testing.T) {
	t.Parallel()
	on := Core()
	on.DoubledQuoteInSingleQuotesIsALiteralQuote = true
	off := Core()
	for _, tc := range []struct {
		name     string
		src      string
		on, offs string
	}{
		// The issue's own reduction, and the shape that makes the rule
		// visible without the word being nothing but quotes.
		{"four quotes", `echo ''''`, `'`, ``},
		{"a quote between two letters", `echo 'a''b'`, `a'b`, `ab`},
		{"two quotes", `echo ''`, ``, ``},
		{"six quotes", `echo ''''''`, `''`, ``},
		{"two pairs inside a run", `echo 'x''''y'`, `x''y`, `xy`},
		// The run may also be opened part-way through a word, which is
		// what says the rule is the run's and not the word's.
		{"a run that begins mid-word", `echo a''''b`, `a'b`, `ab`},
		// And the flag reaches only the plain run. A `''` inside `$'…'`,
		// inside double quotes and inside a here-document body is two
		// ordinary characters either way.
		{"inside double quotes", `echo "a''b"`, `a''b`, `a''b`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := wordValue(t, tc.src, on); got != tc.on {
				t.Errorf("%s with the flag on = %q, want %q", tc.src, got, tc.on)
			}
			if got := wordValue(t, tc.src, off); got != tc.offs {
				t.Errorf("%s with the flag off = %q, want %q", tc.src, got, tc.offs)
			}
		})
	}
}

// The flag does not make a quote escapable: an **odd** number of them still
// runs off the end of the input, and it is the same refusal in both states.
//
// This is the control that separates "a doubled quote is one quote" from "a
// quote inside a run is taken as text". A reading of the second kind would
// swallow the last quote of `”'` and leave a word rather than a failure.
func TestAnOddNumberOfQuotesIsUnterminatedEitherWay(t *testing.T) {
	t.Parallel()
	on := Core()
	on.DoubledQuoteInSingleQuotesIsALiteralQuote = true
	off := Core()
	for _, src := range []string{`echo '''`, `echo '''''`, `echo x'''y`, `echo 'a''`} {
		t.Run(src, func(t *testing.T) {
			if _, err := Parse(src, on); err == nil {
				t.Errorf("%s parsed with the flag on, want the run refused", src)
			}
			if _, err := Parse(src, off); err == nil {
				t.Errorf("%s parsed with the flag off, want the run refused", src)
			}
		})
	}
}

// A `$'…'` ends at its first unescaped quote whatever this flag says, so the
// pair that follows one is a *plain* run and the flag reaches that and not
// the escapes inside the dollar-quote.
//
// The two rows are the null and the positive that falsifies it. `$'a”b\tc'`
// is `$'a'` beside `'b\tc'` in both states — a backslash and a `t`, since a
// plain run has no escapes — which is what a reading that reached inside the
// dollar-quote would have turned into a tab. `$'p””q'` is the same text
// with one more pair and it *does* move, so the flag really was on.
func TestTheFlagDoesNotReachInsideADollarSingleQuote(t *testing.T) {
	t.Parallel()
	on := Core()
	on.DollarSingleQuote = true
	on.DoubledQuoteInSingleQuotesIsALiteralQuote = true
	off := Core()
	off.DollarSingleQuote = true
	for _, tc := range []struct{ name, src, on, offs string }{
		{"the null", `echo $'a''b\tc'`, `ab\tc`, `ab\tc`},
		{"the positive beside it", `echo $'p''''q'`, `p'q`, `pq`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := wordValue(t, tc.src, on); got != tc.on {
				t.Errorf("%s with the flag on = %q, want %q", tc.src, got, tc.on)
			}
			if got := wordValue(t, tc.src, off); got != tc.offs {
				t.Errorf("%s with the flag off = %q, want %q", tc.src, got, tc.offs)
			}
		})
	}
}

// A here-document body is not a word and the flag does not reach it, with a
// quoted delimiter or without one.
func TestTheFlagDoesNotReachAHereDocumentBody(t *testing.T) {
	t.Parallel()
	on := Core()
	on.DoubledQuoteInSingleQuotesIsALiteralQuote = true
	for _, src := range []string{"cat <<'HD'\na''b\nHD\n", "cat <<HD\na''b\nHD\n"} {
		f, err := Parse(src, on)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		var body string
		for _, s := range f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd).Redirs[0].Heredoc.Spans {
			body += s.Value
		}
		if want := "a''b\n"; body != want {
			t.Errorf("%q: body %q, want %q", src, body, want)
		}
	}
}
