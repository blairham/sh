// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package printer_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/fmt/comments"
	"github.com/blairham/sh/internal/fmt/printer"
	"github.com/blairham/sh/syntax"
)

// A bare negation — a `!` with no pipeline after it, which
// syntax.Dialect.BareNegationReach says where a dialect takes — is the one
// expression made of nothing but keywords, and this pass wrote neither of its
// two shapes back.
//
// A single `!` came out as `! ` and left the blank that separates a negation
// from its pipeline standing at the end of a line, which is trailing
// whitespace out of a formatter. An *even* run came out as **nothing at all**:
// syntax.Dialect.RepeatedNegationToggles collapses the pair into one flag, so
// `! !` is a pipeline with no negation and no commands, and a writer keyed on
// the flag alone dropped the statement out of the script. The two exit
// differently — 0 for the pair and 1 for the one — so it is not a spelling
// (#3721).
func TestABareNegationIsWrittenBack(t *testing.T) {
	d, st := ksh.Dialect(), ksh.Style()
	for _, src := range []string{
		"!\n",
		"! !\n",
		"echo a\n!\n",
		"! true\n",
		"! true | cat\n",
	} {
		f, err := syntax.Parse(src, d)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		got := printer.Format(src, f, comments.Recover(src, f), st)
		if got != src {
			t.Errorf("format %q = %q, want it unmoved", src, got)
		}
		if _, err := syntax.Parse(got, d); err != nil {
			t.Errorf("%q: the output does not parse: %v", src, err)
		}
	}
	// The shapes whose spelling is not kept, and it is the collapse rather
	// than this pass: a run of more than one is one flag by the time the
	// tree has it, so there is nothing to write the extra `!`s from. Each
	// still exits what it was written to exit, which is the promise this
	// formatter makes — `! ! !` and `!` are both 1, `! ! true` and `true`
	// both whatever `true` is.
	for _, tc := range []struct{ src, want string }{
		{"! ! !\n", "!\n"},
		{"! ! true\n", "true\n"},
		{"! ! ! true\n", "! true\n"},
	} {
		f, err := syntax.Parse(tc.src, d)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.src, err)
		}
		if got := printer.Format(tc.src, f, comments.Recover(tc.src, f), st); got != tc.want {
			t.Errorf("format %q = %q, want %q", tc.src, got, tc.want)
		}
	}
}
