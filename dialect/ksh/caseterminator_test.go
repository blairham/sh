// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// The word straight after a `case`'s `in` is the first arm's pattern here, so
// this shell matches a subject spelled `esac`. Measured 2026-09-12 on ksh93u+
// under `env -i PATH=/usr/bin:/bin` with a scratch HOME, over `-c` and a
// script file alike; the other five refuse the `)`.
//
//	$ ksh s.sh          # case esac in esac) echo hit;; esac
//	hit
func TestTheCaseTerminatorIsAPatternAfterTheHeaderHere(t *testing.T) {
	if !ksh.Dialect().CaseTerminatorIsAPatternAfterTheHeader {
		t.Error("ksh93 reads the word after `in` as a pattern")
	}
	out, st, err := preset.Combined(t, dialecttest.Base{}, "case esac in esac) echo hit;; esac")
	if err != nil {
		t.Fatalf("case esac in esac): %v", err)
	}
	if out != "hit\n" || st != 0 {
		t.Errorf("out = %q (status %d), want %q at 0", out, st, "hit\n")
	}
}

// Which is why a `case` with no arms of its own is refused on one line and
// runs with a newline in front of the `esac`. The pair is the whole rule, and
// the second row is what says it is not "a case must have an arm".
//
//	$ ksh -n s.sh       # case x in esac
//	s.sh: syntax error at line 2: `newline' unexpected
//	$ ksh -n s.sh       # case x in ⏎ esac
//	(nothing)
func TestAnArmlessCaseNeedsANewlineHere(t *testing.T) {
	d := ksh.Dialect()
	for _, tc := range []struct{ src, want string }{
		{"case x in esac\n", "syntax error at line 2: `newline' unexpected"},
		{"case x in esac; echo done\n", "syntax error at line 1: `;' unexpected"},
		// A line continuation is not a newline, so it does not restore the
		// terminator.
		{"case x in \\\nesac\n", "syntax error at line 3: `newline' unexpected"},
	} {
		_, err := syntax.Parse(tc.src, d)
		if err == nil {
			t.Errorf("%q parsed; this shell refuses it", tc.src)
			continue
		}
		if got := ksh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
	for _, src := range []string{
		"case x in\nesac\n",
		"case x in\nesac; echo done\n",
		// A comment ends the line, so the newline after it counts.
		"case x in # c\nesac\n",
		// And an arm after a `;;` closes with an ordinary terminator, the
		// reading being the first arm's alone.
		"case x in y) ;; esac\n",
		// A parenthesized pattern needs no newline: the paren takes the
		// reservation away in every shell.
		"case esac in (esac) echo hit;; esac\n",
	} {
		if _, err := syntax.Parse(src, d); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}
