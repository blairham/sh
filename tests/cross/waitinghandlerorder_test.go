// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// Three trapped signals arriving while one command runs, in the order USR2,
// HUP, USR1, and the order the dialect then runs their handlers in —
// interp.WaitingHandlerOrder (#7039).
//
// The signals come from a pipeline element, which every shell in the panel
// runs as a child of its own, so all three land while the shell waits for the
// pipeline and are waiting together when it ends. `( … )` would do for four
// columns and not for ksh93, which runs a subshell in its own process and is
// ended by the first signal there. `/bin/cat` rather than `:` as the last
// element for the same shell, which does not wait for the first element
// behind a builtin and has exited by the time it sends.
//
// Measured 2026-10-31 with this script under `-c`, three runs each:
//
//	bash 5.3.20, bash 3.2.57, dash   HUP USR1 USR2 after
//	BusyBox ash 1.38.0               HUP USR1 USR2 after (busybox:latest)
//	ksh93u+ 2012-08-01               USR2 USR1 HUP after
//	zsh 5.9.2                        USR2 HUP USR1 after, and once USR2 USR1 HUP
//
// zsh's one odd run is three signals sent back to back; sent a tenth of a
// second apart from another process, it runs them in the order they came in
// every time, which is its answer. HUP < USR1 < USR2 on darwin and on Linux
// alike, so the expectations hold on either.
const waitingHandlersProbe = "for s in HUP USR1 USR2; do trap \"echo $s\" $s; done\n" +
	"{ kill -USR2 $$; kill -HUP $$; kill -USR1 $$; } | /bin/cat\n" +
	"echo after\n"

func TestSeveralWaitingHandlersRunInEachDialectsOrder(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"bash", "HUP\nUSR1\nUSR2\nafter\n"},
		{"dash", "HUP\nUSR1\nUSR2\nafter\n"},
		{"ash", "HUP\nUSR1\nUSR2\nafter\n"},
		{"ksh", "USR2\nUSR1\nHUP\nafter\n"},
		{"zsh", "USR2\nHUP\nUSR1\nafter\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, err := presets[tc.name].Combined(t, dialecttest.Base{Dir: t.TempDir()}, waitingHandlersProbe)
			if err != nil {
				t.Fatal(err)
			}
			if out != tc.want || st != 0 {
				t.Errorf("got %q at %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
