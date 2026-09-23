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

// A written `~` with no home to read at all is the password entry here.
//
// This shell left the word as written, which was one column's answer taken
// for the panel's: measured 2026-09-23 on bash 5.3.20 and 3.2.57, which
// agree, `env -i PATH=/usr/bin:/bin LC_ALL=C`:
//
//	printf '<%s>' ~ ~/x       </Users/bh><\/Users/bh/x>   with $HOME empty
//	v=a:~:b                   <a:/Users/bh:b>
//
// **No home means the name is not there.** The last two rows below used to
// reach this answer by a second route — `export -n HOME` left bash 5.3.20's
// cached copy absent while the variable was still readable — and that route
// is gone with the cache, which this dialect no longer reproduces. See
// dialect/bash/tildecachedeclined_test.go for the table and the reason. Both
// rows are kept, measured against bash 3.2.57, whose reading is the one taken
// here: they are the controls that say a set `HOME` is read whatever the
// environment carries (#4179, #4304).
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
			// A set `HOME` is read however the environment is arranged
			// around it: the `export -n` takes the name out of a child's
			// block and the command substitution builds one, and neither
			// touches what `~` reads. Measured 2026-09-23 on bash 3.2.57,
			// `</h>`; 5.3.20 answers the password entry here, and that
			// difference is its cache rather than this axis.
			"a home the environment no longer carries",
			`HOME=/h; export -n HOME; : $(:); printf '<%s>' ~`,
			"</h>",
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
			// simply empty — and the export and the child are kept because
			// they used to be what made it reach the copy at all.
			"a home that is set and empty, and exported",
			`export HOME=; : $(:); printf '<%s>' ~/x`,
			"</x>",
		},
		{
			// The same assignment without the export, which reads the same:
			// empty is a home, and an unexported one is still the home.
			// Measured 2026-09-23 on bash 3.2.57, `</x>` for both; 5.3.20
			// answers the password entry here because its copy went absent,
			// which is the cache and not this axis.
			"a home that is set and empty and stays a shell variable",
			`HOME=; printf '<%s>' ~/x`,
			"</x>",
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
