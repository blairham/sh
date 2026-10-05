// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// A write to `$pipestatus` lands in the record, each value read as C's atoi
// reads a number, and an array literal's bare assignment writes the record
// as a command does (#6088). See interp.Runner.WritePipelineStatus and
// interp.Semantics.ArrayAssignmentUpdatesPipelineStatus.
//
// Measured 2026-10-05 on zsh 5.9.2 (`zsh -fc`); every expected line is that
// shell's. This shell listed the stored write and left the record alone.
func TestAZshPipestatusTakesAWrite(t *testing.T) {
	const tail = `; print -r -- "$pipestatus"`
	for _, c := range []struct{ src, want string }{
		{"true|false; pipestatus[1]=9" + tail, "9 1\n"},
		{"true|false; pipestatus[3]=9" + tail, "0 1 9\n"},
		{"true|false; pipestatus[1]=1+1" + tail, "1 1\n"},
		{"true|false; pipestatus[1]+=3" + tail, "3 1\n"},
		{"true|false; pipestatus=5" + tail, "5\n"},
		{"true|false; pipestatus=abc" + tail, "0\n"},
		{"true|false; pipestatus=(5 6); typeset -p pipestatus", "typeset -a pipestatus=( 0 )\n"},
		{"true|false; typeset -a pipestatus=(z); typeset -p pipestatus", "typeset -a pipestatus=( 0 )\n"},
		{"true|false; pipestatus[1,2]=(8)" + tail, "0\n"},
		// An array literal is a command for the record; the other bare
		// assignments are not.
		{"true|false; arr=(1 2)" + tail, "0\n"},
		{"true|false; arr[1]=3" + tail, "0 1\n"},
		{"true|false; x=1" + tail, "0 1\n"},
	} {
		t.Setenv("HOME", t.TempDir())
		out, errs, _ := prompt(t, "", "zsh", "-f", "-c", c.src)
		if out != c.want {
			t.Errorf("%q: stdout %q, want %q (stderr %q)", c.src, out, c.want, errs)
		}
	}
}
