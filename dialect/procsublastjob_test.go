// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import "testing"

// Whether a process substitution sets `$!`, and so whether `wait "$!"` waits
// for its body and reports its status.
//
// Measured 2026-09-16 from a script file per shape, `env -i PATH=/usr/bin:/bin
// LC_ALL=C <shell> case.sh` with stdin from /dev/null in a fresh directory:
// Homebrew bash 5.3.20 and zsh 5.9.2, AT&T ksh93u+ 2012-08-01. Standard output
// only; zsh's complaint about waiting on pid 0 goes to standard error.
func TestAProcessSubstitutionIsTheLastBackgroundJob(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want map[string]string
	}{
		{
			name: "a reading substitution's status comes back through wait",
			src:  "cat <(exit 3) >/dev/null; wait $!; echo \"s=$?\"\n",
			want: map[string]string{"bash": "s=3\n", "zsh": "s=127\n", "ksh": "s=0\n"},
		},
		{
			name: "and waiting really waits for a body that is still running",
			src:  "cat <(sleep 0.1; exit 4) >/dev/null; wait $!; echo \"s=$?\"\n",
			want: map[string]string{"bash": "s=4\n", "zsh": "s=127\n", "ksh": "s=0\n"},
		},
		{
			name: "a substitution moves $! off the job before it",
			src:  "sleep 0 & p=$!; cat <(:) >/dev/null; [ \"$p\" = \"$!\" ] && echo same || echo moved\n",
			want: map[string]string{"bash": "moved\n", "zsh": "same\n", "ksh": "same\n"},
		},
		{
			// The body is not a job in any other sense: nothing lists it.
			name: "jobs does not list the body",
			src:  "cat <(:) >/dev/null; wait $!; jobs; echo end\n",
			want: map[string]string{"bash": "end\n", "zsh": "end\n", "ksh": "end\n"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for preset, want := range c.want {
				t.Run(preset, func(t *testing.T) {
					out, _, _ := splitRun(t, presets[preset], c.src)
					if out != want {
						t.Errorf("wrote %q, want %q", out, want)
					}
				})
			}
		})
	}
}
