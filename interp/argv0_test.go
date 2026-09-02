// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// A command names itself from argv[0], and what belongs there is the word that
// was typed — not the path PATH resolved to.
//
// os/exec sets both from its one argument, so the two have to be pulled apart
// again. Unanimous across the panel, and invisible until something fails:
// then it is in the output of a program the shell did not write, which is why
// a whole-machine run sweep had eighteen lines differing by nothing else.
func TestACommandIsNamedAsItWasWritten(t *testing.T) {
	// A command that prints its own argv[0] and nothing else, without
	// depending on any particular error message: `sh -c 'echo $0'` is run by
	// name, so what it prints is what it was handed.
	for _, tc := range []struct{ name, src, want string }{
		{
			"found on PATH", `sh -c 'echo "[$0]"' `, "[sh]",
		},
		{
			// A command named with a slash is already its own name, so this
			// is the case that must *not* change.
			"named by a path", `/bin/sh -c 'echo "[$0]"' `, "[/bin/sh]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, nil)
			if st != 0 {
				t.Fatalf("status %d: %s", st, out)
			}
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("argv[0] = %q, want %q", got, tc.want)
			}
		})
	}
}

// `exec` replaces the shell, and the name it hands over follows the same rule
// — with `-a` overriding it, which is the whole point of that option.
func TestExecNamesTheCommandTheSameWay(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// In a subshell, so the test's own process survives being replaced.
		{"the word as written", `( exec sh -c 'echo "[$0]"' )`, "[sh]"},
		{"or whatever -a says", `( exec -a chosen sh -c 'echo "[$0]"' )`, "[chosen]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, nil)
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("argv[0] = %q, want %q", got, tc.want)
			}
		})
	}
}
