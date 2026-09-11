// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/driver"
)

// `-t` in a startup file, in the session shape that found it (#1967).
//
// A startup file is where the operator is actually used — it is how a rc
// decides whether to draw color, load a prompt theme or install completions —
// and it is where the fixed `false` was found: #1927's session ran a real
// `~/.zshrc` on a pseudo-terminal and `[[ -t 0 ]]` on its first line took the
// non-terminal arm.
//
// The front end is the half this asserts. interp answers about the streams it
// was handed, so a front end that wrapped standard input on its way to a
// prompt would put the fixed false back without a single interp test noticing.
//
// The terminal goes on standard *input* and the assertion is read off a
// buffer on standard output — the arrangement #793 had to learn, and here it
// earns its keep twice over: descriptor 0 is a terminal and descriptor 1 is
// not, so one run answers both ways and neither a fixed true nor a fixed
// false passes it.
func TestTerminalTestInAStartupFileSeesTheTerminal(t *testing.T) {
	cleanStartupEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	rc := filepath.Join(home, "alternate")
	const probe = `[ -t 0 ] && echo B0T || echo B0F
test -t 0 && echo T0T || echo T0F
[ -t 1 ] && echo B1T || echo B1F
[ -t 2 ] && echo B2T || echo B2F
exit
`
	if err := os.WriteFile(rc, []byte(probe), 0o600); err != nil {
		t.Fatal(err)
	}

	_, tty := terminal(t)
	var out, errs bytes.Buffer
	sh := shell()
	sh.Semantics = bashLike()
	sh.Stdin, sh.Stdout, sh.Stderr = tty, &out, &errs
	code := driver.MainArgs(sh, []string{"testsh", "--rcfile", rc, "-i"})
	// Standard error is the second buffer, so descriptor 2 is not a terminal
	// either — which is what makes B2F evidence that the answer is read per
	// descriptor rather than taken once from the session.
	const want = "B0T\nT0T\nB1F\nB2F\n"
	if out.String() != want || code != 0 {
		t.Errorf("the startup file wrote %q at %d, want %q at 0 (stderr %q)",
			out.String(), code, want, errs.String())
	}
}
