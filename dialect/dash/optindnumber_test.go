// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// TestOptindRefusesAValueThatIsNotANumber pins dash's check on every store
// into OPTIND, each row measured 2026-10-15 on dash 0.5.12 under `-c` and
// `env -i PATH=/usr/bin:/bin`: the refusal is a freeze's route with its own
// sentence, so an assignment, a declaration and a prefix end the script at 2,
// and `read`, `getopts` and a declaration under `command` report 2 and go on.
// See interp.Runner.RefuseValues (#6537).
func TestOptindRefusesAValueThatIsNotANumber(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`OPTIND=x; echo "after [$OPTIND]"`, "Illegal number: x\n"},
		{`OPTIND=; echo after`, "Illegal number: \n"},
		{`OPTIND=-1; echo after`, "Illegal number: -1\n"},
		{`OPTIND=2147483648; echo after`, "Illegal number: 2147483648\n"},
		{`OPTIND=0x10; echo after`, "Illegal number: 0x10\n"},
		{`export OPTIND=x; echo after`, "export: Illegal number: x\n"},
		{`readonly OPTIND=x; echo after`, "readonly: Illegal number: x\n"},
		{`f() { local OPTIND=x; echo in; }; f; echo after`, "local: Illegal number: x\n"},
		{`OPTIND=x true; echo after`, "Illegal number: x\n"},
		{`f() { echo in; }; OPTIND=x f; echo after`, "Illegal number: x\n"},
		{`OPTIND=x /usr/bin/true; echo after`, "Illegal number: x\n"},
		{`for OPTIND in 3 x; do echo "[$OPTIND]"; done; echo after`, "[3]\nIllegal number: x\n"},
		{`: $((OPTIND=-1)); echo after`, "Illegal number: -1\n"},
		{`eval OPTIND=x; echo after`, "eval: Illegal number: x\n"},
		{`command export OPTIND=x; echo "after $? [$OPTIND]"`, "export: Illegal number: x\nafter 2 [1]\n"},
		{`f() { command local OPTIND=x; echo "in $?"; }; f; echo after`, "local: Illegal number: x\nin 2\nafter\n"},
		{"read OPTIND <<E\nx\nE\necho \"after $? [$OPTIND]\"", "read: Illegal number: x\nafter 2 [1]\n"},
		{"read y OPTIND <<E\nb x\nE\necho \"after $? [$OPTIND] [$y]\"", "read: Illegal number: x\nafter 2 [1] [b]\n"},
		{"read OPTIND y <<E\nx b\nE\necho \"after $? [$OPTIND] [$y]\"", "read: Illegal number: x\nafter 2 [1] []\n"},
		{`getopts a OPTIND -a; echo "after $? [$OPTIND]"`, "getopts: Illegal number: a\nafter 2 [2]\n"},
		{`set -- a; getopts a OPTIND; echo "after $? [$OPTIND]"`, "getopts: Illegal number: ?\nafter 2 [1]\n"},
		// What passes the check, stored as written.
		{`OPTIND=" 3"; echo "[$OPTIND]"`, "[ 3]\n"},
		{`OPTIND="3 "; echo "[$OPTIND]"`, "[3 ]\n"},
		{`OPTIND=+3; echo "[$OPTIND]"`, "[+3]\n"},
		{`OPTIND=07; echo "[$OPTIND]"`, "[07]\n"},
		{`OPTIND=-0; echo "[$OPTIND]"`, "[-0]\n"},
		{`OPTIND=0000000002147483647; echo "[$OPTIND]"`, "[0000000002147483647]\n"},
		{`read OPTIND <<E` + "\n5\nE\n" + `echo "[$?] [$OPTIND]"`, "[0] [5]\n"},
		// The shell's own stores are numbers and never refused.
		{`set -- -a -b; getopts ab o; getopts ab o; echo "$o [$OPTIND]"`, "b [3]\n"},
	} {
		got, _ := runDash(t, t.TempDir(), tc.src)
		got = stripLocation(got)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// TestARefusedOptindStoreEndsTheScriptAtTwo pins the status the fatal rows
// above leave, measured the same day: 2, as a freeze's refusal there leaves.
func TestARefusedOptindStoreEndsTheScriptAtTwo(t *testing.T) {
	for _, src := range []string{
		`OPTIND=x`, `export OPTIND=x`, `OPTIND=x true`, `OPTIND=x /usr/bin/true`,
	} {
		if _, st := runDash(t, t.TempDir(), src+"; echo after"); st != 2 {
			t.Errorf("%s: status %d, want 2", src, st)
		}
	}
}

// stripLocation drops the shell-and-line location dash writes in front of a
// diagnostic, so each row reads as the sentence it measured.
func stripLocation(out string) string {
	lines := strings.SplitAfter(out, "\n")
	for i, l := range lines {
		if _, rest, ok := strings.Cut(l, ": 1: "); ok {
			lines[i] = rest
		}
	}
	return strings.Join(lines, "")
}
