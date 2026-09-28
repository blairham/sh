// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `fc -p` takes a history size and a save count after the file, and refuses a
// word that is not an integer — **before anything is pushed**.
//
// That order is the measurement rather than a tidiness: a refused `fc -p`
// leaves the list, `$HISTSIZE` and `$SAVEHIST` exactly as they were. Before
// this the words were stored unread, so `a` became a `HISTSIZE` of *one*, the
// push went ahead, the list emptied and the status was 0.
//
// Measured 2026-09-28 against `/opt/homebrew/bin/zsh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` *not a Go executable*, from
// script files under `env -i PATH=/usr/bin:/bin` with a scratch `HOME`.
//
// **Every row asserts the *contents* of `${(v)history}` and never
// `${#history}`.** The harness and a real `zsh -f` disagree about the
// baseline count, so a row that counts is measuring the harness rather than
// the shell — which is the trap this whole family has to be written around.
func TestFcPushRefusesASizeThatIsNotAnInteger(t *testing.T) {
	seed := "HISTFILE=$PWD/h; SAVEHIST=100; HISTSIZE=100\n" +
		"print -l one two three > $PWD/h\nfc -R $PWD/h\n"
	for _, tc := range []struct {
		name, args, want string
		status           int
	}{
		// Each word names its own parameter, which is what says this is two
		// checks and not one.
		{"a bad history size", `/dev/null a 0`, "HISTSIZE must be an integer", 1},
		{"a bad save count", `/dev/null 0 a`, "SAVEHIST must be an integer", 1},
		// The first bad word decides, so both being bad names only the first.
		{"and the first bad word decides", `/dev/null a b`, "HISTSIZE must be an integer", 1},
		// What counts as an integer is the decimal reading and nothing wider.
		// The last two are the rows that say it is neither "starts like a
		// number" nor a shell arithmetic evaluation.
		{"a plain pair is taken", `/dev/null 10 20`, "", 0},
		{"a negative is taken", `/dev/null -1 0`, "", 0},
		{"leading zeros are taken", `/dev/null 010 0`, "", 0},
		{"a fraction is not", `/dev/null 1.5 0`, "HISTSIZE must be an integer", 1},
		{"hexadecimal is not", `/dev/null 0x10 0`, "HISTSIZE must be an integer", 1},
		{"and neither is an exponent", `/dev/null 1e2 0`, "HISTSIZE must be an integer", 1},
		// Empty is taken, which is why the check is not simply "parses".
		{"an empty word is taken", `/dev/null '' 0`, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := seed + "fc -p " + tc.args + "\nprint -r -- \"st=$?\"\n"
			out, _, err := preset.CombinedWithPrelude(t, dialecttest.Base{}, src)
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if strings.Contains(out, "must be an integer") {
					t.Errorf("out = %q, want the sizes taken", out)
				}
			} else if !strings.Contains(out, tc.want) {
				t.Errorf("out = %q, want it to contain %q", out, tc.want)
			}
			if !strings.Contains(out, "st="+string(rune('0'+tc.status))) {
				t.Errorf("out = %q, want status %d", out, tc.status)
			}
		})
	}
	// **Nothing moves on a refusal**, which is the half a status-only row
	// cannot see: the list is still there and both sizes are where they were.
	t.Run("and a refusal leaves the session alone", func(t *testing.T) {
		src := seed +
			`print -r -- "before=[${(v)history}] H=$HISTSIZE S=$SAVEHIST"` + "\n" +
			"fc -p /dev/null a 0\n" +
			`print -r -- "after=[${(v)history}] H=$HISTSIZE S=$SAVEHIST"` + "\n"
		out, _, err := preset.CombinedWithPrelude(t, dialecttest.Base{}, src)
		if err != nil {
			t.Fatal(err)
		}
		before, after := "", ""
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "before=") {
				before = strings.TrimPrefix(line, "before=")
			}
			if strings.HasPrefix(line, "after=") {
				after = strings.TrimPrefix(line, "after=")
			}
		}
		if before == "" || before != after {
			t.Errorf("before %q and after %q, want the refusal to move nothing", before, after)
		}
	})
}
