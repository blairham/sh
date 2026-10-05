// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// The other three incremental searches, on a real terminal (#5904).
//
// zsh has four — backward and forward, plain and pattern — and this shell had
// only the first: `C-s` and `^X s` did nothing, `^X r` did nothing, and a
// startup file binding `history-incremental-pattern-search-backward` bound a
// widget that did not exist. Measured 2026-10-04 through a pty against
// /opt/homebrew/bin/zsh (zsh 5.9.2) with `FLOW_CONTROL` off, with the same
// four lines typed first; each row's marker is arithmetic, so only running
// the line the search found can draw it:
//
//	C-r alpha C-r C-s C-s Return   runs `alpha three`
//	^X r bra Return                runs `bravo two`
//	^X p b*o Return                runs `bravo two`   (^X p bound to the
//	                               pattern search)
//	a wrapper calling the pattern search by name, `[` Return
//	                               the call answers 2
//	Up×4, a wrapper calling the forward search by name, `thr` Return
//	                               the call answers 0, then `alpha three` runs
func TestTheFourSearchesOnATerminal(t *testing.T) {
	const up = "\x1b[A"
	rc := []string{
		// As in the measurement: with the option set, `C-s` is the
		// terminal's and never reaches the search (#5943).
		`unsetopt flowcontrol`,
		`bindkey '^Xp' history-incremental-pattern-search-backward`,
		`wp() { zle .history-incremental-pattern-search-backward; print -rn -- " ST$(( 40 + 2 ))=$?" }`,
		`wf() { zle .history-incremental-search-forward; print -rn -- " ST$(( 40 + 2 ))=$?" }`,
		`zle -N wp; zle -N wf; bindkey '^Xw' wp; bindkey '^Xf' wf`,
	}
	for _, tc := range []struct {
		name, keys, want string
	}{
		{"C-s turns a C-r search round and walks forward", "\x12alpha\x12\x13\x13\r", "RAN42alpha three"},
		{"^X r is the backward search", "\x18rbra\r", "RAN42bravo two"},
		{"a pattern search", "\x18pb*o\r", "RAN42bravo two"},
		{"a pattern half typed answers 2", "\x18w[\r", "ST42=2"},
		{"the forward search by name answers 0", up + up + up + up + "\x18fthr\r", "ST42=0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control, screen := widgetSession(t, rc...)
			for _, line := range []string{
				"echo RAN$((40+2))alpha one",
				"echo RAN$((40+2))bravo two",
				"echo RAN$((40+2))alpha three",
				"echo RAN$((40+2))charlie",
			} {
				widgetType(t, control, screen, line)
			}
			if _, err := control.WriteString(tc.keys); err != nil {
				t.Fatalf("typing %q: %v", tc.keys, err)
			}
			if err := screen.Await(tc.want, widgetBudget); err != nil {
				t.Fatalf("did not see %q: %v\n%s", tc.want, err,
					smoke.Readable(smoke.LastLines(screen.Text(), 8)))
			}
		})
	}
}

// A lower-case query ignores case and a leading `^` anchors, on the terminal
// (#5932). `xRAN` is reached only by the contiguous pass ignoring case: as a
// subsequence `r_a_n` outranks it, so the fallback alone would run the other
// line.
func TestTheSearchQueryIsReadTheZshWay(t *testing.T) {
	for _, tc := range []struct {
		name, keys, want string
	}{
		{"a lower-case query ignores case", "\x12ran\r", "xRAN42one"},
		{"a caret anchors", "\x12^echo r\r", "r_a_n42two"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control, screen := widgetSession(t)
			widgetType(t, control, screen, "echo xRAN$((40+2))one")
			widgetType(t, control, screen, ": echo r_a_n")
			widgetType(t, control, screen, "echo r_a_n$((40+2))two")
			if _, err := control.WriteString(tc.keys); err != nil {
				t.Fatalf("typing %q: %v", tc.keys, err)
			}
			if err := screen.Await(tc.want, widgetBudget); err != nil {
				t.Fatalf("did not see %q: %v\n%s", tc.want, err,
					smoke.Readable(smoke.LastLines(screen.Text(), 8)))
			}
		})
	}
}
