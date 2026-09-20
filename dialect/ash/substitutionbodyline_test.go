// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// When a command substitution's body is parsed here: **both** spellings with
// the line that holds them, which is dash's answer and is measured again
// rather than inherited from it.
//
// Measured 2026-09-19 against BusyBox 1.37.0, in the digest-pinned alpine
// container docs/spec/ash.md describes, a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on the null
// device. Both of these write nothing at all and leave 2:
//
//	echo before; v=$(if); echo after
//	echo before; v=`if`; echo after
//
// where bash 5.3.20 writes `before` for the second. The control that makes it
// about *parsing* is `false && v=$(if); echo "after=$?"`, which is refused
// here and writes `after=1` in bash 3.2, ksh93 and zsh.
//
// See syntax.Dialect.SubstitutionBodyRead (#2857).
func TestBothSubstitutionSpellingsAreParsedWithTheirLine(t *testing.T) {
	for _, c := range []struct {
		name, src, present string
		absent             []string
	}{
		{
			name: "the newer spelling", src: "echo before; v=$(if); echo after\n",
			absent: []string{"before", "after"},
		},
		{
			name: "the older spelling", src: "echo before; v=`if`; echo after\n",
			absent: []string{"before", "after"},
		},
		{
			name:   "the newer spelling in a branch nothing takes",
			src:    "false && v=$(if); echo \"after=$?\"\n",
			absent: []string{"after="},
		},
		{
			name:   "the older spelling in a branch nothing takes",
			src:    "false && v=`if`; echo \"after=$?\"\n",
			absent: []string{"after="},
		},
		{
			// The logical line is the unit, so a line in front of the
			// refused one still runs.
			name:    "the line before it still runs",
			src:     "printf 'start\\n'\necho before; v=$(if); echo after\n",
			present: "start", absent: []string{"before", "after"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, status, err := preset.Combined(t, dialecttest.Base{
				Name: "ash", Env: []string{"PATH=/usr/bin:/bin"},
			}, c.src)
			if err != nil {
				t.Fatalf("run %q: %v", c.src, err)
			}
			if status != 2 {
				t.Errorf("ran %q: status %d, want 2 (said %q)", c.src, status, out)
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
