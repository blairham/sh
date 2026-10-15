// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestAnFdVariableDuplicationOfAClosedDescriptorSaysSoFirst — a `{name}`
// redirection that duplicates a descriptor that is not open writes an
// unlocated `redirection error: cannot duplicate fd` line before the usual
// one, where a numbered duplication writes only the usual one (#6572). The
// rows are bash 5.3.20's, measured 2026-10-15; the first line goes where
// standard error stands when the redirection is reached.
func TestAnFdVariableDuplicationOfAClosedDescriptorSaysSoFirst(t *testing.T) {
	const first = "bash: redirection error: cannot duplicate fd: Bad file descriptor\n"
	for _, c := range []struct{ name, src, errs string }{
		{"a write", "true {v}>&3; echo \"st=$?\"\n", first + "bash: line 1: 3: Bad file descriptor\n"},
		{"a read", "true {v}<&3; echo \"st=$?\"\n", first + "bash: line 1: 3: Bad file descriptor\n"},
		{"a move", "true {v}<&3-; echo \"st=$?\"\n", first + "bash: line 1: 3: Bad file descriptor\n"},
		{"on exec", "exec {v}>&3; echo \"st=$?\"\n", first + "bash: line 1: 3: Bad file descriptor\n"},
		{"a closed named stream", "exec 1>&-; true {v}>&1; echo \"st=$?\" >&2\n", first + "bash: line 1: 1: Bad file descriptor\nst=1\n"},
		{"on its own line", "echo a\ntrue {v}>&3; echo \"st=$?\"\n", first + "bash: line 2: 3: Bad file descriptor\n"},
		{"where standard error stands", "true 2>/dev/null {v}>&3; echo \"st=$?\"\n", ""},
		{"a numbered duplication says one line", "true 4>&3; echo \"st=$?\"\n", "bash: line 1: 3: Bad file descriptor\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs := runBashSplit(t, c.src)
			want := "st=1\n"
			if c.name == "on its own line" {
				want = "a\n" + want
			}
			if c.name == "a closed named stream" {
				want = ""
			}
			if out != want || errs != c.errs {
				t.Errorf("out %q err %q, want %q and %q", out, errs, want, c.errs)
			}
		})
	}
}
