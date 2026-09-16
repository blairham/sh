// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// dash names no word in this refusal, which is the one sentence in the panel
// that does not — #2298.
//
// Measured 2026-09-16 with standard input on /dev/null: `dash --badopt`,
// `dash --xyz`, `dash --a` and `dash --login=x` each write exactly `<shell>:
// 0: Illegal option --` at status 2. The two dashes and nothing after them.
//
// Every word is asked rather than one, because a sentence that happened to
// drop the word would look identical on a single row, and because this is the
// column where reading the panel's majority would have written the word in.
func TestABadLongOptionNamesNoWord(t *testing.T) {
	for _, word := range []string{"--badopt", "--xyz", "--a", "--login=x"} {
		t.Run(word, func(t *testing.T) {
			var o, e bytes.Buffer
			sh := shell()
			sh.SystemStartupDirectory = t.TempDir()
			sh.Stdout, sh.Stderr = &o, &e
			code := driver.MainArgs(sh, []string{"dash", word})
			if code != 2 {
				t.Errorf("status %d, want 2", code)
			}
			if o.Len() > 0 {
				t.Errorf("stdout = %q, want nothing", o.String())
			}
			if want := "dash: 0: Illegal option --\n"; e.String() != want {
				t.Errorf("stderr = %q, want %q", e.String(), want)
			}
		})
	}
}
