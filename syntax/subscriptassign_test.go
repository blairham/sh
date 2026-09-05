// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A subscript is a word, so an expansion may stand in one and the assignment
// still has to be recognized as an assignment.
//
// It was not. The scan for the `=` looked only at the first span of the word,
// which ends at the bracket the moment anything expands inside it, so `a[$i]=v`
// — the ordinary way a loop writes an element — became a command name.
func TestASubscriptMayHoldAnExpansion(t *testing.T) {
	for _, c := range []struct {
		name   string
		src    string
		assign string // the name assigned to
		index  string // how the subscript prints back
		value  string
		append bool
	}{
		{"a parameter", "a[$i]=v", "a", "$i", "v", false},
		// The printer writes braces back only where they are needed, so this
		// one comes out `$i`. What the row asks is that the expansion reached
		// the subscript at all.
		{"a braced parameter", "a[${i}]=v", "a", "$i", "v", false},
		{"an arithmetic substitution", "a[$((i))]=v", "a", "$((i))", "v", false},
		{"a command substitution", "a[$(echo 1)]=v", "a", "$(echo 1)", "v", false},
		{"text around the expansion", "a[1+$i]=v", "a", "1+$i", "v", false},
		{"a quoted subscript", `a["1"]=v`, "a", `"1"`, "v", false},
		// The plain numeral still arrives the way it always did.
		{"a numeral", "a[1]=v", "a", "1", "v", false},
		{"a bare name", "a[i]=v", "a", "i", "v", false},
		// Both halves of the head can expand at once, and only the spans say
		// which side of the `=` each one is on.
		{"expansions on both sides", "a[$i]=$v", "a", "$i", "$v", false},
		// The `]` and the `+=` after it sit in the same unlooked-at span.
		{"an append", "a[$i]+=v", "a", "$i", "v", true},
		// A bare `name[i]=` assigns the empty string, as `name=` does.
		{"no value", "a[$i]=", "a", "$i", "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := syntax.NewParser(c.src, syntax.Core())
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
			if a.Index == nil {
				t.Fatalf("no subscript, want %q", c.index)
			}
			if got := syntax.PrintWord(a.Index); got != c.index {
				t.Errorf("subscript = %q, want %q", got, c.index)
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
		})
	}
}

// The subscript form is the ArraySubscript flag's, and a dialect without it
// has no subscript to read: the word is a command name, which is what the
// shell without arrays answers `a[1]=Q: not found` to.
//
// Accepting the shape everywhere made that shell assign silently instead.
func TestASubscriptedAssignmentNeedsTheFlag(t *testing.T) {
	d := syntax.Core()
	d.ArraySubscript = false
	for _, src := range []string{"a[1]=v", "a[$i]=v", "a[i]+=v"} {
		p := syntax.NewParser(src, d)
		f := p.Parse()
		if err := p.Err(); err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		cmd := onlySimple(t, f)
		if len(cmd.Assigns) != 0 || len(cmd.Args) != 1 {
			t.Errorf("%q: %d assignments and %d words, want 0 and 1",
				src, len(cmd.Assigns), len(cmd.Args))
		}
	}
}

// The scan stops where the shells stop it. A word whose name half is broken by
// an expansion is a command name in every shell on the panel — `a$b=c` reports
// `aX=c` — so a scan that followed the `=` across spans wherever it found one
// would turn that into an assignment.
func TestWhatIsNotASubscriptedAssignment(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
	}{
		{"a name broken by an expansion", "a$b=c"},
		{"a quoted equals", `a"="b`},
		// A `=` inside the value is not the head's: the bracket has to come
		// before the `=` for the word to be the subscript form at all.
		{"a bracket in the value", "a=b[0]"},
		// The `]` has to be the one the `=` follows.
		{"text after the bracket", "a[1]x=v"},
		// A leading bracket leaves no name in front of it.
		{"no name", "[1]=v"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := syntax.NewParser(c.src, syntax.Core())
			f := p.Parse()
			if err := p.Err(); err != nil {
				t.Fatalf("parse %q: %v", c.src, err)
			}
			cmd := onlySimple(t, f)
			for _, a := range cmd.Assigns {
				if a.Index != nil {
					t.Fatalf("%q parsed as a subscripted assignment to %q", c.src, a.Name)
				}
			}
			if c.src == "a=b[0]" {
				// The one row here that *is* an assignment, kept so the
				// bracket-before-equals rule is tested from both sides.
				if len(cmd.Assigns) != 1 || syntax.PrintWord(cmd.Assigns[0].Value) != "b[0]" {
					t.Errorf("%q did not stay a scalar assignment", c.src)
				}
				return
			}
			if len(cmd.Assigns) != 0 {
				t.Errorf("%q: %d assignments, want 0", c.src, len(cmd.Assigns))
			}
		})
	}
}

// A subscript that expands keeps its position, so a diagnostic about it points
// at the subscript rather than at the start of the word.
func TestASubscriptKeepsItsPosition(t *testing.T) {
	const src = "a[$i]=v"
	p := syntax.NewParser(src, syntax.Core())
	f := p.Parse()
	if err := p.Err(); err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	idx := onlySimple(t, f).Assigns[0].Index
	if idx == nil || idx.Pos().Col != 3 {
		t.Errorf("subscript starts at %v, want column 3", idx.Pos())
	}
}
