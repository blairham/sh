// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `getopts` is the one builtin whose complaint here carries **no line**
// (#3438), and the one builtin whose refused name ends the script by which
// path inside it reached the freeze (#3183).
//
// interp proves what the axis and the diagnostic flag do; this file pins that
// this preset answers both the way the shell does. Measured 2026-09-17 on
// ksh93u+ 2012-08-01, as a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C` with stdin on /dev/null, and again through `-c` and through
// standard input.

// Every other diagnostic this shell writes names a location — `file: line N:`
// or `file[N]:` — and all three of the builtin's wordings write the name and
// then the message. The `readonly` line below is the contrast that makes the
// claim a claim rather than a missing prefix: the same run, the same shell,
// and a line number in front of it.
func TestGetoptsComplaintCarriesNoLine(t *testing.T) {
	for _, tc := range []struct{ name, spec, words, want string }{
		{"a missing argument", "a:", "-a", "ksh: -a: argument expected\n"},
		{"an unknown option", "a", "-x", "ksh: -x: unknown option\n"},
		{"a numeric argument", "n#", "-n q", "ksh: -n: numeric argument expected\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "OPTIND=1\ngetopts '" + tc.spec + "' o " + tc.words + "\n"
			if out, _ := getoptsRun(t, src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
	// The same shell, three lines in, still giving a line to everything else.
	const frozen = "OPTIND=1\nN=kept; readonly N\ngetopts 'a:' N -a\n"
	if out, _ := getoptsRun(t, frozen); out != "ksh: -a: argument expected\nksh: line 3: N: is read only\n" {
		t.Errorf("got %q, want the builtin's line with no number and the freeze's with one", out)
	}
}

// The freeze on the name operand, and the split this shell alone makes: the
// run that reports "no more options" ends the script at the builtin's own 2,
// and the same freeze over a letter the scan found reports and returns 2 with
// the script running on.
func TestGetoptsFrozenNameEndsTheScriptOnlyAtTheEndOfTheOptions(t *testing.T) {
	for _, tc := range []struct {
		name, params, want string
		status             int
	}{
		{"a letter the scan found", "set -- -a v", "ksh: line 3: N: is read only\nst=2\nafter\n", 0},
		{"the end of the options", "set -- x", "ksh: line 3: N: is read only\n", 2},
		{"a double dash", "set -- --", "ksh: line 3: N: is read only\n", 2},
		{"no words at all", "set --", "ksh: line 3: N: is read only\n", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.params + "\nN=kept; readonly N\ngetopts 'a:' N\necho \"st=$?\"\necho after\n"
			out, st, err := preset.Combined(t, dialecttest.Base{
				Name: "ksh", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
			}, src)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if st != tc.status {
				t.Errorf("status = %d, want %d", st, tc.status)
			}
		})
	}
}
