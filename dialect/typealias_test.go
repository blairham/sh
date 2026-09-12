// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// What each dialect's `type`, `command -V` and `command -v` say about a name
// the alias tables hold (#2097).
//
// The tables came first and the builtins that report a name never looked at
// them, so every column answered "not found" about an alias its own `alias`
// builtin listed one line earlier. Measured 2026-09-12 with
// `alias a='echo  hi'` — two spaces, so the quoting is visible:
//
//	dash    a is an alias for echo  hi     / alias a='echo  hi'
//	bash    a is aliased to `echo  hi'     / alias a='echo  hi'
//	ksh93   a is an alias for 'echo  hi'   / 'echo  hi'
//	zsh     a is an alias for echo  hi     / alias a='echo  hi'
//
// ksh93 is the one that quotes the body inside the sentence, and the one
// whose `command -v` writes the body with no `alias name=` in front of it.
func TestEachDialectNamesAnAlias(t *testing.T) {
	for _, c := range []struct {
		dialect       string
		sentence, vee string
	}{
		{"bash", "a is aliased to `echo  hi'\n", "alias a='echo  hi'\n"},
		{"dash", "a is an alias for echo  hi\n", "alias a='echo  hi'\n"},
		{"ksh", "a is an alias for 'echo  hi'\n", "'echo  hi'\n"},
		{"zsh", "a is an alias for echo  hi\n", "alias a='echo  hi'\n"},
	} {
		t.Run(c.dialect, func(t *testing.T) {
			p := presets[c.dialect]
			// bash names an alias only while aliases expand, so the shopt is
			// what puts every column on the same footing — and it is a no-op
			// in the other three, which is measured in the test below.
			const defs = "shopt -s expand_aliases 2>/dev/null; alias a='echo  hi'; "
			for _, tc := range []struct{ src, want string }{
				{defs + "type a", c.sentence},
				{defs + "command -V a", c.sentence},
				{defs + "command -v a", c.vee},
			} {
				out, _, err := p.Combined(t, dialecttest.Base{}, tc.src)
				if err != nil {
					t.Fatal(err)
				}
				if out != tc.want {
					t.Errorf("%s said %q, want %q", tc.src, out, tc.want)
				}
			}
		})
	}
}

// bash alone reports what would *run*, so an alias that cannot expand is not
// there to be named; the other three report what the table holds.
//
// This is the row that makes the axis a measurement rather than a guess about
// how a `-c` string is read: the same call answers in bash one line after
// `shopt -s expand_aliases`, and zsh names one after `unsetopt aliases`.
func TestOnlyOneDialectHidesAnAliasThatCannotExpand(t *testing.T) {
	for _, c := range []struct {
		dialect string
		want    string
	}{
		{"bash", "bash: line 1: type: a: not found\n"},
		{"dash", "a is an alias for echo hi\n"},
		{"ksh", "a is an alias for 'echo hi'\n"},
		{"zsh", "a is an alias for echo hi\n"},
	} {
		t.Run(c.dialect, func(t *testing.T) {
			p := presets[c.dialect]
			out, _, err := p.Combined(t, dialecttest.Base{}, `alias a='echo hi'; type a`)
			if err != nil {
				t.Fatal(err)
			}
			if out != c.want {
				t.Errorf("said %q, want %q", out, c.want)
			}
		})
	}
}
