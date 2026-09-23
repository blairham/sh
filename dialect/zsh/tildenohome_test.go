// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// An unset `HOME` answers a `~` the way an empty one does here, rather than
// leaving the word as written.
//
// Measured 2026-09-23 on zsh 5.9.2, `env -i HOME=/seeded PATH=/usr/bin:/bin
// LC_ALL=C zsh -c`, with `unset HOME` first — which really does unset it
// here, `${HOME-UNSET}` being `UNSET` afterwards:
//
//	printf '<%s>' ~ ~/x    <> </x>
//	v=a:~:b                <a::b>
//
// dash leaves both words as written and bash reads a password entry, so the
// three columns give three different answers to the same line; see
// interp.TildeWithNoHomePolicy.
//
// The startup row is a different question and is left alone: this shell seeds
// `HOME` from the password entry before any line runs, so `env -i zsh -c
// 'echo ~'` has a home to read where bash has none. That is about what a
// shell puts in its environment, not about what a tilde does without one.
func TestAnUnsetHomeReadsAsAnEmptyOne(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a bare tilde", "HOME=/h\nunset HOME\nprintf '<%s>' ~", "<>"},
		{"with a tail", "HOME=/h\nunset HOME\nprintf '<%s>' ~/x", "</x>"},
		{
			"after a colon in an assignment",
			"HOME=/h\nunset HOME\nv=a:~:b\nprintf '<%s>' \"$v\"",
			"<a::b>",
		},
		{
			// The control: with a home to read it is read, so the rows
			// above are about the home being gone rather than about the
			// tilde being dropped.
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

// And the preset says so.
func TestTheEmptyStringIsThisDialectsNoHomeAnswer(t *testing.T) {
	if got := zsh.Semantics().TildeWithNoHome; got != interp.TildeWithNoHomeIsEmpty {
		t.Errorf("TildeWithNoHome = %v, want is empty", got)
	}
}
