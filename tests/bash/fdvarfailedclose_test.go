// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A close through a variable that names nothing open is **silent** here, which
// is the other answer to the axis one column of the panel says yes to.
//
// Measured 2026-09-29: `v=77; exec {v}<&-` writes nothing in bash 5.3.20 and
// nothing in ksh93u+, where zsh 5.9.2 writes `failed to close file descriptor
// 77: bad file descriptor`. See interp.Semantics.FdVariableFailedCloseIsReported.
//
// The row exists so that the column which says nothing is graded rather than
// assumed: a change that made the diagnostic the core's would pass zsh's suite
// and this is what would notice.
func TestAFailedCloseThroughAVariableIsSilent(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a number nothing was opened at", "v=77\nexec {v}<&-\necho \"st $?\"\n"},
		{"a descriptor closed twice", "exec {v}<&0\nexec {v}<&-\nexec {v}<&-\necho \"st $?\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs := runBashSplit(t, tc.src)
			if out != "st 0\n" || errs != "" {
				t.Errorf("out %q err %q, want %q and no diagnostic", out, errs, "st 0\n")
			}
		})
	}
}
