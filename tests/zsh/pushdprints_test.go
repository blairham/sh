// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// TestPushdAndPopdPrintTheStackWhenInteractive is #5982: an interactive shell
// writes the stack after `pushd` and `popd` move it, the rotations and a
// `popd +N` that leaves the shell where it is included, unless `pushdsilent`
// is set or the call said `-q`. Measured 2026-10-05 against zsh 5.9.2 run as
// `zsh -f -i` with the lines on standard input; under `-c` neither shell
// writes anything.
func TestPushdAndPopdPrintTheStackWhenInteractive(t *testing.T) {
	const pre = "HOME=/nonexistent; cd /\n"
	for _, tc := range []struct {
		name, src, want string
		interactive     bool
	}{
		{"pushd", "pushd /tmp\npushd /usr", "/tmp /\n/usr /tmp /\n", true},
		{"a rotation", "pushd /tmp\npushd /usr\npushd +1", "/tmp /\n/usr /tmp /\n/tmp / /usr\n", true},
		{"a swap", "pushd /usr\npushd", "/usr /\n/ /usr\n", true},
		{"popd", "pushd /tmp\npushd /usr\npopd", "/tmp /\n/usr /tmp /\n/tmp /\n", true},
		{"popd +N, which does not move", "pushd /tmp\npushd /usr\npopd +1", "/tmp /\n/usr /tmp /\n/usr /\n", true},
		{"from inside a function", "f() { pushd /tmp; }; f", "/tmp /\n", true},
		{"quiet", "pushd -q /tmp\npopd -q", "", true},
		{"pushdsilent", "setopt pushdsilent\npushd /tmp\npopd", "", true},
		// The location in front of the sentence is this harness's; what the row
		// holds is that nothing follows it.
		{"a failure writes only its refusal", "pushd +9", "zsh:pushd:2: no such entry in dir stack\n", true},
		{"autopushd's cd is not pushd", "setopt autopushd\ncd /tmp", "", true},
		{"not interactive", "pushd /tmp\npushd /usr\npopd", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := preset.CombinedWithPrelude(t, dialecttest.Base{
				Dir: t.TempDir(), Interactive: tc.interactive,
			}, pre+tc.src+"\n")
			if err != nil {
				t.Fatal(err)
			}
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
