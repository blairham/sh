// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// `login_shell` and `restricted_shell` are indicators rather than switches.
//
// Measured 2026-09-10, every one of the sixty names bash 5.3.15 lists asked
// for a `shopt -s`, then queried, then a `shopt -u`, then queried again, and
// the same sweep against bash 3.2.57. These two are the only names that do
// not move: both report 0 with nothing on standard error and both stay
// exactly where they were. The two bashes agree, so it is not an axis.
//
// This shell refused them as `not implemented`, which is the answer for a
// behavior we have not built — and there is no behavior missing here, because
// bash promises nothing either. #1709: `shopt -p` in a login shell writes
// `shopt -s login_shell`, and a harness sources that dump back.

// TestAnIndicatorTakesARequestAndDoesNotMove is the fix, both names and both
// directions.
func TestAnIndicatorTakesARequestAndDoesNotMove(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`shopt -s login_shell; echo "s=$?"`, "s=0\n"},
		{`shopt -u login_shell; echo "s=$?"`, "s=0\n"},
		{`shopt -s restricted_shell; echo "s=$?"`, "s=0\n"},
		{`shopt -u restricted_shell; echo "s=$?"`, "s=0\n"},
		// Taken, and nothing moved: the name still reads off, at status 1.
		{`shopt -s login_shell; shopt login_shell; echo "q=$?"`, "login_shell         \toff\nq=1\n"},
		{`shopt -s restricted_shell; shopt -p restricted_shell; echo "p=$?"`, "shopt -u restricted_shell\np=1\n"},
		// Still listed, which is the other half of the pairing.
		{`shopt | grep -c -E "^(login_shell|restricted_shell) "`, "2\n"},
	} {
		var out, errs bytes.Buffer
		code := driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-c", c.src})
		if out.String() != c.want || errs.String() != "" || code != 0 {
			t.Errorf("%s = %q / %q status %d, want %q and nothing said",
				c.src, out.String(), errs.String(), code, c.want)
		}
	}
}

// TestLoginShellReportsTheInvocation is the half that makes the indicator
// worth having: it answers what the shell *is*. bash keeps the fact here
// rather than in `$-`, which is measured — see docs/spec/invocation.md.
func TestLoginShellReportsTheInvocation(t *testing.T) {
	for _, c := range []struct {
		name   string
		argv   []string
		want   string
		status int
	}{
		// The status is the query's answer, which is bash's: off is 1.
		{"not a login shell", []string{"bash", "-c", `shopt -p login_shell`}, "shopt -u login_shell\n", 1},
		{"-l says so", []string{"bash", "-l", "-c", `shopt -p login_shell`}, "shopt -s login_shell\n", 0},
		// And the request still moves nothing in the other direction.
		{"-l and a shopt -u", []string{"bash", "-l", "-c", `shopt -u login_shell; shopt -p login_shell`}, "shopt -s login_shell\n", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			code := driver.MainArgs(bashShell(&out, &errs), c.argv)
			if out.String() != c.want || code != c.status || errs.String() != "" {
				t.Errorf("%v = %q / %q status %d, want %q status %d",
					c.argv, out.String(), errs.String(), code, c.want, c.status)
			}
		})
	}
}

// TestAShoptDumpSourcesBack is the round trip, and the login-shell one is the
// case that needs the indicator: the dump a login shell writes holds
// `shopt -s login_shell`, and the shell that wrote it has to be able to read
// it back.
func TestAShoptDumpSourcesBack(t *testing.T) {
	for _, argv := range [][]string{
		{"bash", "-c", "shopt -p"},
		{"bash", "-l", "-c", "shopt -p"},
	} {
		var out, errs bytes.Buffer
		if code := driver.MainArgs(bashShell(&out, &errs), argv); code != 0 || errs.String() != "" {
			t.Fatalf("%v: %q / %q status %d", argv, out.String(), errs.String(), code)
		}
		dump := out.String()
		path := filepath.Join(t.TempDir(), "snap.sh")
		if err := os.WriteFile(path, []byte(dump), 0o600); err != nil {
			t.Fatal(err)
		}
		out.Reset()
		errs.Reset()
		code := driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-c", ". " + path + "\necho COMMAND-RAN\n"})
		if out.String() != "COMMAND-RAN\n" || errs.String() != "" || code != 0 {
			t.Errorf("sourcing %v's dump: %q / %q status %d, want the command to run with nothing said",
				argv, out.String(), errs.String(), code)
		}
	}
	// And the login dump really does carry the line that used to be refused,
	// so the round trip above is not passing because it was never tested.
	var out, errs bytes.Buffer
	driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-l", "-c", "shopt -p"})
	if !strings.Contains(out.String(), "shopt -s login_shell\n") {
		t.Errorf("a login shell's dump %q does not hold the line this is about", out.String())
	}
}
