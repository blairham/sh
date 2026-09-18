// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// SeparatorOnlyWhereAnAndOrWantsOne is the narrowest of the four readings of a
// `;` written where a command belongs: an and-or's missing right-hand side and
// nothing else.
//
// It is what [syntax.Dialect.SubstitutionBodyRefusesASteppedOverSeparator]
// turns the one-separator reading into for the body of a `$( … )`, and the
// interpreter is what applies it, so this is the parser's half on its own —
// the four positions, each asked of the value directly.
func TestTheSeparatorReadingThatOnlyAnAndOrTakes(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		wantOK    bool
	}{
		// The one position it takes.
		{"an and-or's missing right-hand side", "false || ; echo b", true},
		// And the four it does not, which the wider readings take.
		{"a list beginning with one", "; echo a", false},
		{"one between two statements", "echo a; ; echo b", false},
		{"one after a `&`", "echo a & ; echo b", false},
		{"one after a bar", "echo a | ; cat", false},
		// The condition row's wider half needs an empty condition list to be
		// allowed as well, which is a flag of its own, so only the narrow
		// reading is asked of it here.
		{"one where a condition begins", "if ; then echo a; fi", false},
		// The control: a separator that *terminates* a statement is not
		// this question at all and every reading takes it.
		{"a separator that terminates", "echo a; ", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := syntax.Core()
			d.AbsentAndOrOperandIsAnEmptyCommand = true
			d.SeparatorWhereACommandBelongs = syntax.SeparatorOnlyWhereAnAndOrWantsOne
			_, err := syntax.Parse(tc.src, d)
			if ok := err == nil; ok != tc.wantOK {
				t.Errorf("parsed = %v (%v), want %v", ok, err, tc.wantOK)
			}
			// And the same text under the widest reading, which takes all
			// seven — the row that says these refusals are the value's and
			// not the grammar's.
			if tc.name == "one where a condition begins" {
				return
			}
			wide := syntax.Core()
			wide.SeparatorWhereACommandBelongs = syntax.AnySeparatorWhereACommandBelongs
			if _, err := syntax.Parse(tc.src, wide); err != nil {
				t.Errorf("the widest reading refused it: %v", err)
			}
		})
	}
}
