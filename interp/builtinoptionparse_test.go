// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"testing"
)

// builtinOptions is tested directly because one of its rules cannot yet be
// seen from outside: a lone `-` is an *operand*, and no builtin here validates
// its operands, so eating the dash or leaving it produces the same silence.
//
// It is not a rule to drop for that. Measured: `export -` is `not a valid
// identifier` in bash, `bad variable name` in dash and `is not an identifier`
// in ksh93 — all three took the dash as a name — and zsh listed the
// environment. Every one of them treated it as something other than an option,
// and the moment operand validation arrives, eating it would be wrong.
// A lone `-` is kept as an operand where the dialect says so, which is three
// of the four. That it is an *answer* rather than the rule is #204: zsh eats
// it, and this test named the dash as a settled fact until then.
func TestBuiltinOptionsKeepsALoneDash(t *testing.T) {
	sem := PosixSemantics()
	sem.LoneDashIsAnOption = No
	r := newTestRunner(t, &Runner{Semantics: &sem})
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

// TestBuiltinOptionsTakesArguments: a `:` after a letter in known marks one
// that takes an argument — the getopts convention, and the same rule the
// shells apply to their builtins. The argument is the rest of the word when
// anything follows the letter and the next word when the bundle ends there,
// which is what makes `-d :` and `-d:` the same option.
func TestBuiltinOptionsTakesArguments(t *testing.T) {
	sem := PosixSemantics()
	sem.LoneDashIsAnOption = No
	r := newTestRunner(t, &Runner{Semantics: &sem})
	for _, c := range []struct {
		args []string
		rest []string
		opts string
		d    string
	}{
		// Apart: the argument is the next word.
		{[]string{"-d", ":", "x"}, []string{"x"}, "d", ":"},
		// Attached: the argument is the rest of the word.
		{[]string{"-d:", "x"}, []string{"x"}, "d", ":"},
		// Clustered behind a plain letter, both spellings.
		{[]string{"-vd", ":", "x"}, []string{"x"}, "vd", ":"},
		{[]string{"-vd:", "x"}, []string{"x"}, "vd", ":"},
		// The argument may look like an option; it is not read as one.
		{[]string{"-d", "-v", "x"}, []string{"x"}, "d", "-v"},
		// `--` still ends the options, and an argument-less run still works.
		{[]string{"-v", "--", "-d"}, []string{"-d"}, "v", ""},
	} {
		rest, opts, optArg, code := r.builtinOptionsArg("test", c.args, "vfd:")
		if code != 0 {
			t.Errorf("%q: status %d, want it accepted", c.args, code)
			continue
		}
		if opts != c.opts {
			t.Errorf("%q: options %q, want %q", c.args, opts, c.opts)
		}
		if got := optArg['d']; got != c.d {
			t.Errorf("%q: -d argument %q, want %q", c.args, got, c.d)
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

// TestBuiltinOptionsRefusesAMissingArgument: an argument-taking letter whose
// bundle ends the argument list has nothing to take, and every shell refuses
// rather than inventing an empty one.
func TestBuiltinOptionsRefusesAMissingArgument(t *testing.T) {
	sem := PosixSemantics()
	sem.LoneDashIsAnOption = No
	var buf strings.Builder
	r := newTestRunner(t, &Runner{Semantics: &sem, Stderr: &buf})
	for _, args := range [][]string{
		{"-d"},
		{"-vd"},
	} {
		_, _, _, code := r.builtinOptionsArg("test", args, "vfd:")
		if code != 2 {
			t.Errorf("%q: status %d, want 2", args, code)
		}
	}
	if !strings.Contains(buf.String(), "-d") {
		t.Errorf("said %q, want the letter named", buf.String())
	}
}
