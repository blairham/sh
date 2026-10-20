// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestDirsAbbreviatesTheHomeOnlyAtABoundary is #6025's bash half: the
// prelude's `dirs` wrote `${d/#$HOME/\~}`, so `HOME=/` drew `/usr` as `~usr`
// and an empty `HOME` put `~` in front of everything. Measured 2026-10-05 on
// bash 5.3 (/opt/homebrew/bin/bash), from /tmp with /usr pushed.
func TestDirsAbbreviatesTheHomeOnlyAtABoundary(t *testing.T) {
	out, st := runBashPrelude(t, t.TempDir(), `cd /tmp; pushd /usr >/dev/null
HOME=/; dirs; dirs +0
HOME=/us; dirs
HOME=/usr/; dirs
HOME=; dirs
HOME=/usr; dirs; dirs -v +0; dirs -l
pushd /usr/bin
HOME=/; cd /; dirs`)
	want := strings.Join([]string{
		"/usr /tmp",
		"/usr",
		"/usr /tmp",
		"/usr /tmp",
		"/usr /tmp",
		"~ /tmp",
		" 0  ~",
		"/usr /tmp",
		"~/bin ~ /tmp",
		"/ /usr /tmp",
	}, "\n") + "\n"
	if st != 0 || out != want {
		t.Errorf("status %d, output:\n%s\nwant:\n%s", st, out, want)
	}
}
