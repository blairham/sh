// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **A handler for a signal the shell sent itself sees `kill`'s own status**
// in bash, dash and ksh93, and the status from before the statement in zsh —
// the corpus case trap/signal-handler-status-diverges, recorded against bash
// 5.3.20, dash, ksh93u+ and zsh 5.9.2.
//
// The handler runs before `kill` returns (#5359), and running it there showed
// every column the `false` before it until the status was set first.
func TestASelfSentSignalsHandlerSeesKillsStatus(t *testing.T) {
	src := "trap 'echo st=$?' INT\nfalse\nkill -INT $$\necho after\n"
	for name, want := range map[string]string{
		"bash": "st=0\nafter\n",
		"dash": "st=0\nafter\n",
		"ksh":  "st=0\nafter\n",
		"zsh":  "st=1\nafter\n",
	} {
		t.Run(name, func(t *testing.T) {
			out, _, err := presets[name].Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
			if err != nil {
				t.Fatal(err)
			}
			if out != want {
				t.Errorf("got %q, want %q", out, want)
			}
		})
	}
}
