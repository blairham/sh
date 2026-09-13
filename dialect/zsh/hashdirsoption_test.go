// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `hashdirs` follows **interactive**, and not the terminal.
//
// Measured on zsh 5.9.2 with `-f`, 2026-09-12, reading `${options[hashdirs]}`
// through `zmodload zsh/parameter` and writing the listing by redirection
// rather than through a pipe — a pipeline forks, and a forked zsh has already
// moved two of these (#2122):
//
//	zsh -f script                   off
//	zsh -f -c …                     off
//	zsh -f -i script, no terminal   on
//	zsh -f -i script, under a pty   on
//
// The compiled-in default is on, so what a bare `setopt` shows is the state:
// `nohashdirs` is the `-c` baseline's one line, and an interactive shell does
// not print the name at all. That second row was a constant `off` here, so
// every interactive listing carried a `nohashdirs` real zsh's does not — the
// only name the two listings disagreed about on that route (#2351).
//
// Every test that pinned the listings ran a `-c` shell, where both sides
// agree, which is the blind spot #2122 was filed about and #2283 found for
// `fc -W`: a route nothing swept.
func TestHashdirsIsOnAtAnInteractivePromptAndOffInAScript(t *testing.T) {
	for _, tc := range []struct {
		name        string
		interactive bool
		terminal    bool
		want        string
	}{
		{"an interactive shell hashes directories", true, false, "on\n"},
		{"and a terminal does not change that", true, true, "on\n"},
		{"a script does not", false, false, "off\n"},
		{"nor does a script with a terminal", false, true, "off\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := dialecttest.Base{
				Dir: t.TempDir(), Interactive: tc.interactive, Terminal: tc.terminal,
			}
			out, st, err := preset.Combined(t, base, `[[ -o hashdirs ]] && print -r on || print -r off`)
			if err != nil {
				t.Fatal(err)
			}
			if out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The listing is where the difference shows, and it is what #2351 is about:
// the interactive route carried one extra row.
func TestAnInteractiveListingDoesNotCarryNoHashdirs(t *testing.T) {
	for _, tc := range []struct {
		name        string
		interactive bool
		want        string
	}{
		// `zle` and `interactive` are the interactive shell's own rows.
		// Real zsh under `-f` adds `norcs` to both; this harness does not
		// suppress startup files, so that row is absent from either side and
		// the comparison is the rest of the listing.
		{"interactive", true, "interactive\nzle\n"},
		{"a script", false, "nohashdirs\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := dialecttest.Base{Dir: t.TempDir(), Interactive: tc.interactive}
			out, st, err := preset.Combined(t, base, `setopt`)
			if err != nil {
				t.Fatal(err)
			}
			if out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// It still moves both ways wherever it starts, which is `recordedOver`'s
// bargain and is what real zsh does with the name in either kind of shell.
func TestHashdirsMovesBothWaysFromEitherBase(t *testing.T) {
	const src = `unsetopt hashdirs; s1=$?
[[ -o hashdirs ]] && a=on || a=off
setopt hashdirs; s2=$?
[[ -o hashdirs ]] && b=on || b=off
print -r -- "$s1 $a $s2 $b"`
	for _, interactive := range []bool{true, false} {
		base := dialecttest.Base{Dir: t.TempDir(), Interactive: interactive}
		out, st, err := preset.Combined(t, base, src)
		if err != nil {
			t.Fatal(err)
		}
		if want := "0 off 0 on\n"; out != want || st != 0 {
			t.Errorf("interactive=%v: out %q status %d, want %q at 0", interactive, out, st, want)
		}
	}
}
