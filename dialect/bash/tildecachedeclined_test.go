// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A bare `~` here is `HOME` as it stands, and the cache GNU bash 5.3.20 keeps
// is a **recorded refusal** rather than a behavior this dialect reproduces.
//
// That build answers a written `~` from a copy of `HOME` its own assignments
// do not reach, refreshed whenever an environment is built for a child — so
// the same `~` is one home before an external command on the line and another
// after it. Measured 2026-09-22 on 5.3.20 and again 2026-09-23 on 5.3.15,
// `env -i PATH=/usr/bin:/bin LC_ALL=C HOME=/orig`, `--norc --noprofile`,
// every line of the shape `HOME=/h; … ; echo ~`:
//
//	                                5.3.20   5.3.15   3.2.57   zsh   ksh93   dash
//	nothing between                 /orig    /h       /h       /h    /h      /h
//	a builtin, `eval`, `.`          /orig    /h       /h       /h    /h      /h
//	`export HOME`, `export FOO=1`   /orig    /h       /h       /h    /h      /h
//	a subshell, a function call     /orig    /h       /h       /h    /h      /h
//	an external command             /h       /h       /h       /h    /h      /h
//	a pipeline, a `&` job           /h       /h       /h       /h    /h      /h
//	a command substitution          /h       /h       /h       /h    /h      /h
//
// **One patch range of one build.** The cache appeared inside a single
// release's patch series: 5.3.15 is the same release and reads the variable on
// every row, as bash's own 3.2 does, as zsh, ksh93, dash and BusyBox ash do,
// and as POSIX XCU 2.6.1 says. A behavior no other shell and no standard
// shares is an upstream regression, and the common denominator of real shells
// is what this core is for — so it is written down and not taken.
//
// It was modeled as an axis for a day, on the 5.3.20 measurement alone. What
// that cost is in docs/spec/grammar/expansion.md; the short version is that it
// made `glob.tests` disagree with the build the suite is graded in.
//
// Every case still reads `~` three times — before the assignment, after it,
// and after a child has been built — because a probe with fewer readings
// cannot tell the variable, a home frozen at startup and a refreshed cache
// apart. That trap put a wrong premise in two issues (#3484, #4039, #4156),
// and it is worth keeping the discriminating shape on the answer that is now
// the plain one.
func TestABareTildeReadsTheVariableAndNotACachedHome(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{
			"an ordinary word",
			`echo ~; HOME=/h; echo ~; /usr/bin/true; echo ~`,
		},
		{
			"an assignment's value",
			`x=~; echo "$x"; HOME=/h; x=~; echo "$x"; /usr/bin/true; x=~; echo "$x"`,
		},
		{
			"a tilde after a colon in an assignment",
			`v=a:~; echo "${v#a:}"; HOME=/h; v=a:~; echo "${v#a:}"; ` +
				`/usr/bin/true; v=a:~; echo "${v#a:}"`,
		},
		{
			// The variable beside it, which read the same way all along: the
			// row that used to be the odd one out is now the shape of all of
			// them.
			"the variable beside it",
			`echo "$HOME"; HOME=/h; echo "$HOME"; /usr/bin/true; echo "$HOME"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runTildeHome(t, tc.src)
			// The first reading is the startup home under every candidate
			// rule, so it is asserted apart from the pair that discriminates:
			// a case whose first line moved is measuring something else.
			if len(got) != 3 || got[0] != "/orig" {
				t.Fatalf("%s read %q, want three lines opening /orig", tc.src, got)
			}
			if pair := got[1] + " " + got[2]; pair != "/h /h" {
				t.Errorf("%s read %q after the assignment and after a child, want %q",
					tc.src, pair, "/h /h")
			}
		})
	}
}

// And the constructs that moved the copy in 5.3.20 move nothing here, which is
// the half a single-reading test cannot see: a shell that had taken the cache
// by accident would pass the cases above only by luck of where the child is.
func TestNoConstructMovesTheHomeATildeReads(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a command substitution", `HOME=/h; echo ~; v=$(true); echo ~`},
		{"a pipeline", `HOME=/h; echo ~; : | :; echo ~`},
		{"a background job", `HOME=/h; echo ~; : & wait; echo ~`},
		{"an external command", `HOME=/h; echo ~; /usr/bin/true; echo ~`},
		{"a subshell", `HOME=/h; echo ~; ( true ); echo ~`},
		{"a builtin", `HOME=/h; echo ~; true; echo ~`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runTildeHome(t, tc.src)
			if len(got) != 2 || got[0] != "/h" || got[1] != "/h" {
				t.Errorf("%s read %q, want /h both sides", tc.src, got)
			}
		})
	}
}

// runTildeHome runs src with a known startup home and hands back its lines.
//
// The environment is given rather than inherited, because the question is
// about the home the shell started with: a run that borrowed the machine's
// would be measuring the person running the tests.
func runTildeHome(t *testing.T, src string) []string {
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
