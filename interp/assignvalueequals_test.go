// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// assignEqualsSem answers the two axes this road stands on by name: whether
// `=cmd` is an expansion at all, and whether the first unquoted `=` in any
// word opens the context an assignment's value has. Which preset answers each
// is the dialect packages' claim and not this one's.
func assignEqualsSem(equals, magic Answer) Semantics {
	s := permissive()
	s.EqualsExpansion = equals
	s.TheFirstUnquotedEqualsInAWordOpensATildeContext = magic
	return s
}

// `=cmd` at an assignment value's head and after each of its colons, which is
// where a tilde already ran and where this did not (#4566). See
// interp/assignvalueequals.go for the measurement.
func TestAnAssignmentValueTakesTheEqualsExpansion(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"the value's head", `v==ls; echo "$v"`, "/ls"},
		{"a declaration's value", `typeset t==ls; echo "$t"`, "/ls"},
		{"an export's value", `export x==ls; echo "$x"`, "/ls"},
		{"after a colon", `v=x:=ls; echo "$v"`, "/ls"},
		{"an append", `v+==ls; echo "$v"`, "/ls"},
		{"an array element by subscript", `a=(p); a[0]==ls; echo "${a[@]}"`, "/ls"},
		{"an array literal's element", `a=(=ls); echo "${a[@]}"`, "/ls"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := run(t, c.src, withSem(assignEqualsSem(Yes, No)))
			if st != 0 {
				t.Fatalf("status %d, out %q", st, out)
			}
			if !strings.HasSuffix(strings.TrimSpace(out), c.want) {
				t.Errorf("%s = %q, want a path ending %q", c.src, out, c.want)
			}
		})
	}
}

// The rows that do **not** move, and the axis answered off. Each keeps the
// `=ls` in the output, so a row that produced nothing at all is visible as
// such rather than compared against an expectation of nothing.
func TestAnAssignmentValueLeavesTheOtherEqualsAlone(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		equals          Answer
	}{
		// Not at a head and not after a colon: an `=` in the middle of a
		// value is two characters.
		{"an equals in the middle", `v=x=ls; echo "$v"`, "x=ls\n", Yes},
		// A lone `=` names nothing.
		{"a bare equals", `v==; echo "$v"`, "=\n", Yes},
		// Quoting removes it, as it does the tilde.
		{"a quoted value", `v="=ls"; echo "$v"`, "=ls\n", Yes},
		{"a quoted colon segment", `v=x:"=ls"; echo "$v"`, "x:=ls\n", Yes},
		// **The control that says the option decides and not the road.**
		// With the axis off the same value keeps its characters, in the
		// head position and after a colon alike.
		{"the axis off, at the head", `v==ls; echo "$v"`, "=ls\n", No},
		{"the axis off, after a colon", `v=x:=ls; echo "$v"`, "x:=ls\n", No},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := run(t, c.src, withSem(assignEqualsSem(c.equals, No)))
			if st != 0 {
				t.Fatalf("status %d, out %q", st, out)
			}
			if out != c.want {
				t.Errorf("%s = %q, want %q", c.src, out, c.want)
			}
		})
	}
}

// The word road, which takes the same two positions from the first unquoted
// `=` rather than from the front of the word. Both axes have to be on: the
// narrower assignment-shape rule does not carry this, because the columns
// that have it have no `=cmd` expansion at all.
func TestTheFirstUnquotedEqualsOpensTheEqualsExpansionToo(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		equals, magic   Answer
		path            bool
	}{
		{"straight after the equals", `echo a==ls`, "", Yes, Yes, true},
		{"after a colon in the value", `echo a=x:=ls`, "", Yes, Yes, true},
		{"a word that is not a name", `echo --opt==ls`, "", Yes, Yes, true},
		// Not the *last* `=`: the split is at offset 1, so `=ls` opens
		// neither the value nor one of its colon segments.
		{"a second equals in the value", `echo a=b==ls`, "a=b==ls\n", Yes, Yes, false},
		// **The two controls.** With the wider axis off the word keeps its
		// characters although `=cmd` is on, and with `=cmd` off it keeps
		// them although the wider axis is on — so neither answer alone
		// produces the row.
		{"the wider axis off", `echo a==ls`, "a==ls\n", Yes, No, false},
		{"the expansion off", `echo a==ls`, "a==ls\n", No, Yes, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := run(t, c.src, withSem(assignEqualsSem(c.equals, c.magic)))
			if st != 0 {
				t.Fatalf("status %d, out %q", st, out)
			}
			if c.path {
				if !strings.HasSuffix(strings.TrimSpace(out), "/ls") {
					t.Errorf("%s = %q, want a path ending /ls", c.src, out)
				}
				return
			}
			if out != c.want {
				t.Errorf("%s = %q, want %q", c.src, out, c.want)
			}
		})
	}
}

// A value naming a command that is not there is reported and abandons the
// script, exactly as the word road's failure does — one wording through
// Runner.equalsPath rather than two.
func TestAnAssignmentValuesEqualsExpansionFailureIsFatal(t *testing.T) {
	sem := assignEqualsSem(Yes, No)
	sem.FatalErrorStatusIsOne = Yes
	out, st := run(t, `v==nosuchcommand_xyz; echo after`, withSem(sem))
	if strings.Contains(out, "after") {
		t.Errorf("the script continued: %q", out)
	}
	if !strings.Contains(out, "nosuchcommand_xyz not found") {
		t.Errorf("got %q", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}
