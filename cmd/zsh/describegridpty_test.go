// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// Options that share a description are listed on one row, the way the
// completion system's `list-grouped` style asks (#6178): `compdescribe -g`
// answers a grid a cell at a time, `compadd` adds the names under `-2V` and
// the blanks and descriptions under `-E`, and the editor draws it packed.
//
// The widget is the loop `_describe` runs, cut to its bones. Measured
// 2026-10-05 against zsh 5.9.2 on a 100-column pseudo-terminal with the same
// rc and `x ` typed before the key: `ex`, then `-c  -b  -a  -- same` and
// `-d          -- other` with each description padded to the screen, then
// `-y` and `-z` 49 columns apart. Before this the shell drew a row per name,
// `ex` once per name, and no grid.
func TestOptionsSharingADescriptionShareARow(t *testing.T) {
	control, screen := widgetSession(t, `w() {
  local -a E=(-J ej -X ex) G=(-a:same -b:same -c:same -d:other -y -z) args tm td; local csl
  compdescribe -I '' 60 '-- ' E -g G
  while compdescribe -g csl args tm td; do
    [[ -n $csl ]] && compstate[list]="$compstate[list] $csl"
    compadd "$args[@]" -d td -a tm
  done
}
zle -C w .list-choices w
bindkey '^T' w
`)
	if _, err := control.WriteString("x \x14"); err != nil {
		t.Fatalf("typing: %v", err)
	}
	for _, want := range []string{
		"-c  -b  -a  -- same",
		"-d          -- other",
		"-y" + strings.Repeat(" ", 47) + "-z",
	} {
		if err := screen.Await(want, widgetBudget); err != nil {
			t.Fatalf("want %q on the screen\n%v\n%q", want, err, smoke.LastLines(screen.Text(), 8))
		}
	}
	if n := strings.Count(screen.Text(), "\nex"); n != 1 {
		t.Errorf("the heading was drawn %d times, want once:\n%q", n, smoke.LastLines(screen.Text(), 8))
	}
	// Leave an empty line for the `exit` the session ends with.
	if _, err := control.WriteString("\x15"); err != nil {
		t.Fatalf("clearing the line: %v", err)
	}
}
