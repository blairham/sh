// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strings"
	"testing"
)

// A `$( … )` whose opener ends the line is numbered from the **opener** in one
// preset and from the line its text begins on in the other four (#3362).
//
// Measured 2026-09-17 from a script file, `env -i PATH=/usr/bin:/bin LC_ALL=C
// <shell> s.sh` with stdin from /dev/null, `echo one` on line one and the
// substitution written from line two.
//
// The row with text after the `$(` is the control and is why this is about
// the opener rather than about the body: every column answers 2 for it. The
// two-blank row is what says it is not a constant offset of one — however
// many newlines stand between the opener and the first command, that command
// is the opener's line in the column that moves.
func TestASubstitutionOpenedAtTheEndOfALineIsNumberedFromItsOpener(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		// opener is what the preset that counts from the opener answers, and
		// physical what the other four do.
		opener, physical string
	}{
		{
			"the opener ends the line",
			"echo one\necho \"$(\necho \"L=$LINENO\"\n)\"\necho \"after=$LINENO\"\n",
			"one\nL=2\nafter=5\n", "one\nL=3\nafter=5\n",
		},
		{
			"a blank line after the opener",
			"echo one\necho \"$(\n\necho \"L=$LINENO\"\n)\"\n",
			"one\nL=2\n", "one\nL=4\n",
		},
		{
			"three blank lines after it",
			"echo one\necho \"$(\n\n\n\necho \"L=$LINENO\"\n)\"\n",
			"one\nL=2\n", "one\nL=6\n",
		},
		{
			"a second command in the body",
			"echo one\necho \"$(\necho x\necho \"L=$LINENO\"\n)\"\n",
			"one\nx\nL=3\n", "one\nx\nL=4\n",
		},
		{
			// The control.
			"text after the opener",
			"echo one\necho \"$(echo \"L=$LINENO\"\n)\"\n",
			"one\nL=2\n", "one\nL=2\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _, _ := splitRun(t, presets["bash"], tc.src); out != tc.opener {
				t.Errorf("the preset that counts from the opener wrote %q, want %q", out, tc.opener)
			}
			for _, preset := range []string{"zsh", "ksh", "dash", "ash", "posix"} {
				if out, _, _ := splitRun(t, presets[preset], tc.src); out != tc.physical {
					t.Errorf("%s wrote %q, want %q", preset, out, tc.physical)
				}
			}
		})
	}
}

// The same offset shows in a diagnostic and not only in `$LINENO`, because
// both are read off the one number the body's runner is given.
func TestADiagnosticInsideSuchABodyCarriesTheSameLine(t *testing.T) {
	const src = "echo one\necho \"$(\nnosuchcmd_zz\n)\"\n"
	if _, errs, _ := splitRun(t, presets["bash"], src); !strings.Contains(errs, "line 2:") {
		t.Errorf("wrote %q, want it located at the opener's line", errs)
	}
	if _, errs, _ := splitRun(t, presets["dash"], src); !strings.Contains(errs, ": 3:") {
		t.Errorf("wrote %q, want it located at the command's own line", errs)
	}
}
