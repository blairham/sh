// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"slices"
	"testing"
)

// **Homebrew's ncurses database is read before the system's** — the order the
// reference reads them in, its library being that keg's (#5291). Measured
// 2026-10-01 under `env -i`: zsh 5.9.2's `${#terminfo}` for `TERM=screen` is
// the keg's 144 rather than the system's 136, and `TERM=1178`, which only the
// system database holds, is still found — so the system list follows. The
// environment's own directories still come first.
func TestTheKegDatabaseIsReadBeforeTheSystems(t *testing.T) {
	dirs := terminfoDirectories(func(string) string { return "" })
	keg := slices.Index(dirs, "/opt/homebrew/opt/ncurses/share/terminfo")
	system := slices.Index(dirs, "/usr/share/terminfo")
	if keg < 0 || system < 0 || keg > system {
		t.Errorf("search order %q: want the keg's database ahead of /usr/share/terminfo", dirs)
	}
	env := map[string]string{"TERMINFO": "/mine", "TERMINFO_DIRS": "/also"}
	dirs = terminfoDirectories(func(k string) string { return env[k] })
	if len(dirs) < 2 || dirs[0] != "/mine" || dirs[1] != "/also" {
		t.Errorf("with TERMINFO and TERMINFO_DIRS set: %q, want those first", dirs)
	}
}
