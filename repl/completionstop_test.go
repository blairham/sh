// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "testing"

// CompletionStopped from the shell's completion is a completion that stopped
// on an error: nothing else is asked and the line stays (#6068). Nil and an
// empty list are "nothing to say", and this editor's own completion answers
// as before.
func TestAStoppedShellCompletionAsksNothingElse(t *testing.T) {
	for _, c := range []struct {
		name   string
		answer []Candidate
		want   string
	}{
		{"stopped", CompletionStopped(), "ech"},
		{"nothing to say", nil, "echo "},
		{"an empty list", []Candidate{}, "echo "},
	} {
		asked := false
		e := &editor{
			comp: CompleterFunc(func(Completion) []Candidate {
				asked = true
				return []Candidate{{Word: "echo"}}
			}),
			shellComplete: func(string, Completion) []Candidate { return c.answer },
			line:          []rune("ech"), pos: 3,
			drawn: drawnLine{valid: true}, row: 2,
		}
		e.complete(e.completerFor("cw"))
		if got := string(e.line); got != c.want {
			t.Errorf("%s: line %q, want %q", c.name, got, c.want)
		}
		stopped := c.name == "stopped"
		if asked == stopped {
			t.Errorf("%s: the editor's own completion asked = %v", c.name, asked)
		}
		if stopped && (e.drawn.valid || e.row != 0) {
			t.Errorf("%s: the draw still describes the screen above the diagnostic", c.name)
		}
	}
}
