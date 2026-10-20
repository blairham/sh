// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `$NULLCMD` authorizes a redirection with no command on **both** sides, and
// `$READNULLCMD` only chooses what runs once it has.
//
// With `NULLCMD` unset or emptied, `<f` is `redirection with no command` at 1
// even with `READNULLCMD` pointed at something that works — this shell read
// the reading parameter first and ran it, so the line succeeded. That is what
// `A04redirect.ztst` stops on under `READNULLCMD with NULLCMD unset`.
//
// Measured 2026-09-29 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`), script files
// with standard input on the null device:
//
//	unset NULLCMD; READNULLCMD=cat; <out1     redirection with no command, 1
//	NULLCMD=;      READNULLCMD=cat; <out1     the same
//	NULLCMD=:;     READNULLCMD=cat; <out1     reads the file
//	NULLCMD=:;     unset READNULLCMD; <out1   0, nothing written
//
// So emptying the writer disables the construct outright, and emptying the
// reader falls back to the writer — two different answers to two different
// parameters, which is the whole reason the order of the two reads matters.
func TestTheWritingNullCommandAuthorisesBothSides(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			"unset, with a reader that would work",
			"print body > out1\nunset NULLCMD\nREADNULLCMD=cat\n<out1\n", "", 1,
		},
		{
			"emptied, with a reader that would work",
			"print body > out1\nNULLCMD=\nREADNULLCMD=cat\n<out1\n", "", 1,
		},
		{
			"unset, on the writing side too",
			"unset NULLCMD\nREADNULLCMD=cat\n>out2\n", "", 1,
		},
		// The controls. A writer that is set authorizes the construct, and
		// then the reader decides.
		{"set, and the reader reads", "print body > out1\nNULLCMD=:\nREADNULLCMD=cat\n<out1\n", "body\n", 0},
		{"set, with no reader", "print body > out1\nNULLCMD=:\nunset READNULLCMD\n<out1\n", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := pipeMultioRun(t, tc.src)
			if tc.status == 0 {
				if out != tc.want || st != 0 {
					t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
				}
				return
			}
			if st != tc.status {
				t.Errorf("status %d, want %d — out %q", st, tc.status, out)
			}
			if want := "redirection with no command"; !contains(out, want) {
				t.Errorf("said %q, want it to mention %q", out, want)
			}
		})
	}
}
