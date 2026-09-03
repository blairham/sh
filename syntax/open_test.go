// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// What the parser was still inside when the input ran out.
//
// The stack was already kept, for a diagnostic that names the construct that
// ran out and the line it began on. It was unreadable afterwards: opens closes
// by deferring, so by the time Parse returns the stack has been unwound and a
// caller asking is told nothing was open. A prompt asks afterwards.
func TestWhatIsStillOpen(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want string
	}{
		{"echo hi\n", ""},
		{"for i in 1\n", "for"},
		// A loop's `do` opens nothing further: the loop is what is waiting,
		// and `done` is what it waits for.
		{"for i in 1\ndo\n", "for"},
		{"for i in 1\ndo\n:\n", "for"},
		{"if true\n", "if"},
		{"if true\nthen\n", "if then*"},
		{"if true\nthen\n:\n", "if then*"},
		{"if true\nthen\n:\nelse\n", "if else*"},
		{"while false\ndo\n", "while do*"},
		{"case x in\n", "case"},
		{"{\n", "{"},
		{"f() {\n", "{"},
		// Innermost last, so a caller drawing them reads the list in the
		// order they were opened.
		{"for i in 1\ndo\nif true\nthen\n", "for if then*"},
		// A bar is open only while the command after it is looked for.
		{"echo x |\n", "|*"},
		{"echo x | cat\n", ""},
		// And stops being open once it has one: the input below runs out
		// inside the `if`, and a bar left standing would be reported as
		// still waiting when its command arrived two words ago.
		{"if true\nthen\necho a | cat\n", "if then*"},
		// And a bar written inside a construct does not displace it: as a
		// clause it evicted the `then`, and the input below then reported an
		// `if` waiting for a bar.
		{"if true\nthen\necho a |\n", "if then* |*"},
		// Wrong input is not unfinished input, and has nothing open.
		{"for do done\n", ""},
	} {
		t.Run(strings.ReplaceAll(tc.src, "\n", "\\n"), func(t *testing.T) {
			p := syntax.NewParser(tc.src, syntax.Core())
			p.Parse()
			var got []string
			for _, o := range p.Open() {
				w := o.Word
				if !o.Construct {
					w += "*"
				}
				got = append(got, w)
			}
			if strings.Join(got, " ") != tc.want {
				t.Errorf("open = %q, want %q", strings.Join(got, " "), tc.want)
			}
		})
	}
}

// The line each was opened on travels with it, which is what a diagnostic
// naming a construct from three lines up needs.
func TestWhereAConstructWasOpened(t *testing.T) {
	p := syntax.NewParser("for i in 1\ndo\nif true\nthen\n", syntax.Core())
	p.Parse()
	open := p.Open()
	if len(open) != 3 {
		t.Fatalf("open = %v, want three things", open)
	}
	for i, want := range []int{1, 3, 4} {
		if open[i].Line != want {
			t.Errorf("%s opened on line %d, want %d", open[i].Word, open[i].Line, want)
		}
	}
}

// Complete input has nothing open, and asking twice gives the same answer.
func TestNothingOpenAfterAWholeCommand(t *testing.T) {
	p := syntax.NewParser("if true\nthen\n:\nfi\n", syntax.Core())
	p.Parse()
	if got := p.Open(); len(got) != 0 {
		t.Errorf("open = %v, want nothing", got)
	}
	if got := p.Open(); len(got) != 0 {
		t.Errorf("asked twice gave %v, want nothing", got)
	}
}

// The snapshot is of the first time the input ran out, not the last.
//
// A parser reports one failure, and what was open at the moment it happened
// is the state that describes it. Anything later is a parser carrying on
// past the end.
func TestTheSnapshotIsOfWhenTheInputRanOut(t *testing.T) {
	p := syntax.NewParser("if true\nthen\n", syntax.Core())
	p.Parse()
	first := p.Open()
	if len(first) != 2 {
		t.Fatalf("open = %v, want the if and its then", first)
	}
	if first[0].Word != "if" || first[1].Word != "then" {
		t.Errorf("open = %v, want if then", first)
	}
}
