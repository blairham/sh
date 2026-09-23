// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
)

// A `~` with no home stays the character it was written as, which is the
// standard's reading: the tilde prefix is replaced by the value of `HOME`,
// and with no value there is nothing to replace it with.
//
// Measured 2026-09-23 on dash 0.5.12, `env -i PATH=/usr/bin:/bin LC_ALL=C
// dash -c`, with and without a `HOME` to unset — the two agree:
//
//	printf '<%s>' ~ ~/x    <~> <~/x>
//	v=a:~:b                <a:~:b>
//
// This is the column the shell already matched, and it is pinned because the
// other two answers now exist: bash reads a password entry and zsh reads an
// unset home as an empty one, and either arriving here by a flipped default
// would turn `~` into a path on a shell that has no business producing one.
// See interp.TildeWithNoHomePolicy.
func TestATildeWithNoHomeStaysWritten(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a bare tilde", "HOME=/h\nunset HOME\nprintf '<%s>' ~", "<~>"},
		{"with a tail", "HOME=/h\nunset HOME\nprintf '<%s>' ~/x", "<~/x>"},
		{
			"after a colon in an assignment",
			"HOME=/h\nunset HOME\nv=a:~:b\nprintf '<%s>' \"$v\"",
			"<a:~:b>",
		},
		{
			"a home that is there",
			"HOME=/h\nprintf '<%s>' ~/x",
			"</h/x>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// And the preset says so, inherited from the base rather than set here: the
// standard's reading is this shell's, so there is nothing to override.
func TestTheWrittenWordIsThisDialectsNoHomeAnswer(t *testing.T) {
	if got := dash.Semantics().TildeWithNoHome; got != interp.TildeWithNoHomeStaysWritten {
		t.Errorf("TildeWithNoHome = %v, want stays written", got)
	}
}
