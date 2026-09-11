// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// This shell chooses emacs when it becomes interactive and neither mode
// before that (#1858).
//
// Measured 2026-09-11 on bash 5.3.15, and the same on the 3.2 macOS ships and
// under an `argv[0]` of `sh`:
//
//	bash -c 'set -o'     emacs off, vi off
//	bash -i -c 'set -o'  emacs on,  vi off
//
// The `-i` row with no terminal attached is what says the trigger is
// interactivity rather than a tty. Ours reported `emacs on` for a script,
// which is a keymap claimed by a shell with no line to edit.
func TestTheEditingModeIsChosenWhenTheShellBecomesInteractive(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		emacs string
	}{
		{"a script chooses neither", []string{"bash", "-c", "set -o"}, "off"},
		{"and `-i` chooses emacs", []string{"bash", "-i", "-c", "set -o"}, "on"},
		// Chosen off is not never chosen, and this is the shell the two read
		// differently in: an interactive session that turned emacs off stays
		// off rather than falling back to the default.
		{"turning it off in one is remembered", []string{"bash", "-i", "-c", "set +o emacs; set -o"}, "off"},
		// And turning off the mode that is not selected changes nothing.
		{"turning the other off is not", []string{"bash", "-i", "-c", "set +o vi; set -o"}, "on"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			if code := driver.MainArgs(bashShell(&out, &errs), tc.args); code != 0 {
				t.Fatalf("status %d, stderr %q", code, errs.String())
			}
			if !hasRow(out.String(), "emacs", tc.emacs) {
				t.Errorf("want emacs %s; listing was %q", tc.emacs, out.String())
			}
			// `vi` is off throughout, which is what keeps the row above a
			// statement about *which* mode rather than about the listing.
			if !hasRow(out.String(), "vi", "off") {
				t.Errorf("want vi off; listing was %q", out.String())
			}
		})
	}
}

// hasRow reads one row of a `set -o` listing without depending on its padding.
func hasRow(listing, name, state string) bool {
	for _, line := range strings.Split(listing, "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[0] == name && f[1] == state {
			return true
		}
	}
	return false
}
