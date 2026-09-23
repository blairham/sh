// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// A written `~` with no home to read is the password entry here, which is the
// answer the cached home made reachable.
//
// This shell left the word as written, and that was recorded as deliberate in
// Semantics.TildeReadsACachedHome's own doc — "this package carries no
// password database, so a `~` whose cached home is absent is left as
// written". One column's answer taken for the panel's: measured 2026-09-23 on
// bash 5.3.20 and 3.2.57, which agree, `env -i PATH=/usr/bin:/bin LC_ALL=C`:
//
//	printf '<%s>' ~ ~/x       </Users/bh><\/Users/bh/x>   with $HOME empty
//	v=a:~:b                   <a:/Users/bh:b>
//
// The route that needs the cache is the reason this is asserted here rather
// than only against the axis: `export -n HOME` takes the name out of the
// environment, so the *copy* a child's environment refreshes becomes absent
// while the variable is still there to read — and a shell that answered `~`
// from the variable would never reach the question at all. That is the case
// #4304 made reachable, and it was a second wrong answer rather than a fixed
// one until this (#4179).
func TestATildeWithNoHomeReadsThePasswordEntry(t *testing.T) {
	// The database is a stub. A test that read the real one passes on a
	// laptop and fails on a runner with no such user, which is why
	// internal/testenv exists.
	homeless := func(t *testing.T, src string) (string, int) {
		t.Helper()
		var buf strings.Builder
		r := preset.Runner(dialecttest.Base{
			Name: "sh", Stdout: &buf, Stderr: &buf,
			// No HOME anywhere: not in the environment, so `export -n`
			// has nothing to leave behind either.
			Env: []string{"PATH=/usr/bin:/bin"},
		})
		r.UserHomeDir = func(name string) (string, bool) {
			if name == "" {
				return "/from/the/database", true
			}
			return "", false
		}
		st, err := r.Run(context.Background(), preset.Parse(t, src))
		if err != nil {
			t.Fatalf("run %q: %v", src, err)
		}
		return buf.String(), st
	}
	for _, tc := range []struct{ name, src, want string }{
		{"a bare tilde", `printf '<%s>' ~`, "</from/the/database>"},
		{"with a tail", `printf '<%s>' ~/x`, "</from/the/database/x>"},
		{
			"after a colon in an assignment",
			`v=a:~:b; printf '<%s>' "$v"`,
			"<a:/from/the/database:b>",
		},
		{
			// The route the cache opens. The assignment gives the shell a
			// home, the `export -n` takes it back out of the environment,
			// and the command substitution builds a child's environment —
			// which is what refreshes the copy, to absent.
			"a home the environment no longer carries",
			`HOME=/h; export -n HOME; : $(:); printf '<%s>' ~`,
			"</from/the/database>",
		},
		{
			// The control for it: with HOME still exported the copy is
			// refreshed to the assigned home and the database is never
			// reached, so the row above is about the copy being absent
			// rather than about a fallback that fires on every line.
			"a home the environment still carries",
			`HOME=/h; export HOME; : $(:); printf '<%s>' ~`,
			"</h>",
		},
		{
			// A `HOME=` that is set and empty is not this question — it is
			// simply empty — but on this column it has to reach the *copy*
			// to be read at all, so the export and the child are part of
			// the row rather than decoration.
			"a home that is set and empty, and exported",
			`export HOME=; : $(:); printf '<%s>' ~/x`,
			"</x>",
		},
		{
			// And the same assignment without the export leaves the copy
			// absent, so the database answers. Measured 2026-09-23 on
			// 5.3.20, which prints the password entry's home here and `/x`
			// on the line above; bash 3.2.57 keeps no copy and prints `/x`
			// on both. Two columns of the same shell, and the difference is
			// the cache rather than this axis.
			"a home that is set and empty and stays a shell variable",
			`HOME=; printf '<%s>' ~/x`,
			"</from/the/database/x>",
		},
		{
			// A name in the script is the other question, and it keeps the
			// answer it had: the stub has no such user, so the word stands.
			"a named tilde",
			`printf '<%s>' ~nosuchuser/x`,
			"<~nosuchuser/x>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := homeless(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// And the preset says so, pinned so that a vector edit cannot quietly put
// this column back on the standard's reading.
func TestThePasswordEntryIsThisDialectsNoHomeAnswer(t *testing.T) {
	if got := bash.Semantics().TildeWithNoHome; got != interp.TildeWithNoHomeReadsThePasswordEntry {
		t.Errorf("TildeWithNoHome = %v, want reads the password entry", got)
	}
}
