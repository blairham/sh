// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// What `${v:=word}` *yields* is the value the variable ended up holding, not
// the word it was handed.
//
// The two part company wherever the name carries an attribute that rewrites
// what is stored, which is why this lives here: `declare -i`, `-u` and `-l`
// are this column's. Both halves were already right — the variable held the
// converted value — while the expansion handed back the word, so a line that
// assigned and read in one breath disagreed with the very next line that read
// the name.
//
// Measured 2026-09-22 against bash 5.3.20, and it is two lines of that
// shell's own `exp13.sub`.
func TestAnAssignmentInsideAnExpansionYieldsWhatTheAttributeStored(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an integer attribute evaluates the word",
			`declare -i a; echo ${a:=4+3}; declare -p a`,
			"7\ndeclare -i a=\"7\"\n",
		},
		{
			"an uppercase attribute rewrites it",
			`declare -u A; A=; echo ${A:=foo}; declare -p A`,
			"FOO\ndeclare -u A=\"FOO\"\n",
		},
		{
			"a lowercase attribute rewrites it",
			`declare -l L; L=; echo ${L:=FOO}; declare -p L`,
			"foo\ndeclare -l L=\"foo\"\n",
		},
		// With no attribute in play the word is the answer, which is the row
		// that says nothing is read back that should not be.
		{
			"and a plain name yields the word",
			`unset v; echo ${v:=foo}; declare -p v`,
			"foo\ndeclare -- v=\"foo\"\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			code := driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-c", tc.src})
			if code != 0 || errs.Len() != 0 {
				t.Fatalf("%s: status = %d, stderr = %q", tc.src, code, errs.String())
			}
			if out.String() != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, out.String(), tc.want)
			}
		})
	}
}
