// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"
)

// Turning brace expansion off really stops it, under both spellings (#1856).
//
// Measured on bash 5.3.15, 2026-09-11: `set +B; echo {a,b}` writes `{a,b}`,
// `set +o braceexpand` does the same, `set -B` afterwards puts the expansion
// back, `$-` drops the letter while it is off and `[[ -o braceexpand ]]`
// answers 1. Ours refused `+B` as not implemented and expanded the braces
// anyway — the refusal and the behaviour disagreeing about the same request.
func TestBraceExpansionCanBeTurnedOff(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{"the letter", "set +B; echo {a,b}\n", "{a,b}\n"},
		{"the long name", "set +o braceexpand; echo {a,b}\n", "{a,b}\n"},
		{"and back again", "set +B; set -B; echo {a,b}\n", "a b\n"},
		// Not one-way, unlike `noexec`: the request is a switch and the
		// second half of it has to work or the first is a trap.
		{"and back again by the long name", "set +o braceexpand\nset -o braceexpand\necho {a,b}\n", "a b\n"},
		{"the letter leaves `$-`", "set +B; echo $-", "h\n"},
		{"and comes back with it", "set +B; set -B; echo $-", "hB\n"},
		{"the condition follows", "set +B; [[ -o braceexpand ]]; echo \"st=$?\"\n", "st=1\n"},
		{
			// Through the re-inputtable listing, which writes the state as
			// the command that would restore it — so a capture sourced back
			// into a shell carries the option off rather than on.
			"and so does the listing",
			"set +B\nrows=$(set +o)\ncase $rows in (*\"set +o braceexpand\"*) echo listed=off ;; (*) echo listed=on ;; esac\n",
			"listed=off\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runBash(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("%s gave %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}
