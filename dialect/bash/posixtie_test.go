// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// `POSIXLY_CORRECT` and `set -o posix` are one state under two spellings.
//
// Measured 2026-09-23 on bash 5.3.20 in all four directions, which is the tie's
// whole contract: assigning the parameter turns the option on, unsetting it
// turns the option off, `set -o posix` writes `y` into the parameter, and
// `set +o posix` takes the parameter away. **Any** value turns it on, the empty
// string included, so nothing reads the value.
//
// Found as three lines of `appendop.tests`: the file sets `POSIXLY_CORRECT=1`
// and then depends on posix mode for whether an assignment preceding a special
// builtin persists, so a shell that ignored the parameter answered every line
// after it from the wrong mode (#4142).
func TestPosixlyCorrectAndTheOptionAreOneState(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ name, src, want string }{
		{"assigning turns it on", `POSIXLY_CORRECT=1; [[ -o posix ]] && echo on`, "on"},
		{"an empty value does too", `POSIXLY_CORRECT=; [[ -o posix ]] && echo on`, "on"},
		{"unsetting turns it off", `POSIXLY_CORRECT=1; unset POSIXLY_CORRECT; [[ -o posix ]] || echo off`, "off"},
		{"the option writes the parameter", `set -o posix; echo "[${POSIXLY_CORRECT-unset}]"`, "[y]"},
		{"turning it off removes it", `set -o posix; set +o posix; echo "[${POSIXLY_CORRECT-unset}]"`, "[unset]"},
		{"and removes one a script assigned", `POSIXLY_CORRECT=1; set +o posix; echo "[${POSIXLY_CORRECT-unset}]"`, "[unset]"},
		// The other tie this shell has, so the two cannot be collapsed: it
		// keeps its own value and its own name.
		{"the other tie is untouched", `set -o ignoreeof; echo "[${IGNOREEOF-unset}][${POSIXLY_CORRECT-unset}]"`, "[10][unset]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runBash(t, dir, c.src)
			if strings.TrimSpace(out) != c.want {
				t.Errorf("output %q, want %q", out, c.want)
			}
		})
	}
}

// And the one this tie does from the environment, which the other does not.
//
// Measured 2026-09-23 under `env NAME=1 bash -c 'set -o'`: an inherited
// `POSIXLY_CORRECT` turns `posix` **on** and an inherited `IGNOREEOF` leaves
// `ignoreeof` **off**. So the startup route is a fact about the pair rather than
// about ties, which is why interp.Runner.TieAlsoFiresFromTheEnvironment is a
// call of its own and not a default.
func TestOnlyOneTieFiresFromTheEnvironment(t *testing.T) {
	for _, c := range []struct{ name, variable, src, want string }{
		{"posix", "POSIXLY_CORRECT", `[[ -o posix ]] && echo on || echo off`, "on"},
		{"ignoreeof", "IGNOREEOF", `[[ -o ignoreeof ]] && echo on || echo off`, "off"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			t.Setenv(c.variable, "1")
			if code := driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-c", c.src}); code != 0 {
				t.Fatalf("status %d, stderr %q", code, errs.String())
			}
			if strings.TrimSpace(out.String()) != c.want {
				t.Errorf("%s inherited: %q, want %q", c.variable, out.String(), c.want)
			}
		})
	}
}
