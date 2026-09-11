// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// This shell selects no editing mode on its own, interactive or not (#1858).
//
// Measured on zsh 5.9.2, 2026-09-11, in a session at a real terminal as well
// as under `-c`: `[[ -o emacs ]]` and `[[ -o vi ]]` both answer 1 until a
// script selects one. So the name's default here is **off**, and the bare
// `setopt` listing says nothing about it — where ours answered 0 for `emacs`
// and then wrote a spurious `noemacs` row the moment a script chose `vi`,
// which is a deviation from a default this shell did not actually hold.
func TestNeitherEditingModeIsSelectedOnItsOwn(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		interactive     bool
	}{
		{
			name: "neither is on in a script",
			src:  `[[ -o emacs ]]; echo "emacs=$?"; [[ -o vi ]]; echo "vi=$?"`,
			want: "emacs=1\nvi=1\n",
		},
		{
			// And an interactive session says the same, which is what makes
			// this dialect's answer to the axis a `no` rather than a
			// deferral.
			name:        "nor in an interactive session",
			interactive: true,
			src:         `[[ -o emacs ]]; echo "emacs=$?"; [[ -o vi ]]; echo "vi=$?"`,
			want:        "emacs=1\nvi=1\n",
		},
		{
			// The listing row that came with it: selecting `vi` deselects
			// `emacs`, and with `emacs` already off there is no deviation to
			// report. `nohashdirs` is this shell's standing one and is here
			// so the assertion is the whole listing rather than a substring.
			name: "and selecting one writes only that one",
			src:  "setopt vi; setopt",
			want: "nohashdirs\nvi\n",
		},
		{
			name: "a script may still select emacs",
			src:  `setopt emacs; [[ -o emacs ]]; echo "emacs=$?"; setopt`,
			want: "emacs=0\nemacs\nnohashdirs\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			args := []string{"zsh", "-c", tc.src}
			if tc.interactive {
				args = []string{"zsh", "-i", "-c", tc.src}
			}
			if code := driver.MainArgs(zshWriting(&out, &errs), args); code != 0 {
				t.Fatalf("status %d, stderr %q", code, errs.String())
			}
			if out.String() != tc.want {
				t.Errorf("got %q, want %q", out.String(), tc.want)
			}
		})
	}
}
