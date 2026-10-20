// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A parameter of the shell's own that an `unset` removed has no row in a
// whole-table listing, whatever attributes it kept for its next value — and
// the next value still reaches a child. Measured 2026-10-03 on zsh 5.9.2
// under `env -i PATH=/usr/bin:/bin`, `-f` (#5600).
func TestARemovedShellOwnNameIsNoRowInAListing(t *testing.T) {
	src := `export HOME=/x TERM=t; unset HOME TERM COLUMNS
for form in + +x ''; do typeset $form | /usr/bin/grep -cE '^(HOME|TERM|COLUMNS)$|^(HOME|TERM|COLUMNS)='; done
HOME=/y; typeset + | /usr/bin/grep -c '^HOME$'; /usr/bin/env | /usr/bin/grep '^HOME='`
	if out, _ := runZsh(t, t.TempDir(), src); out != "0\n0\n0\n1\nHOME=/y\n" {
		t.Errorf("got %q", out)
	}
}
