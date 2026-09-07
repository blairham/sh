// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A whole file printed in a chosen arrangement, which is what [syntax.Print]
// is with no arrangement at all.
//
// A file rather than a command because a *line* is what an interactive front
// end has in hand, and a line can hold several statements. Printing them one
// at a time would leave the caller to decide what goes between, which is
// exactly the part an arrangement is for.
func TestAFileIsPrintedInTheArrangementAsked(t *testing.T) {
	lines := syntax.Layout{
		Indent: "\t", Nested: true, Lines: true,
		ThenOnItsOwnLine:         true,
		DoAfterWordsOnItsOwnLine: true,
	}
	for _, c := range []struct {
		name, src, plain, arranged string
	}{
		{
			name: "two statements on one line",
			src:  "true;false\n", plain: "true; false", arranged: "true\nfalse",
		},
		{
			name:     "a loop written on one line",
			src:      "for i in 1 2; do echo $i; done\n",
			plain:    "for i in 1 2; do echo $i; done",
			arranged: "for i in 1 2\ndo\n\techo $i\ndone",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := syntax.Parse(c.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := syntax.Print(f); got != c.plain {
				t.Errorf("Print = %q, want %q", got, c.plain)
			}
			if got := syntax.PrintFileWith(f, syntax.Layout{}); got != c.plain {
				t.Errorf("PrintFileWith with no arrangement = %q, want %q — it is what Print is", got, c.plain)
			}
			if got := syntax.PrintFileWith(f, lines); got != c.arranged {
				t.Errorf("PrintFileWith = %q, want %q", got, c.arranged)
			}
		})
	}
}

// Nothing to print is the empty string rather than a panic, which is what
// every other entry point in this file answers for nil.
func TestPrintingNoFileIsNothing(t *testing.T) {
	if got := syntax.PrintFileWith(nil, syntax.Layout{Lines: true}); got != "" {
		t.Errorf("printing no file gave %q, want nothing", got)
	}
}
