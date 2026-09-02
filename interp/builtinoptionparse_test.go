// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// builtinOptions is tested directly because one of its rules cannot yet be
// seen from outside: a lone `-` is an *operand*, and no builtin here validates
// its operands, so eating the dash or leaving it produces the same silence.
//
// It is not a rule to drop for that. Measured: `export -` is `not a valid
// identifier` in bash, `bad variable name` in dash and `is not an identifier`
// in ksh93 — all three took the dash as a name — and zsh listed the
// environment. Every one of them treated it as something other than an option,
// and the moment operand validation arrives, eating it would be wrong.
func TestBuiltinOptionsKeepsALoneDash(t *testing.T) {
	r := &Runner{}
	for _, c := range []struct {
		args []string
		rest []string
		opts string
	}{
		{[]string{"-"}, []string{"-"}, ""},
		{[]string{"-", "x"}, []string{"-", "x"}, ""},
		{[]string{"-v", "-"}, []string{"-"}, "v"},
		// `--` ends them, and what follows is an operand however it looks.
		{[]string{"--", "-v"}, []string{"-v"}, ""},
		{[]string{"--"}, nil, ""},
		// Ordinary cases, so the above is not the only thing holding.
		{[]string{"-v", "x"}, []string{"x"}, "v"},
		{[]string{"-vf", "x"}, []string{"x"}, "vf"},
		{[]string{"-v", "-f", "x"}, []string{"x"}, "vf"},
		{[]string{"x", "-v"}, []string{"x", "-v"}, ""},
		{nil, nil, ""},
	} {
		rest, opts, code := r.builtinOptions("test", c.args, "vf")
		if code != 0 {
			t.Errorf("%q: status %d, want it accepted", c.args, code)
			continue
		}
		if opts != c.opts {
			t.Errorf("%q: options %q, want %q", c.args, opts, c.opts)
		}
		if len(rest) != len(c.rest) {
			t.Errorf("%q: rest %q, want %q", c.args, rest, c.rest)
			continue
		}
		for i := range rest {
			if rest[i] != c.rest[i] {
				t.Errorf("%q: rest %q, want %q", c.args, rest, c.rest)
				break
			}
		}
	}
}
