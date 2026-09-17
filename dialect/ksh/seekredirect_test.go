// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// The file-position redirections, which this shell alone has (#3034).
//
// syntax and interp prove what the flag and the operators do; this file pins
// that this preset turns the flag on and words the three refusals its own
// way. Measured 2026-09-16 on ksh93u+ 2012-08-01, as script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin on /dev/null and a fresh
// directory.

func seekRun(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Name: "ksh", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

func TestTheSeekRedirectionsAreThisShells(t *testing.T) {
	const src = `printf abcdefghij > f
exec 3< f
read -n4 v <&3; printf '[%s]' "$v"
exec 3<#((0))
read -n2 v <&3; printf '[%s]' "$v"
n=2
exec 3<#((n * 3 + 1))
read -n2 v <&3; printf '[%s]' "$v"
exec 3<&-
printf 0123456789 > g
exec 4<> g
exec 4>#((3))
printf XY >&4
exec 4>&-
printf '[%s]\n' "$(cat g)"`
	out, st := seekRun(t, src)
	const want = "[abcd][ab][hi][012XY56789]\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// The three refusals, each in this shell's words. A failed redirection on
// `exec` ends the script and one on an ordinary command does not, which is
// the answer this shell already gives every other redirection.
func TestASeekThisShellWillNotMakeIsWordedItsOwnWay(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			// Not the sentence a duplication writes about a number nothing
			// is open at, which is `7: cannot open [Bad file descriptor]`.
			name: "a descriptor nothing is open at", src: "exec 6<#((0))\necho after",
			want: "6: bad file unit number [Bad file descriptor]\n", status: 1,
		},
		{
			name: "and the same on an ordinary command", src: "true 6<#((0))\necho after",
			want: "6: bad file unit number [Bad file descriptor]\nafter\n", status: 0,
		},
		{
			name: "an offset before the start",
			src:  "printf abcdefghij > f\nexec 3< f\nexec 3<#((-1))\necho after",
			want: "-1: invalid seek offset\n", status: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := seekRun(t, tc.src)
			if !strings.HasSuffix(out, tc.want) || st != tc.status {
				t.Errorf("got %q (status %d), want it to end with %q at %d",
					out, st, tc.want, tc.status)
			}
		})
	}
}
