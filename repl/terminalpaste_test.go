// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"testing"

	"github.com/blairham/sh/internal/terminfofixture"
)

// The paste markers are turned off for a terminal that cannot take them: one
// of the dialect's names, or a `$TERM` with no description. Measured
// 2026-10-07 on bash 5.3.20 through a pseudo-terminal; see
// EditorStyle.BracketedPasteOffForTerminals (#6310).
func TestATerminalThatCannotTakeThePasteMarkersTurnsThemOff(t *testing.T) {
	const setting = ".test.paste"
	db := terminfofixture.Database(t,
		terminfofixture.Description{Name: "fixtureterm"},
		terminfofixture.Description{Name: "dumbx"})
	shell := func(vars map[string]string) Shell {
		vars["TERMINFO"] = db
		return Shell{
			Runner: newTestRunner(vars),
			Editor: EditorStyle{
				BracketedPaste:                       true,
				BracketedPasteSetting:                setting,
				BracketedPasteOffForTerminals:        []string{"dumb", "vt52", "emacs"},
				BracketedPasteOffWithoutADescription: true,
			},
			counts: &counts{},
		}
	}
	paste := func(s Shell) string {
		v, _ := s.Runner.GetVar(setting)
		return v
	}
	for _, c := range []struct {
		name string
		vars map[string]string
		want string
	}{
		{"a named terminal", map[string]string{"TERM": "vt52"}, "off"},
		{"dumb", map[string]string{"TERM": "dumb"}, "off"},
		{"no description", map[string]string{"TERM": "unknownterm"}, "off"},
		{"TERM empty", map[string]string{"TERM": ""}, "off"},
		{"TERM unset", map[string]string{}, "off"},
		{"a described terminal", map[string]string{"TERM": "fixtureterm"}, ""},
		{"a name is a name, not a prefix", map[string]string{"TERM": "dumbx"}, ""},
		{"decided before the first line", map[string]string{"TERM": "dumb", setting: "on"}, "on"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := shell(c.vars)
			s.takeUpTheTerminal()
			if got := paste(s); got != c.want {
				t.Errorf("setting %q, want %q", got, c.want)
			}
		})
	}

	// And again each time `$TERM` changes, never turning it back on.
	t.Run("a change of terminal", func(t *testing.T) {
		s := shell(map[string]string{"TERM": "fixtureterm"})
		s.takeUpTheTerminal()
		s.Runner.SetVar("TERM", "dumb")
		s.takeUpTheTerminal()
		if got := paste(s); got != "off" {
			t.Fatalf("after TERM=dumb the setting is %q, want off", got)
		}
		s.Runner.SetVar("TERM", "fixtureterm")
		s.takeUpTheTerminal()
		if got := paste(s); got != "off" {
			t.Errorf("after TERM=fixtureterm the setting is %q, want it left off", got)
		}
		s.Runner.SetVar(setting, "on")
		s.takeUpTheTerminal()
		if got := paste(s); got != "on" {
			t.Errorf("with TERM unchanged the setting is %q, want what bind left", got)
		}
	})
}
