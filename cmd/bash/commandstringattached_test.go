// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// `bash -cecho hi` — the command string welded to its letter. Measured
// 2026-10-03 on bash 5.3.20 with standard input on /dev/null: the bundle is
// read a letter at a time, `o` with no word behind it writes the whole `set -o`
// table to standard output with errexit and hashall on (`e` and `h` came
// first) and emacs on (the shell has not yet decided it is not interactive),
// and then the space is refused — at 1 and with no usage block, because
// errexit was already on. Ours refused the missing command string at 2.
func TestACommandStringWeldedToItsLetterListsAndThenFails(t *testing.T) {
	var o, e bytes.Buffer
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &o, &e
	code := driver.MainArgs(sh, []string{"bash", "-cecho hi"})
	if code != 1 {
		t.Errorf("status %d, want 1", code)
	}
	for _, row := range []string{"errexit        \ton", "hashall        \ton", "emacs          \ton", "xtrace         \toff"} {
		if !strings.Contains(o.String(), row+"\n") {
			t.Errorf("stdout lacks %q:\n%s", row, o.String())
		}
	}
	if strings.Contains(e.String(), "Usage:") || strings.Contains(e.String(), "requires an argument") {
		t.Errorf("stderr = %q, want the refused letter alone", e.String())
	}
}

// The emacs row of that listing is on because of *when* it is written and not
// because the welded `i` made the shell interactive: `bash -co`, with no `i`
// anywhere, lists it on too, while `bash -c 'set -o'` lists it off. Measured
// 2026-10-03 on bash 5.3.20 with standard input on /dev/null.
func TestTheInvocationListingReadsEmacsOn(t *testing.T) {
	var o, e bytes.Buffer
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &o, &e
	if code := driver.MainArgs(sh, []string{"bash", "-co"}); code != 2 {
		t.Errorf("status %d, want the missing command string's 2", code)
	}
	if !strings.Contains(o.String(), "emacs          \ton\n") {
		t.Errorf("stdout lacks emacs on:\n%s", o.String())
	}
	o.Reset()
	driver.MainArgs(sh, []string{"bash", "-c", "set -o"})
	if !strings.Contains(o.String(), "emacs          \toff\n") {
		t.Errorf("a script's listing lacks emacs off:\n%s", o.String())
	}
}

// `--posix` is `set -o posix` and the mode it enters can be left again —
// measured 2026-10-03 on bash 5.3.20, `bash --posix -c 'shopt -o posix'` is
// on and a `set +o posix` after it is off.
func TestThePosixLongOptionEntersTheMode(t *testing.T) {
	var o, e bytes.Buffer
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &o, &e
	code := driver.MainArgs(sh, []string{
		"bash", "--posix", "-c",
		`shopt -o posix; set +o posix; shopt -o posix`,
	})
	want := "posix               \ton\nposix               \toff\n"
	if code != 1 || o.String() != want {
		t.Errorf("got %q status %d (stderr %q), want %q at 1", o.String(), code, e.String(), want)
	}
}
