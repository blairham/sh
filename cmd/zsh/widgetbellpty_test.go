// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// A widget that a key ran rings the bell when its function returns non-zero,
// and `unsetopt beep` silences that bell and every other the editor rings
// (#6108).
//
// Measured 2026-10-05 against zsh 5.9.2 through a pty, with `ab` typed and
// then the key:
//
//	w() { return 1 }                     \a
//	w() { false }                        \a
//	w() { return 0 }                     nothing
//	w() { zle w2; return 0 } (w2 fails)  nothing: only the widget the key ran
//	unsetopt beep; w() { return 1 }      nothing
//	unsetopt beep; ^G                    nothing
//	w() { zle send-break; return 1 }     nothing
//
// Each step types the key and then a widget that rewrites the line to a
// marker, so the bytes between the two are exactly what the key wrote.
func TestAFailingWidgetRingsTheBell(t *testing.T) {
	rc := `fail() { return 1 }; zle -N fail; bindkey '^T' fail
falsy() { false }; zle -N falsy; bindkey '^Xf' falsy
ok() { return 0 }; zle -N ok; bindkey '^Xo' ok
inner() { zle fail; return 0 }; zle -N inner; bindkey '^Xi' inner
sb() { zle send-break; return 1 }; zle -N sb; bindkey '^Xb' sb
mark() { BUFFER="MARK$((n+=1))" }; zle -N mark; bindkey '^Xm' mark`
	for _, tc := range []struct {
		name, setup, key string
		bell             bool
	}{
		{"return 1", "", "\x14", true},
		{"false", "", "\x18f", true},
		{"return 0", "", "\x18o", false},
		{"inner call", "", "\x18i", false},
		{"nobeep widget", "unsetopt beep", "\x14", false},
		{"nobeep send-break", "unsetopt beep", "\a", false},
		{"send-break", "", "\a", true},
		{"send-break from a widget", "", "\x18b", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control, screen := widgetSession(t, rc, tc.setup)
			if _, err := control.WriteString("ab"); err != nil {
				t.Fatal(err)
			}
			if err := screen.Await("ab", widgetBudget); err != nil {
				t.Fatalf("the line was not drawn: %v", err)
			}
			from := len(screen.Text())
			if _, err := control.WriteString(tc.key + "\x18m"); err != nil {
				t.Fatal(err)
			}
			if err := screen.Await("MARK1", widgetBudget); err != nil {
				t.Fatalf("no marker after %q: %v\n%q", tc.key, err, smoke.LastLines(screen.Text(), 6))
			}
			wrote := screen.Text()[from:]
			wrote = wrote[:strings.Index(wrote, "MARK1")]
			if got := strings.Contains(wrote, "\a"); got != tc.bell {
				t.Errorf("%q rang %v, want %v; wrote %q", tc.key, got, tc.bell, wrote)
			}
			// The marker off the line, so the session's own `exit` runs.
			if _, err := control.WriteString("\x15"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
