// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// A widget is told what the keystroke before it ran: a typed character, a
// default key, a binding, itself, and — across a line — the key that ended
// the line before. See LastWidget.
func TestAWidgetIsToldTheWidgetBeforeIt(t *testing.T) {
	var out strings.Builder
	s := Shell{
		KeyBindings: func(Keymap) map[string]Binding {
			return map[string]Binding{"\a": {Function: "w"}, "\x0f": {Widget: WidgetYank}}
		},
	}
	e := s.newEditor(t.Context(), nil)
	var seen []LastWidget
	e.runFunc = func(_ string, in Line, _ Actions) (Line, bool) {
		seen = append(seen, in.Last)
		return in, true
	}
	// x, the widget; left arrow, the widget; the widget again; a bound
	// action, the widget; Return; then the widget first on the next line.
	e.in, e.out = typing("x\a\x1b[D\a\a\x0f\a\r"), &out
	if _, err := e.readLine(drawPrompt("$ ")); err != nil {
		t.Fatal(err)
	}
	e.in = typing("\a\r")
	if _, err := e.readLine(drawPrompt("$ ")); err != nil {
		t.Fatal(err)
	}
	want := []LastWidget{
		{Widget: WidgetSelfInsert, Known: true},
		{Widget: WidgetBackwardChar, Known: true},
		{Function: "w", Known: true},
		{Widget: WidgetYank, Known: true},
		{Accepted: true, Known: true},
	}
	if len(seen) != len(want) {
		t.Fatalf("the widget ran %d times, want %d: %+v", len(seen), len(want), seen)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("call %d told %+v, want %+v", i, seen[i], want[i])
		}
	}
}
