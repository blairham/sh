// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **`E` and `V` are `emacs` and `vi`, and the two are separate options.**
// Measured 2026-10-03 on dash 0.5.12: `set -E` is 0 and leaves `$-` as `E`;
// `set -o vi; set -o emacs` leaves both on and `$-` as `EV`; `set -V; set -E;
// set +V` leaves `E`. This refused `-E` as not implemented and ended the
// script at 2, and made the two names one keymap.
func TestEmacsAndViAreLettersAndTwoSwitches(t *testing.T) {
	src := `set -E; echo "a=$? [$-]"; set -o vi; echo "b=[$-]"; set +V; echo "c=[$-]"; set +E; set -V; echo "d=[$-]"; set -b -I; echo "e=[$-]"` + "\n"
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if want := "a=0 [E]\nb=[EV]\nc=[E]\nd=[V]\ne=[bVI]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
