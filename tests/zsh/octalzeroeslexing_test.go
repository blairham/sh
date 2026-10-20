// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// `octalzeroes` moves two things and this shell moved one of them.
//
// Semantics.ArithLeadingZeroIsOctal says what a numeral's digits are *worth*.
// Where the numeral **ends** is a separate question, read in the parser
// rather than the evaluator, and with only the first half moved the two
// disagreed: `08` was read whole in base ten and handed to a converter that
// wanted octal, which refused the lot — where the reference had already
// stopped at the `8` and gone on to report a stray token.
//
// Every `want` is the reference's own answer, measured 2026-09-28 on
// `/opt/homebrew/bin/zsh` — zsh 5.9.2 (aarch64-apple-darwin25.4.0), `go
// version -m` says *not a Go executable* for it — `-f` over a script file
// under `env -i PATH=/usr/bin:/bin TERM=dumb` with a scratch `HOME` and
// stdin at `/dev/null` (#4436).
func TestOctalZeroesMovesWhereTheNumeralEnds(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		code            int
	}{
		// **The rows that were wrong.** The quoted text is the rest of the
		// expression, so the reader stopped at the bad digit rather than
		// refusing the whole numeral.
		{
			"a digit eight cannot use",
			"setopt octalzeroes\nprint $(( 08 ))",
			"zsh:2: bad math expression: operator expected at `8 '\n", 1,
		},
		{
			"and nine",
			"setopt octalzeroes\nprint $(( 09 ))",
			"zsh:2: bad math expression: operator expected at `9 '\n", 1,
		},
		{
			"after digits the base can use",
			"setopt octalzeroes\nprint $(( 0778 ))",
			"zsh:2: bad math expression: operator expected at `8 '\n", 1,
		},
		{
			"with an operator in front",
			"setopt octalzeroes\nprint $(( 1 + 08 ))",
			"zsh:2: bad math expression: operator expected at `8 '\n", 1,
		},
		// **The discriminator.** The quoted text is `8 + 1` and not `8`, so
		// what the reader handed back is the *rest of the expression* — two
		// tokens where this shell had one. A change that only reworded the
		// refusal would put `8` here.
		{
			"with an operator behind",
			"setopt octalzeroes\nprint $(( 08 + 1 ))",
			"zsh:2: bad math expression: operator expected at `8 + 1 '\n", 1,
		},
		{
			"through a declaration's own reader",
			"setopt octalzeroes\ntypeset -i v; v=08; print $v",
			"zsh:2: bad math expression: operator expected at `8'\n", 1,
		},
		// **The controls, and they hold on both sides of the option.** A
		// numeral octal *can* read still reads, a radix prefix still names
		// its own base, and with the option off a leading zero names nothing
		// and ten can use the digit — which is the row that says this is the
		// option's doing.
		{"octal that reads", "setopt octalzeroes\nprint $(( 010 ))", "8\n", 0},
		{"a longer one", "setopt octalzeroes\nprint $(( 0777 ))", "511\n", 0},
		{"zero itself", "setopt octalzeroes\nprint $(( 0 ))", "0\n", 0},
		{"the last digit octal has", "setopt octalzeroes\nprint $(( 07 ))", "7\n", 0},
		{"a radix prefix still names its base", "setopt octalzeroes\nprint $(( 0x18 ))", "24\n", 0},
		{"the option off", "print $(( 08 ))", "8\n", 0},
		{"the option off, nine", "print $(( 09 ))", "9\n", 0},
		{"the option off, a longer one", "print $(( 0778 ))", "778\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := zshScriptFileAt(t, tc.src+"\n")
			var out, errs bytes.Buffer
			code := driver.MainArgs(zshWriting(&out, &errs), []string{"zsh", "-f", script})
			got := out.String() + errs.String()
			// The location carries the script's own path; the assertions
			// above are written against the name the harness gives it.
			got = strings.ReplaceAll(got, script, "zsh")
			if got != tc.want || code != tc.code {
				t.Errorf("= %q (status %d), want %q at %d", got, code, tc.want, tc.code)
			}
		})
	}
}
