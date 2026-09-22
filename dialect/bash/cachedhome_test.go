// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A bare `~` here is a **copy** of `HOME` that the script's own assignments
// do not reach, and that is refreshed whenever an environment is built for a
// child. It is this shell alone in the panel — its own 3.2 reads the
// variable, and so do zsh, ksh93, dash and BusyBox ash, which is what
// interp/tildecurrenthome_test.go pins for every other column.
//
// Measured 2026-09-22 on bash 5.3.20, `env -i PATH=/usr/bin:/bin LC_ALL=C
// HOME=/orig`, `--norc --noprofile`, every line from `-c` and every one of
// them of the shape `HOME=/h; … ; echo ~`:
//
//	nothing between                 /orig      a builtin (`true`)        /orig
//	`export HOME`                   /orig      `export FOO=1`            /orig
//	`eval "x=1"`                    /orig      `. /dev/null`             /orig
//	a subshell `( true )`           /orig      `unset HOME`              /orig
//	a function call                 /orig
//
//	an external command             /h         a pipeline                /h
//	a background job and a `wait`   /h         a command substitution    /h
//
// Three readings are separated by those rows and only one survives all of
// them. The variable would answer `/h` throughout; a home frozen at startup
// would answer `/orig` throughout; a cache a child's environment refreshes
// answers the split above. #3484 and #4039 were each filed on the frozen
// reading and each was wrong about `HOME=/h; date >/dev/null; cd ~`, which
// goes to the **new** home here.
//
// So every case below reads `~` three times — before the assignment, after
// it, and after a child has been built. A probe with fewer readings cannot
// tell the three apart, which is the trap that put a wrong premise in two
// issues (#4156).
func TestABareTildeReadsACachedHome(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an ordinary word",
			`echo ~; HOME=/h; echo ~; /usr/bin/true; echo ~`,
			"/orig /h",
		},
		{
			"an assignment's value",
			`x=~; echo "$x"; HOME=/h; x=~; echo "$x"; /usr/bin/true; x=~; echo "$x"`,
			"/orig /h",
		},
		{
			"a tilde after a colon in an assignment",
			`v=a:~; echo "${v#a:}"; HOME=/h; v=a:~; echo "${v#a:}"; ` +
				`/usr/bin/true; v=a:~; echo "${v#a:}"`,
			"/orig /h",
		},
		{
			// The variable itself is not the copy and never was: `$HOME` is
			// the assigned value on the line where `~` is still the old one,
			// which is the split seen from the other side.
			"the variable beside it",
			`echo "$HOME"; HOME=/h; echo "$HOME"; /usr/bin/true; echo "$HOME"`,
			"/h /h",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runCachedHome(t, tc.src)
			// The first reading is the startup home under every one of the
			// three candidate rules, so it is asserted apart from the pair
			// that discriminates: a case whose first line moved is measuring
			// something else entirely.
			if len(got) != 3 || got[0] != "/orig" {
				t.Fatalf("%s read %q, want three lines opening /orig", tc.src, got)
			}
			if pair := got[1] + " " + got[2]; pair != tc.want {
				t.Errorf("%s read %q after the assignment and after a child, want %q",
					tc.src, pair, tc.want)
			}
		})
	}
}

// A subshell takes the copy with it and moves its own, which is the half that
// says the refresh is not a message sent back to the parent: measured the same
// day, `HOME=/h; ( /usr/bin/true; echo ~ )` is `/h` inside the parentheses and
// a bare `~` after them is still `/orig`.
func TestASubshellMovesItsOwnCachedHome(t *testing.T) {
	got := runCachedHome(t, `HOME=/h; ( /usr/bin/true; echo ~ ); echo ~`)
	if want := []string{"/h", "/orig"}; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("read %q, want %q", got, want)
	}
}

// And `~+`, `~-` and `~user` are outside it — it is `$HOME` behind a written
// `~` alone. Measured the same day: `~+` and `~-` follow `PWD` and `OLDPWD`
// with no copy anywhere, and `~root` reads the password database.
func TestOnlyTheHomeTildeIsCached(t *testing.T) {
	got := runCachedHome(t, `cd /; cd /tmp; HOME=/h; echo ~+; echo ~-`)
	if want := []string{"/tmp", "/"}; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("read %q, want %q", got, want)
	}
}

// runCachedHome runs src with a known startup home and hands back its lines.
//
// The environment is given rather than inherited, because what the copy is
// taken from is the environment the shell started with: a run that borrowed
// the machine's would be measuring the person running the tests.
func runCachedHome(t *testing.T, src string) []string {
	t.Helper()
	var out, errs bytes.Buffer
	sh := bashShell(&out, &errs)
	sh.Env = []string{"HOME=/orig", "PATH=/usr/bin:/bin", "LC_ALL=C"}
	if code := driver.MainArgs(sh, []string{"bash", "-c", src}); code != 0 {
		t.Fatalf("status = %d; out %q, stderr %q", code, out.String(), errs.String())
	}
	if errs.Len() != 0 {
		t.Fatalf("stderr = %q", errs.String())
	}
	return strings.Split(strings.TrimSpace(out.String()), "\n")
}
