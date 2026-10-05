// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The terminal settings a command leaves behind are the ones the next command
// gets (#6105). Before this, the session put back the discipline it captured
// at startup before every command, so `stty -ixon` typed at the prompt lasted
// only for the command that ran it.
//
// Measured 2026-10-05 against zsh 5.9.2 through a pty, reading `stty -a` in
// the command after:
//
//	stty -ixon                                  -ixon
//	ttyctl -f; stty -ixon                       ixon
//	ttyctl -f; ttyctl -u; stty -ixon            -ixon
//	sh -c 'stty -echo -icanon -isig -icrnl'     echo icanon -isig -icrnl
//	sh -c 'stty -ixon; kill -TERM $$'           -ixon
//
// The fourth row is zsh keeping what a command changed with line buffering
// and echo turned back on. The fifth is zsh keeping the settings of a command
// a signal ended, where bash puts back what was there before it.
func TestTheNextCommandGetsTheSettingsTheLastOneLeft(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines []string
		want  map[string]bool
	}{
		{"kept", []string{"stty -ixon"}, map[string]bool{"ixon": false}},
		{"signaled", []string{"sh -c 'stty -ixon; kill -TERM $$'"}, map[string]bool{"ixon": false}},
		{"frozen", []string{"ttyctl -f", "stty -ixon"}, map[string]bool{"ixon": true}},
		{"unfrozen", []string{"ttyctl -f", "ttyctl -u", "stty -ixon"}, map[string]bool{"ixon": false}},
		{
			"sanitized",
			[]string{"sh -c 'stty -echo -icanon -isig -icrnl'"},
			map[string]bool{"echo": true, "icanon": true, "isig": false, "icrnl": false},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control, screen := widgetSession(t)
			for _, l := range tc.lines {
				widgetType(t, control, screen, l)
			}
			out := filepath.Join(t.TempDir(), "stty")
			widgetType(t, control, screen, "stty -a > "+out)
			got := sttyFlags(t, out)
			for flag, on := range tc.want {
				if v, ok := got[flag]; !ok || v != on {
					t.Errorf("%s: %s is %v (present %v), want %v", strings.Join(tc.lines, "; "), flag, v, ok, on)
				}
			}
		})
	}
}

// sttyFlags reads the on/off flags out of a saved `stty -a`.
func sttyFlags(t *testing.T, path string) map[string]bool {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	flags := map[string]bool{}
	for _, w := range regexp.MustCompile(`[\s;]+`).Split(string(b), -1) {
		if name, off := strings.CutPrefix(w, "-"); off {
			flags[name] = false
		} else if w != "" {
			flags[w] = true
		}
	}
	return flags
}
