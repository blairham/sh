// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// A leading zero under octalzeroes does not make a float or a base octal.
// Through the front end, so the line after the setopt is read with the
// option on, which is the only place it reaches the grammar.
//
// Measured 2026-10-02 on zsh 5.9.2, `-f -c` with the setopt on the line
// before (#5145).
func TestALeadingZeroDoesNotMakeAFloatOrABaseOctal(t *testing.T) {
	src := "setopt octalzeroes\nprint $(( 09.5 )) $(( 07.5 )) $(( 01e2 )) $(( 08#77 )) $(( 010 )) $(( 016 ))"
	if got, want := runZshInLocale(t, "C", src), "9.5 7.5 100. 63 8 14\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
