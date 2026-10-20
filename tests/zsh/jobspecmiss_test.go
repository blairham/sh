// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A job spec that names nothing is worded by its shape, on every job builtin
// alike. Measured 2026-10-01 on zsh 5.9.2 (`-f`, a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, no jobs); each row is what that shell wrote.
func TestAJobSpecThatNamesNothingIsWordedByItsShape(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ src, want string }{
		{"wait %%", "zsh:wait:1: no current job\n127\n"},
		{"wait %+", "zsh:wait:1: no current job\n127\n"},
		{"wait %-", "zsh:wait:1: no previous job\n127\n"},
		{"wait %foo", "zsh:wait:1: job not found: foo\n127\n"},
		{"wait '%?bar'", "zsh:wait:1: job not found: ?bar\n127\n"},
		{"kill %%", "zsh:kill:1: no current job\n1\n"},
		{"kill %-", "zsh:kill:1: no previous job\n1\n"},
		{"kill %foo", "zsh:kill:1: job not found: foo\n1\n"},
		{"disown %%", "zsh:disown:1: no current job\n127\n"},
		{"jobs %foo", "zsh:jobs:1: job not found: foo\n127\n"},
		// The control: a job *number* keeps the builtin's own sentence.
		{"wait %1", "zsh:wait:1: %1: no such job\n127\n"},
		{"kill %1", "zsh:kill:1: %1: no such job\n1\n"},
	} {
		if out, _ := runZsh(t, dir, c.src+" 2>&1\necho $?\n"); out != c.want {
			t.Errorf("%s: got %q, want %q", c.src, out, c.want)
		}
	}
}
