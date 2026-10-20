// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/internal/terminfofixture"
)

// Slots in the string array the rows below need, by terminfo(5)'s order.
const (
	stringBell = 1  // bel
	stringIs3  = 50 // is3
)

// setupFixture is a description whose two readings differ: `is3` is stored,
// so the stored reading lists `is3` and the converted one `OTi2`.
func setupFixture(t *testing.T) (dir string) {
	t.Helper()
	return terminfofixture.Database(t,
		terminfofixture.Description{
			Name: fixtureTerm, StrCount: stringIs3 + 1,
			Strs: map[int]string{
				stringBell: "\a", stringCursorUp: "\x1b[A", stringIs3: "\x1bA",
			},
		},
		// A second terminal for the rows that move `$TERM`, with nothing
		// either reading would rename.
		terminfofixture.Description{
			Name: "otherterm", StrCount: stringCursorUp + 1,
			Strs: map[int]string{stringBell: "\a", stringCursorUp: "\x1b[A"},
		},
	)
}

// The reading a script is handed, at each point the state machine in
// terminalsetup.go can move. Every row is zsh 5.9.2 under `env -i` on
// 2026-10-02, with `TERM=d217-unix` — a real description that stores `is3` —
// in place of the fixture; the same rows run against this shell's binary
// over the same database agree on all of them.
//
// `e` writes S where the stored reading is enumerated, C where the converted
// one is, so each row's string is the sequence of readings it saw.
func TestTheReadingFollowsHowTheTerminalWasSetUp(t *testing.T) {
	const e = `e() { local k; for k in ${(k)terminfo}; do
  [[ $k == is3 ]] && { print -n S; return; }
  [[ $k == OTi2 ]] && { print -n C; return; }
done; print -n N; }
`
	for _, tc := range []struct {
		name        string
		interactive bool
		src, want   string
	}{
		{"a listing sets nothing up", false, `e; e`, "SS"},
		{"a lookup converts", false, `e; : $terminfo[bel]; e`, "SC"},
		{"the lookup itself is converted", false, `e; print -n ${+terminfo[is3]}${+terminfo[OTi2]}; e`, "S01C"},
		{
			"a prompt first freezes the stored reading", false,
			`print -P %B >/dev/null; e; print -n ${+terminfo[is3]}; e`, "S1S",
		},
		{"the termcap half follows", false, `print -n ${+terminfo[OTi2]}${+termcap[i2]}${+termcap[i3]}; e`, "110C"},
		{"and interactively too", true, `print -n ${+terminfo[OTi2]}${+termcap[i2]}${+termcap[i3]}; e`, "001S"},
		{"a prompt after a load converts", false, `zmodload zsh/terminfo; print -P %B >/dev/null; e`, "C"},
		{"either module counts", false, `zmodload zsh/termcap; print -P %B >/dev/null; e`, "C"},
		{"echoti", false, `echoti bel >/dev/null; e`, "C"},
		{"echotc", false, `echotc bl >/dev/null; e`, "C"},
		{"a termcap lookup", false, `: ${termcap[bl]}; e`, "C"},
		{"a termcap listing", false, `: ${(k)termcap}; e`, "S"},
		{"a pattern subscript", false, `: ${terminfo[(I)b*]}; e`, "S"},
		{"a subshell keeps its own", false, `(: $terminfo[bel]); x=$(: $terminfo[bel]); e`, "S"},
		{"a subshell's prompt is its own", false, `(print -P %B >/dev/null); : $terminfo[bel]; e`, "C"},
		{"set up for this TERM", false, `: $terminfo[bel]; TERM=otherterm; TERM=` + fixtureTerm + `; e`, "C"},
		{"set up for another TERM", false, `TERM=otherterm; : $terminfo[bel]; TERM=` + fixtureTerm + `; e; : $terminfo[bel]; e`, "SC"},
		{"interactive starts set up", true, `e; print -n ${+terminfo[is3]}; e`, "S1S"},
		{"interactive assignment converts", true, `: $terminfo[bel]; TERM=$TERM; e`, "C"},
		{"interactive after a load", true, `zmodload zsh/terminfo; TERM=otherterm; TERM=` + fixtureTerm + `; e`, "C"},
		{"interactive with nothing reached", true, `TERM=otherterm; TERM=` + fixtureTerm + `; e`, "S"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupFixture(t)
			out, _, err := preset.Combined(t, dialecttest.Base{
				Dir: t.TempDir(), Interactive: tc.interactive,
				Vars: map[string]string{
					"PATH": t.TempDir(), "TERM": fixtureTerm, "TERMINFO": dir,
					"HOME": t.TempDir(), "TERMINFO_DIRS": "", "HISTFILE": "/dev/null",
				},
			}, e+tc.src)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if out != tc.want {
				t.Errorf("readings = %q, want %q", out, tc.want)
			}
		})
	}
}
