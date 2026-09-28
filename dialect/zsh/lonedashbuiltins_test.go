// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A lone `-` ends a builtin's option scan, in the builtins whose option
// readers are their own.
//
// `Semantics.LoneDashIsAnOption` reached the shared reader and `echo` (#5026)
// and reached none of these, so each of them went on reading options past the
// dash — or, worse, read the dash itself as the thing it was scanning for.
//
// **The discriminating shape is an option word *behind* the dash.** "Eaten and
// skipped" and "eaten and the options end" agree wherever nothing but operands
// follows it, which is every row the axis was first measured from; that
// agreement is what hid six of these for as long as it did.
//
// Measured 2026-09-28 against `/opt/homebrew/bin/zsh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` *not a Go executable*, from
// script files under `env -i PATH=/usr/bin:/bin` with a scratch `HOME`. Every
// row below is that run, and every one of them was re-run against this
// dialect's own binary through the same driver.
//
// `umask` is measured with the rest and graded in interp rather than here —
// this harness gives the runner no umask seam, so `umask` answers "this shell
// was not given a umask to read or change" whatever the dash does. See
// TestALoneDashEndsTheUmaskOptions.
func TestALoneDashEndsTheOptionsOfABuiltinWithItsOwnReader(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      []string // substrings the output must hold
		notWant   []string // and ones it must not
		status    int
	}{
		{
			// Two complaints and not three: the dash is eaten, `-f` and `x`
			// are both table elements that are not there.
			name: "enable names what is behind the dash and not the dash",
			src:  `enable - -f x`,
			want: []string{"no such hash table element: -f", "no such hash table element: x"},
			// The row that was wrong: the dash named as an element of its own.
			notWant: []string{"no such hash table element: -\n"},
			status:  1,
		},
		{
			name:    "whence takes the name behind the dash",
			src:     `whence - echo`,
			want:    []string{"echo"},
			notWant: []string{"not found"},
			status:  0,
		},
		{
			// `setopt -x` is taken, so a single complaint about `-x` is the
			// dash having ended the options and `-x` having become a *name*.
			name:    "setopt reads what is behind the dash as a name",
			src:     `setopt - -x`,
			want:    []string{"no such option: -x"},
			notWant: []string{"no such option: -\n"},
			status:  1,
		},
		{
			// **And it does not become the bare command.** Measured: `setopt`
			// alone lists and `setopt -` is silent, so eating a word is not
			// the same as never having been given one.
			name:    "but setopt with the dash eaten is silent rather than a listing",
			src:     `setopt -`,
			notWant: []string{"nohashdirs"},
			status:  0,
		},
		{
			name:    "and so is unsetopt",
			src:     `unsetopt -`,
			notWant: []string{"aliases"},
			status:  0,
		},
		{
			// The count is eaten with the word, leaving `shift` its bare form.
			name:   "shift with the dash eaten shifts one",
			src:    `set -- a b c; shift -; print -r -- "rest=$*"`,
			want:   []string{"rest=b c"},
			status: 0,
		},
		{
			// And what follows really is the count, not an operand a marker
			// would have protected.
			name:   "and a count behind the dash is still the count",
			src:    `set -- a b c; shift - 2; print -r -- "rest=$*"`,
			want:   []string{"rest=c"},
			status: 0,
		},
		{
			name:    "type names what is behind the dash and not the dash",
			src:     `type - -a echo`,
			want:    []string{"-a not found", "echo is a shell builtin"},
			notWant: []string{"- not found\n"},
			status:  1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshOnPath(t, t.TempDir(), tc.src)
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("out = %q, want it to contain %q", out, w)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(out, w) {
					t.Errorf("out = %q, want it NOT to contain %q", out, w)
				}
			}
			if st != tc.status {
				t.Errorf("status = %d, want %d", st, tc.status)
			}
		})
	}
}

// The controls: the same letters **without** a dash in front of them are read
// exactly as they were, which is what says the change is about the dash and
// not about the letters.
//
// Two of these do not agree with the reference and are recorded that way
// rather than quietly fixed: `enable -f` is not implemented here at all, and
// `setopt -x` is refused here where the reference takes it. Both are older
// than #5040 and neither moved.
func TestTheOptionLettersBehindNoDashAreUnmoved(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{"a declaration letter still declares", `typeset -r cx=1; print -r -- "st=$?"`, "st=0", 0},
		{"type still reports the builtin", `type -a echo`, "echo is a shell builtin", 0},
		{"whence still writes the sentence", `whence -v echo`, "echo is a shell builtin", 0},
		{"shift alone still shifts one", `set -- a b c; shift; print -r -- "rest=$*"`, "rest=b c", 0},
		{"setopt still takes an option name", `setopt xtrace; print -r -- ok`, "ok", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshOnPath(t, t.TempDir(), tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("out = %q, want it to contain %q", out, tc.want)
			}
			if st != tc.status {
				t.Errorf("status = %d, want %d", st, tc.status)
			}
		})
	}
}
