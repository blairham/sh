// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"testing"
)

// TestAPlainDecimalReadsAsTheExpressionReaderReadsIt puts the texts an
// integer name is assigned through integerNumber with the plain-decimal
// reading on and off (#5873), under both answers to the octal question, and
// requires the same number and the same verdict.
func TestAPlainDecimalReadsAsTheExpressionReaderReadsIt(t *testing.T) {
	texts := []string{
		"0", "5", "-5", "42", "-0", "123456789012345678", "-123456789012345678",
		"1234567890123456789", "9223372036854775807", "007", "-010", "0x10",
		"1+2", "-", "--5", "5-", "+5", "1e3", "08", "x", "١",
	}
	for _, octal := range []Answer{Yes, No} {
		read := func(text string, shortcut bool) (int, bool, string) {
			sem := PosixSemantics()
			sem.ArithLeadingZeroIsOctal = octal
			sem.IntegerAssignmentReadsALeadingZeroAsDecimal = No
			var errs strings.Builder
			r := newTestRunner(t, &Runner{Semantics: &sem, Stderr: &errs})
			plainDecimalShortcut = shortcut
			defer func() { plainDecimalShortcut = true }()
			n, ok := r.integerNumber(text)
			return n, ok, errs.String()
		}
		for _, text := range texts {
			wantN, wantOK, wantErr := read(text, false)
			gotN, gotOK, gotErr := read(text, true)
			if gotN != wantN || gotOK != wantOK || gotErr != wantErr {
				t.Errorf("octal %v, %q: shortcut %d %v %q, reader %d %v %q",
					octal, text, gotN, gotOK, gotErr, wantN, wantOK, wantErr)
			}
		}
	}
	plainDecimalShortcut = true
	for _, text := range []string{"5", "-42", "123456789012345678"} {
		if _, ok := plainDecimal(text); !ok {
			t.Errorf("%q is not read as a plain decimal, so the rows above test nothing", text)
		}
	}
}
