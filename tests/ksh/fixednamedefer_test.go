// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// After a special parameter, a positional one or a subscript, some characters
// leave the refusal for the run and the rest are refused while reading.
// Measured 2026-10-03 on ksh93u+ with each body in a branch never taken and
// then again where it is reached.
func TestABadSubstitutionAfterAFixedNameWaitsForTheRun(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		body, want string
		status     int
	}{
		{`${?x}`, "reached\nksh: \"${?x}\": bad substitution\n", 1},
		{`${#$w}`, "reached\nksh: \"${#$w}\": bad substitution\n", 1},
		{`${-x}`, "reached\nksh: \"${-x}\": bad substitution\n", 1},
		{`${1(x)}`, "reached\nksh: \"${1(x)}\": bad substitution\n", 1},
		{`${a[1]_}`, "reached\nksh: \"${a[1]_}\": bad substitution\n", 1},
		// Refused while reading, so nothing runs.
		{`${?1}`, "ksh: syntax error at line 1: `1' unexpected\n", 3},
		{`${$$}`, "ksh: syntax error at line 1: `$' unexpected\n", 3},
		{`${?[1]}`, "ksh: syntax error at line 1: `[' unexpected\n", 3},
		{`${a[1](x)}`, "ksh: syntax error at line 1: `(' unexpected\n", 3},
		{`${ab!}`, "ksh: syntax error at line 1: `!' unexpected\n", 3},
	} {
		src := `if false; then echo "` + c.body + `"; fi; echo reached; echo "` + c.body + `"; echo after`
		var out, errs bytes.Buffer
		status := driver.MainArgs(kshShell(&out, &errs), []string{"ksh", "-c", src})
		if got := out.String() + errs.String(); got != c.want || status != c.status {
			t.Errorf("%s: %q at %d, want %q at %d", c.body, out.String()+errs.String(), status, c.want, c.status)
		}
	}
}

// An assignment's value is named with the assignment in front of it, where an
// array literal's element is named alone. Measured 2026-10-03 on ksh93u+.
func TestABadSubstitutionInAnAssignmentNamesTheAssignment(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, want string }{
		{`x=${a@Z}`, "ksh: x=${a@Z}: bad substitution\n"},
		{`x+=pre${?x}`, "ksh: x+=pre${?x}: bad substitution\n"},
		{`x=1 y=${?x} true`, "ksh: y=${?x}: bad substitution\n"},
		{`a=(${?x})`, "ksh: ${?x}: bad substitution\n"},
		{`echo a=${?x}`, "ksh: a=${?x}: bad substitution\n"},
	} {
		var out, errs bytes.Buffer
		driver.MainArgs(kshShell(&out, &errs), []string{"ksh", "-c", c.src})
		if got := out.String() + errs.String(); got != c.want {
			t.Errorf("%s: %q, want %q", c.src, got, c.want)
		}
	}
}
