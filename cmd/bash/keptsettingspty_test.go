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

// What bash keeps of the terminal settings a command leaves behind (#6105).
// Measured 2026-10-05 against bash 5.3 through a pty, reading `stty -a` in the
// command after:
//
//	sh -c 'stty -echo -icanon -ixon'        -echo icanon -ixon
//	sh -c 'stty -ixon; kill -TERM $$'       ixon
//
// bash keeps what a command changed, `-echo` included, with line buffering
// back on. It keeps nothing of a command a signal ended. zsh differs on both;
// see cmd/zsh's TestTheNextCommandGetsTheSettingsTheLastOneLeft.
func TestBashKeepsTheSettingsACommandLeft(t *testing.T) {
	for _, tc := range []struct {
		name string
		line string
		want map[string]bool
	}{
		{"exited", "sh -c 'stty -echo -icanon -ixon'", map[string]bool{"echo": false, "icanon": true, "ixon": false}},
		{"signaled", "sh -c 'stty -ixon; kill -TERM $$'", map[string]bool{"ixon": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control, screen := interruptSession(t, "")
			interruptAnswer(t, control, screen, tc.line, nil)
			out := filepath.Join(t.TempDir(), "stty")
			interruptAnswer(t, control, screen, "stty -a > "+out, nil)
			b, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]bool{}
			for _, w := range regexp.MustCompile(`[\s;]+`).Split(string(b), -1) {
				if name, off := strings.CutPrefix(w, "-"); off {
					got[name] = false
				} else if w != "" {
					got[w] = true
				}
			}
			for flag, on := range tc.want {
				if v, ok := got[flag]; !ok || v != on {
					t.Errorf("after %s: %s is %v (present %v), want %v", tc.line, flag, v, ok, on)
				}
			}
		})
	}
}
