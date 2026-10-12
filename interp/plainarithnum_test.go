// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"strings"
	"testing"
)

// TestAPlainDecimalNumeralReadsAsTheArithmeticReaderReadsIt reads stored and
// written numerals through readArithNum with the plain-decimal reading on and
// off, under the answers that change how a numeral is read — octal, digit
// separators, floats — and requires the same number, the same error and the
// same diagnostics (#6353).
func TestAPlainDecimalNumeralReadsAsTheArithmeticReaderReadsIt(t *testing.T) {
	texts := []string{
		"0", "5", "-5", "42", "-0", "100000", "123456789012345678", "-123456789012345678",
		"1234567890123456789", "9223372036854775807", "99999999999999999999", "007", "-010",
		"0x10", "1_000", "1e3", "1.5", "08", " 5", "5 ", "+5", "--5", "2#101", "", "x",
	}
	type config struct {
		name  string
		setup func(*Runner)
	}
	configs := []config{
		{"octal", func(r *Runner) { r.Semantics.ArithLeadingZeroIsOctal = Yes }},
		{"decimal", func(r *Runner) { r.Semantics.ArithLeadingZeroIsOctal = No }},
	}
	for _, c := range configs {
		for _, written := range []bool{false, true} {
			read := func(text string, shortcut bool) string {
				sem := PosixSemantics()
				sem.ArithInvalidOctalDigitIsError = Yes
				var errs strings.Builder
				r := newTestRunner(t, &Runner{Semantics: &sem, Stderr: &errs})
				c.setup(r)
				plainDecimalShortcut = shortcut
				defer func() { plainDecimalShortcut = true }()
				n, err := r.readArithNum(text, "", written)
				return fmt.Sprintf("%#v %v %q", n, err, errs.String())
			}
			for _, text := range texts {
				want, got := read(text, false), read(text, true)
				if got != want {
					t.Errorf("%s, written %v, %q: shortcut %s, reader %s", c.name, written, text, got, want)
				}
			}
		}
	}
}
