// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// When a command substitution's body is parsed here, which is **both**
// spellings with the line that holds them — the far end of the panel from
// bash 3.2, ksh93 and zsh, which read neither until the substitution runs.
//
// Measured 2026-09-19 against dash 0.5.12, script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on the null
// device. Both of these write nothing at all:
//
//	echo before; v=$(if); echo after
//	echo before; v=`if`; echo after
//
// where bash 5.3.20 writes `before` for the second of them, which is what
// makes the spelling a question rather than a symmetry. The control that
// makes it about *parsing* is the same body in a branch nothing takes: `false
// && v=$(if)` is refused here and prints `after=1` in the three lazy columns.
//
// See syntax.Dialect.SubstitutionBodyRead (#2857).
func TestBothSubstitutionSpellingsAreParsedWithTheirLine(t *testing.T) {
	for _, c := range []struct {
		// absent are the words nothing on the refused line may write;
		// present is what a line in front of it wrote before the refusal.
		name, src, present string
		absent             []string
		status             int
	}{
		{
			name: "the newer spelling", src: "echo before; v=$(if); echo after\n",
			absent: []string{"before", "after"}, status: 2,
		},
		{
			name: "the older spelling", src: "echo before; v=`if`; echo after\n",
			absent: []string{"before", "after"}, status: 2,
		},
		{
			name:   "the newer spelling in a branch nothing takes",
			src:    "false && v=$(if); echo \"after=$?\"\n",
			absent: []string{"after="}, status: 2,
		},
		{
			name:   "the older spelling in a branch nothing takes",
			src:    "false && v=`if`; echo \"after=$?\"\n",
			absent: []string{"after="}, status: 2,
		},
		{
			// The logical line is the unit, so what stands in front of the
			// refused line still runs.
			name:    "the line before it still runs",
			src:     "printf 'start\\n'\necho before; v=$(if); echo after\n",
			present: "start", absent: []string{"before", "after"}, status: 2,
		},
		{
			// A quote inside a double-quoted operand hides nothing here,
			// which is where this column parts from bash 5.3: the body is
			// read with the line and the line writes nothing. See
			// syntax.Dialect.AQuotedOperandHidesASubstitutionFromItsLine.
			name:   "a quote in an operand hides nothing",
			src:    "echo before; echo \"${v-'$(if)'}\"; echo after\n",
			absent: []string{"before", "after"}, status: 2,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, status, err := preset.Combined(t, dialecttest.Base{}, c.src)
			if err != nil {
				t.Fatalf("run %q: %v", c.src, err)
			}
			if status != c.status {
				t.Errorf("ran %q: status %d, want %d (said %q)",
					c.src, status, c.status, out)
			}
			for _, word := range c.absent {
				if strings.Contains(out, word) {
					t.Errorf("ran %q: said %q, which holds %q: the line ran",
						c.src, out, word)
				}
			}
			if c.present != "" && !strings.Contains(out, c.present) {
				t.Errorf("ran %q: said %q, want %q from the line in front",
					c.src, out, c.present)
			}
			if out == "" {
				t.Errorf("ran %q: said nothing at all, want a complaint", c.src)
			}
		})
	}
}
