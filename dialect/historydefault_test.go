// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/repl"
)

// Which file each dialect records history in when `HISTFILE` is unset.
//
// One table rather than a row in each dialect's own tests, because the answer
// is only meaningful as a set: the substrate used to give every dialect ksh's
// `.sh_history`, and a per-dialect assertion would have passed for ksh and
// been absent for the other four. **Two of the five have no file at all**,
// which is the value a shared default cannot express.
//
// Measured 2026-09-30 with an empty home directory and no rc file, each shell
// interactive on a pipe — see repl.HistoryStyle.DefaultFile for the panel and
// for the separate question of whether the shell publishes the path in
// `$HISTFILE`, which bash and ash do and ksh93 does not.
func TestEachDialectsDefaultHistoryFile(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		style repl.HistoryStyle
		want  string
	}{
		{"zsh records nothing without HISTFILE", zsh.HistoryStyle(), ""},
		{"dash records nothing without HISTFILE", dash.HistoryStyle(), ""},
		{"bash", bash.HistoryStyle(), ".bash_history"},
		{"ksh", ksh.HistoryStyle(), ".sh_history"},
		{"ash", ash.HistoryStyle(), ".ash_history"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.style.DefaultFile; got != c.want {
				t.Errorf("default history file is %q, want %q", got, c.want)
			}
		})
	}
}
