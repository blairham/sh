// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// A here-document's body is handed to the command open for reading only, so a
// write to it fails. Measured 2026-10-03 on ksh93u+ 2012-08-01: `{ echo w >&0;
// } <<EOF` answers 1 and the body still reads back (#5551).
func TestASpooledBodyIsOpenForReadingOnly(t *testing.T) {
	out, _, _ := runKshScript(t, "{ echo w >&0; } <<EOF\nx\nEOF\necho st=$?\ncat <<EOF\nbody\nEOF\n")
	if want := "st=1\nbody\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
