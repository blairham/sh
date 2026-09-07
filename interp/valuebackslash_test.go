// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A backslash that was in a *value* survives the word it is expanded into.
//
// Measured 2026-09-07, `env -i PATH=/usr/bin:/bin` with a scratch HOME,
// ZDOTDIR and HISTFILE, from a script file, against bash 5.3.15, bash 5.3.15
// under an argv[0] of `sh`, bash 3.2.57, dash, ksh93u+ and zsh 5.9.2. All six
// keep the backslash in every row below, and keep both of a doubled pair.
//
// The fields an expansion produces are carried in an escaped form, where a
// backslash in front of a character means "this was quoted" and is taken off
// again once the glob stage has had its look. A backslash *in the value* is
// the one byte that form cannot carry unmarked, and nothing marked it: the
// mark and the data were the same byte, so the unescape read the value's own
// backslash as a mark and removed it. `v='a\b'; w=$v` assigned `ab` at status
// 0 with no diagnostic, and `v='a\\b'` assigned one backslash where the panel
// keeps two (#1222).
//
// Written as a table over the contexts rather than as one case, because the
// loss was in the escaped form itself and therefore in every context that
// builds a field from a value — an assignment, a concatenation, a `case`
// subject, a split word and an array element all reached it, and a single
// assignment case could not have said how wide it was. The direct read is in
// the table as the control: it was **correct** throughout, which is what
// makes this a round trip through a word rather than a fault in the value.
func TestAValueKeepsItsBackslashThroughAWord(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"read directly", `v='a\b'; printf '[%s]' "$v"`, `[a\b]`},
		{"assigned through an expansion", `v='a\b'; w=$v; printf '[%s]' "$w"`, `[a\b]`},
		{"assigned through a quoted expansion", `v='a\b'; w="$v"; printf '[%s]' "$w"`, `[a\b]`},
		{"concatenated with itself", `v='a\b'; u=$v$v; printf '[%s]' "$u"`, `[a\ba\b]`},
		{"a doubled backslash is not halved", `v='a\\b'; w=$v; printf '[%s]' "$w"`, `[a\\b]`},
		{"counted rather than printed", `v='a\b'; w=$v; printf '[%s]' "${#w}"`, `[3]`},
		{"a case subject", `v='a\b'; case $v in 'a\b') printf '[esc]';; ab) printf '[plain]';; *) printf '[none]';; esac`, `[esc]`},
		{"split into fields", `v='a\b'; set -- $v; printf '[%s]' "$@"`, `[a\b]`},
		{"an array element taken one at a time", `a=('x\y' 'p\q'); printf '[%s]' ${a[@]}`, `[x\y][p\q]`},
		{"a command substitution's result", `w=$(printf 'a\\b'); printf '[%s]' "$w"`, `[a\b]`},
		{"a trailing backslash", `v='a\'; w=$v; printf '[%s]' "$w"`, `[a\]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, nil)
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// The character a value's backslash precedes is not a metacharacter, and the
// backslash stays in the text.
//
// The other half of the same marking, and it needs a directory because it is
// only observable against real names. Measured 2026-09-07 in a directory
// holding exactly `a\b` and `a*`: bash 5.3.15, bash-as-`sh`, bash 3.2.57,
// dash and zsh 5.9.2 all leave the word as `a\*`, matching neither name — so
// the `*` behind the backslash is not live and the backslash is still there
// to be printed. Both files are present on purpose: a directory holding
// neither would print `a\*` whatever the rule was, and could not tell the
// readings apart.
//
// ksh93u+ is the one shell that reads it the other way, taking the backslash
// as data and the `*` as live, and it answers `a\b`. That divergence is a
// dialect's to hold and is recorded in the corpus rather than decided here;
// what this pins is that the majority reading is what the escaped form
// produces, and that the backslash is not eaten either way.
func TestAValueBackslashTakesTheMetacharacterOffWhatFollowsIt(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{`a\b`, `a*`} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatalf("write %q: %v", name, err)
		}
	}
	// The control runs in the same directory and against the same two names:
	// an unmarked `*` matches both, so a fix that simply stopped globbing
	// expansion results would pass the row above and fail this one.
	for _, tc := range []struct{ name, src, want string }{
		{"a backslash before the metacharacter", `v='a\*'; set -- $v; printf '[%s]' "$@"`, `[a\*]`},
		{"the metacharacter alone", `v='a*'; set -- $v; printf '[%s]' "$@"`, `[a*][a\b]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, func(r *Runner) { r.Dir = dir })
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}
