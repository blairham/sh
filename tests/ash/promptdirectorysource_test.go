// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
)

// `\w` here reads the directory the applet is **in**, not the `PWD`
// parameter — #4540, and the row that says this table is not bash's however
// much of it was copied across.
//
// Measured 2026-09-26 in the panel's own image,
// `alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b`,
// BusyBox v1.37.0, under `env -i PATH=/usr/bin:/bin LC_ALL=C HOME=/root
// TERM=dumb /bin/busybox ash -i` with the lines fed on a pipe — the applet
// draws its prompt on a pipe as readily as on a terminal, which is the same
// route the escape table in prompt.go was read through:
//
//	cd /usr; PWD=/bogus; <prompt>         /usr #
//	cd /usr;             <prompt>         /usr #
//
// Three controls, because a prompt that did not move at all would draw the
// first row too: `cd /etc` afterwards moves the prompt to `/etc`, `echo
// "[$PWD]"` writes `[/bogus]` so the assignment really landed, and `cd /root`
// draws `~ #`, which is the abbreviating code under test rather than some
// other one.
//
// bash is the other way round on the same shape — see
// interp.PromptStyle.CwdIsTheShellsOwnDirectory for the panel.
func TestAPromptReadsTheDirectoryTheShellIsIn(t *testing.T) {
	if !ash.PromptStyle().CwdIsTheShellsOwnDirectory {
		t.Error("CwdIsTheShellsOwnDirectory is off, so a written PWD would move this prompt")
	}
}
