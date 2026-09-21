// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `. file ARG`, and the half a one-line probe does not see.
//
// DotPassesArguments was unset here, so this dialect took the POSIX preset's
// `No` — dash's answer, and dash is the panel's only holdout: bash 5.3, that
// binary as `sh`, bash 3.2, zsh 5.9.2, ksh93u+ and BusyBox ash all give the
// sourced file its own parameters. Nothing graded it. axes_test.go asserts
// that an *unanswered* axis refuses rather than that an answered one is
// right, and the suite had no `.`-with-words row at all until this change
// added one (#3248).
//
// Measured 2026-09-16 against BusyBox v1.37.0 in the digest-pinned Alpine
// image internal/oracle reaches, under `env -i PATH=/usr/bin:/bin LC_ALL=C`.
//
// The two rows are the discriminator and its control, and they part on both
// halves of the axis. A shell that ignores the words writes the caller's
// parameters at the first site; a shell that passes them but forgets to
// restore writes the sourced file's at the second. Only "passes and
// restores" gives the want below, and it is BusyBox's answer.
func TestDotGivesTheSourcedFileItsOwnParameters(t *testing.T) {
	for _, tc := range []struct{ name, body, call, want string }{
		{
			// The discriminator. dash's answer here is
			// `inside:[outer1][outer2][3]`.
			"words after the filename",
			`echo "inside:[$1][$2][$#]"`,
			`. ./sub.sh arg`,
			"inside:[arg][][1]\nafter:[outer1][outer2][3]\n",
		},
		{
			// The control every column agrees on: with no words, the
			// caller's parameters are what the sourced file sees. It is what
			// says the row above is about the *words* and not about `.`
			// resetting the list.
			"no words after the filename",
			`echo "inside:[$1][$2][$#]"`,
			`. ./sub.sh`,
			"inside:[outer1][outer2][3]\nafter:[outer1][outer2][3]\n",
		},
		{
			// `$0` is not one of the words: the sourced file is not a
			// script's argv, so the shell's own name stands throughout.
			// Asserted because a runner that built a fresh argv for the file
			// would move it, and the rows above could not tell.
			"the shell name is left alone",
			`echo "inside0:[$0]"`,
			`. ./sub.sh arg`,
			"inside0:[ash]\nafter:[outer1][outer2][3]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "sub.sh"), []byte(tc.body+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			src := "set -- outer1 outer2 outer3\n" + tc.call + "\n" +
				`echo "after:[$1][$2][$#]"` + "\n"
			out, st, err := preset.Combined(t, dialecttest.Base{
				Name: "ash", Dir: dir, Env: []string{"PATH=/usr/bin:/bin"},
			}, src)
			if err != nil {
				t.Fatalf("unsupported: %v", err)
			}
			if st != 0 {
				t.Errorf("status %d, want 0; output %q", st, out)
			}
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// TestASetInASourcedFileIsRestoredOverInBusyBox: the words after the
// filename are put back over whatever the sourced file did to them, `set`
// included — BusyBox ash is on zsh's and ksh93's side of the split and not
// on bash's.
//
// Measured 2026-09-21 against BusyBox v1.37.0 in the digest-pinned Alpine
// image internal/oracle reaches, under `env -i PATH=/usr/bin:/bin LC_ALL=C`,
// with `set -- a b c; . ./g p q; echo "$@"`:
//
//	set -- m n o p in the file    a b c   — bash 5.3.20 and 3.2.57: m n o p
//	shift in the file             a b c   — the control, unanimous
//
// Two bodies rather than one, because the second is what says the first is
// about a *replacement* being let through: a shell that had stopped
// restoring at all would answer the file's list on both.
func TestASetInASourcedFileIsRestoredOverInBusyBox(t *testing.T) {
	for _, body := range []string{"set -- m n o p", "set --", "shift"} {
		t.Run(body, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "g.sh"), []byte(body+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			src := `set -- a b c; . ./g.sh p q; echo "after=[$@]"` + "\n"
			out, st, err := preset.Combined(t, dialecttest.Base{
				Name: "ash", Dir: dir, Env: []string{"PATH=/usr/bin:/bin"},
			}, src)
			if err != nil {
				t.Fatalf("unsupported: %v", err)
			}
			if st != 0 {
				t.Errorf("status %d, want 0; output %q", st, out)
			}
			if out != "after=[a b c]\n" {
				t.Errorf("got %q, want the caller's parameters back", out)
			}
		})
	}
}
