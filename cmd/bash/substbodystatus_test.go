// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// A refusal raised while reading a substitution's body costs 1 when bash is
// interactive and its program is a `-c` string — whether the input ran out
// inside the body or the body would not parse — where the same string without
// `-i` costs 2 or 127 and a script costs 2 either way (#6267).
//
// Measured 2026-10-06 against /opt/homebrew/bin/bash 5.3.20, `env -i
// PATH=/usr/bin:/bin`, `--norc`, every route of each row; the expected
// numbers are that shell's. See
// interp.Diagnostics.SubstitutionParseFailureStatusFromAnInteractiveCommandString.
func TestASubstitutionBodyRefusalStatusFollowsTheRoute(t *testing.T) {
	for _, c := range []struct {
		src             string
		interactiveC, c int
		script          int
	}{
		{"echo $(", 1, 2, 2},
		{"echo $(case", 1, 127, 2},
		{"echo $(echo a; fi", 1, 127, 2},
		{"echo <(fi", 1, 127, 2},
		{"echo ${x:-$(fi", 1, 127, 2},
		{"echo $(if", 1, 2, 2},
		{"echo ${", 1, 2, 2},
		{"echo ${|", 1, 2, 2},
		// The controls: what runs out inside the body, or is not a body.
		{"echo $((", 2, 2, 2},
		{"echo ${x", 2, 2, 2},
		{"echo $(echo 'a", 2, 2, 2},
		{"if true; then", 2, 2, 2},
	} {
		t.Run(c.src, func(t *testing.T) {
			home := scratchHome(t)
			t.Chdir(home)
			writeHomeFile(t, home, "s.sh", c.src)
			for _, r := range []struct {
				route string
				argv  []string
				want  int
			}{
				{"-i -c", []string{"bash", "--norc", "-i", "-c", c.src}, c.interactiveC},
				{"-c", []string{"bash", "--norc", "-c", c.src}, c.c},
				{"-i script", []string{"bash", "--norc", "-i", "s.sh"}, c.script},
				{"script", []string{"bash", "--norc", "s.sh"}, c.script},
			} {
				if _, _, got := prompt(t, "", r.argv...); got != r.want {
					t.Errorf("%s: status %d, want %d", r.route, got, r.want)
				}
			}
		})
	}
}

// And a body that closed and would not parse leaves 1 behind under `-i -c`,
// where the same string without `-i` ends the shell at 127, and the next line
// runs as it does for every refusal under `-i` (#6073).
func TestARefusedSubstitutionBodyLeavesOneUnderInteractiveC(t *testing.T) {
	for _, src := range []string{"echo $(fi)", `eval "echo \$(fi)"`, "x=$(fi)", "(echo $(fi))", "echo ${ fi; }"} {
		t.Run(src, func(t *testing.T) {
			home := scratchHome(t)
			t.Chdir(home)
			out, _, _ := prompt(t, "", "bash", "--norc", "-i", "-c", src+"\necho st=$?")
			if !strings.Contains(out, "st=1\n") {
				t.Errorf("-i -c: output %q, want st=1", out)
			}
			if _, _, got := prompt(t, "", "bash", "--norc", "-c", src); got != 127 {
				t.Errorf("-c: status %d, want 127", got)
			}
		})
	}
}
