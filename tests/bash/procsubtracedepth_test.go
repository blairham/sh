// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A process substitution's body is traced one level deeper than the command
// that names it, the level a command substitution's body is given (#6736).
//
// Measured 2026-10-16 on bash 5.3.20, `-c` and from a file: the body is text
// read again, which is what the repeated first character of the prefix counts,
// so `<( … )` adds a level wherever it is written — on the script's own line,
// inside a `$( … )`, inside another `<( … )`, and inside an `eval`. A function
// call does not add one, which is the control. Most rows read the body with
// `source`, which waits for it, so their lines arrive in the same order on
// every run of the reference (a body beside a command that does not read it
// can arrive on either side of that command's own line there), and `source`
// adds a level of its own to what it reads.
func TestAProcessSubstitutionBodyIsTracedOneLevelDeeper(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			name: "on the script's own line",
			src:  "set -x; source <(echo echo 1)",
			want: "+ source /dev/fd/63\n++ echo echo 1\n++ echo 1\n1\n",
		},
		{
			name: "inside a command substitution",
			src:  "set -x; echo $(source <(echo echo 1))",
			want: "++ source /dev/fd/63\n+++ echo echo 1\n+++ echo 1\n+ echo 1\n1\n",
		},
		{
			name: "after another command in the command substitution",
			src:  "set -x; : $(true; : <(true))",
			want: "++ true\n++ : /dev/fd/63\n+++ true\n+ :\n",
		},
		{
			name: "inside another process substitution",
			src:  "set -x; source <(source <(echo echo echo 1))",
			want: "+ source /dev/fd/63\n++ source /dev/fd/63\n+++ echo echo echo 1\n+++ echo echo 1\n++ echo 1\n1\n",
		},
		{
			name: "inside an eval",
			src:  `set -x; eval "source <(echo echo 1)"`,
			want: "+ eval 'source <(echo echo 1)'\n++ source /dev/fd/63\n+++ echo echo 1\n+++ echo 1\n1\n",
		},
		{
			// The control: a function call is not text read again.
			name: "inside a function",
			src:  "set -x; f(){ source <(echo echo 1); }; f",
			want: "+ f\n+ source /dev/fd/63\n++ echo echo 1\n++ echo 1\n1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runBash(t, t.TempDir(), tc.src)
			if out != tc.want {
				t.Errorf("out %q\nwant %q", out, tc.want)
			}
		})
	}
}
