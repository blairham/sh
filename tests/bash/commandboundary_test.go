// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// The other end of the axis three shells hold: here the `command` word takes
// the named builtin's specialness away (#2741) and draws **no boundary of its
// own**, so a fatal error raised inside the builtin it ran ends the script
// exactly as it does without the word.
//
// Measured 2026-09-13 in subshells from a script file under `env -i`, under
// this shell's own name and under `sh` alike, both builds agreeing: each
// producer below ends the subshell with the word and without it, where ksh93,
// dash and BusyBox ash print `alive` with it (#2755).
//
// The control half is what makes the row about the word: a build that had
// stopped calling these errors fatal would pass a bare "it did not catch"
// assertion without running anything.
func TestTheCommandWordIsNoBoundaryHere(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup string
		text  string
	}{
		{"`${x?word}` firing", "", `echo "${NOPE?bad}"`},
		{"an unset name under `set -u`", "set -u\n", `echo "${NOPE}"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			out, st := runBash(t, dir, tc.setup+"eval '"+tc.text+"; echo inner'\necho after\n")
			if strings.Contains(out, "after") || st == 0 {
				t.Fatalf("bare %q gave %q at %d, want the script to stop", tc.text, out, st)
			}

			out, st = runBash(t, dir, tc.setup+"command eval '"+tc.text+"; echo inner'\necho after\n")
			if strings.Contains(out, "after") || st == 0 {
				t.Errorf("`command eval '%s'` gave %q at %d, want the error to reach past the word", tc.text, out, st)
			}
		})
	}
}
