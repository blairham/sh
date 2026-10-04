// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// An interactive shell with no terminal says so behind `$0`, not behind its
// own name. Measured 2026-10-04 on BusyBox ash 1.37.0 in the pinned Alpine
// image, standard input on /dev/null: `ash -i -c 'echo main' name A` is
// `name: can't access tty; job control turned off`, and `/bin/ash -i -c` is
// `/bin/ash: …` (#5723).
func TestTheNoTerminalRemarkNamesTheZerothParameter(t *testing.T) {
	for _, tc := range []struct {
		argv []string
		want string
	}{
		{[]string{"ash", "-i", "-c", "echo main", "name", "A"}, "name: can't access tty; job control turned off\n"},
		{[]string{"/bin/ash", "-i", "-c", "echo main"}, "/bin/ash: can't access tty; job control turned off\n"},
	} {
		t.Run(tc.argv[0]+" "+strings.Join(tc.argv[4:], " "), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			devnull, err := os.Open(os.DevNull)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = devnull.Close() })
			var o, e bytes.Buffer
			sh := shell()
			sh.SystemStartupDirectory = t.TempDir()
			sh.Stdout, sh.Stderr, sh.Stdin = &o, &e, devnull
			driver.MainArgs(sh, tc.argv)
			if !strings.Contains(e.String(), tc.want) {
				t.Errorf("stderr = %q, want %q in it", e.String(), tc.want)
			}
		})
	}
}
