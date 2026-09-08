// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A run of digits is an assignment's name where the dialect says so, and a
// command name everywhere else.
//
// The difference is in the parse rather than in what is done with the word
// afterwards, which is what puts the switch here. Measured 2026-09-07: `set --
// x; 1=*` leaves `$1` holding a literal `*` in zsh 5.9.2, where bash 5.3.15
// globs the whole word and complains about `1=*` — an assignment's value is
// not a pattern and a command name is, so the two shells subject the same
// characters to different expansions.
func TestANumberIsAnAssignmentNameOnlyWithTheFlag(t *testing.T) {
	for _, c := range []struct {
		name   string
		src    string
		assign string
		value  string
		append bool
	}{
		{"the first parameter", "1=abc", "1", "abc", false},
		{"a two-digit one", "10=abc", "10", "abc", false},
		{"leading zeros", "01=z", "01", "z", false},
		{"the shell's own name", "0=abc", "0", "abc", false},
		{"an append", "1+=x", "1", "x", true},
		{"no value", "1=", "1", "", false},
		{"a value that would glob", "1=*", "1", "*", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := syntax.Core()
			d.AppendAssign = true
			d.PositionalAssignment = true
			p := syntax.NewParser(c.src, d)
			f := p.Parse()
			if err := p.Err(); err != nil {
				t.Fatalf("parse %q: %v", c.src, err)
			}
			cmd := onlySimple(t, f)
			if len(cmd.Assigns) != 1 || len(cmd.Args) != 0 {
				t.Fatalf("%d assignments and %d words, want 1 and 0",
					len(cmd.Assigns), len(cmd.Args))
			}
			a := cmd.Assigns[0]
			if a.Name != c.assign {
				t.Errorf("name = %q, want %q", a.Name, c.assign)
			}
			if a.Index != nil {
				t.Errorf("subscript = %q, want none", syntax.PrintWord(a.Index))
			}
			got := ""
			if a.Value != nil {
				got = syntax.PrintWord(a.Value)
			}
			if got != c.value {
				t.Errorf("value = %q, want %q", got, c.value)
			}
			if a.Append != c.append {
				t.Errorf("append = %v, want %v", a.Append, c.append)
			}

			// And the same text without the flag is the command name the
			// other five shells run. Asserted on every row, because a flag
			// that only ever turns things *on* is one nothing would notice
			// had been left on.
			off := syntax.Core()
			off.AppendAssign = true
			p = syntax.NewParser(c.src, off)
			f = p.Parse()
			if err := p.Err(); err != nil {
				t.Fatalf("parse %q without the flag: %v", c.src, err)
			}
			cmd = onlySimple(t, f)
			if len(cmd.Assigns) != 0 || len(cmd.Args) != 1 {
				t.Fatalf("without the flag: %d assignments and %d words, want 0 and 1",
					len(cmd.Assigns), len(cmd.Args))
			}
			if got := syntax.PrintWord(cmd.Args[0]); got != c.src {
				t.Errorf("without the flag: word = %q, want %q", got, c.src)
			}
		})
	}
}

// What the flag does *not* admit, and each of these is a shape that would look
// like a plausible widening of the rule.
//
// A name that merely starts with a digit is not one anywhere — `1a=z` is
// `command not found` in all six panel columns — and a subscript after the
// digits is not this construct either: `1[0]=v` is a command name in zsh too,
// which is why the head still insists on a name in front of the bracket.
//
// `+=x` is the shape the digit test has to be careful about rather than the
// one anybody writes: the append strips the `+` and leaves *nothing*, and a
// run-of-digits test that answered yes for the empty string would make it an
// assignment to a parameter with no name.
func TestTheFlagAdmitsDigitsAndNothingElse(t *testing.T) {
	d := syntax.Core()
	d.AppendAssign = true
	d.ArraySubscript = true
	d.PositionalAssignment = true
	for _, src := range []string{
		"1a=z",   // a digit then a letter — a name, but not one anywhere
		"1[0]=v", // a subscript on the digits
		"+=x",    // the append leaves *no* name, which is not a run of digits
		"+=",     // and neither is it with no value
		"1-=v",   // punctuation the append rule does not cover
		"-1=v",   // a sign is not a digit
		"1.2=v",  // nor is a decimal point
	} {
		p := syntax.NewParser(src, d)
		f := p.Parse()
		if err := p.Err(); err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		cmd := onlySimple(t, f)
		if len(cmd.Assigns) != 0 {
			t.Errorf("%q: read as an assignment to %q, want a command name",
				src, cmd.Assigns[0].Name)
		}
	}
}
