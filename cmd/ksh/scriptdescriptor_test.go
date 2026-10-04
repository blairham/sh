// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// The script ksh93 is running is held on descriptor 10, positioned where the
// shell has read to: the first number `{a}` allocates is 11, and `cat <&10`
// prints what is left of the script, empty lines straight after its own line
// skipped. Measured 2026-10-03 on ksh93u+ from script files. See
// driver.Shell.ScriptDescriptor.
func TestTheScriptIsHeldOnTen(t *testing.T) {
	out, errs, code := runKshScript(t, "exec {a}>/dev/null; echo a=$a\ncat <&10\n\n\n# rest\necho done\n")
	if want := "a=11\n# rest\necho done\ndone\n"; out != want || code != 0 {
		t.Errorf("got %q (stderr %q) at %d, want %q at 0", out, errs, code, want)
	}
}
