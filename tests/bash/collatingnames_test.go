// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A collating element written as a **name** from the portable character set,
// which is this column alone in the panel.
//
// Measured 2026-09-18 on bash 5.3.20, script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`: every one of these rows answers the
// same way there, and ksh93u+ and dash 0.5.12 read the same bodies as bodies
// that are not elements — so `[[.hyphen.]]` matches no `-` in either (#3378).
//
// The roster is measured a name at a time rather than derived from the
// standard's charmap, which the `low-line` row is the evidence for: the
// character it names is reached under `underscore` and the shell does not
// know the other spelling.
func TestACollatingElementMayBeWrittenAsAName(t *testing.T) {
	for _, tc := range []struct {
		pattern, subject string
		want             string
		why              string
	}{
		{"[[.hyphen.]]", "-", "Y", "a name under the collating delimiter"},
		{"[[=hyphen=]]", "-", "Y", "and under the equivalence one"},
		{"[[.period.]]", ".", "Y", "a name whose character is a delimiter"},
		{"[[=space=]]", " ", "Y", "and one that is not printable as itself"},
		{"[[.hyphen.]]", "h", "n", "no letter of the name is a member"},
		{"[[.hyphen.]q]", "q", "Y", "a name is one member of a wider set"},
		{"[[.zero.]-[.nine.]]", "5", "Y", "and a bound at either end of a range"},
		{"[[.zero.]-[.nine.]]", "a", "n", "which is a range and not a set of two"},
		{"[[.underscore.]]", "_", "Y", "the spelling this shell has"},
		{"[[.low-line.]]", "_", "n", "and not the one it has not"},
		{"[[.nosuch.]]", "n", "n", "a body outside the roster is not an element"},
		{"[[.a.]]", "a", "Y", "and one character is the element it spells"},
	} {
		src := `case "` + tc.subject + `" in ` + tc.pattern + `) printf Y;; *) printf n;; esac` + "\n"
		out, status := runBash(t, t.TempDir(), src)
		if status != 0 {
			t.Fatalf("%s vs %q: status %d, out %q", tc.pattern, tc.subject, status, out)
		}
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s vs %q = %q, want %q — %s",
				tc.pattern, tc.subject, out, tc.want, tc.why)
		}
	}
}
