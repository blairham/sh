// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// timedDir is a directory with one external command in it, which is what the
// per-command layout needs: a pipeline element that forked nothing writes no
// line at all, in this shell and in the reference alike.
func timedDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"ext", "other"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestTheTimeFormatVariableShapesTheReport — `$TIMEFMT` had the reference's
// value here and nothing read it, so a script setting `TIMEFMT='%*E'` got the
// default layout where the reference gives it its own (#4910).
//
// The figures move between runs, so each row is a pattern over the shape
// rather than a literal — and the shape is the whole of what this is about:
// the bare letter carries its unit, `*` drops it, `m` and `u` change it, and
// a directive outside the set is written out rather than refused.
//
// Measured 2026-09-27 against `/opt/homebrew/bin/zsh` — `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, which `go version -m` calls *not a Go
// executable* where it calls ours `github.com/blairham/sh/cmd/zsh` — script
// files run `-f` under `env -i PATH=/usr/bin:/bin`, `time sleep 0.123456`
// in one run:
//
//	%E  0.13s   %*E  0.134   %mE  134ms   %uE  134015us   %P  0%
func TestTheTimeFormatVariableShapesTheReport(t *testing.T) {
	for _, tc := range []struct{ name, format, want string }{
		// The four spellings of one figure. `%E` is seconds to two places
		// with the unit after it; `*` is a plain decimal to three places
		// and no unit; `m` and `u` are whole units of their own.
		{"the bare letter carries its unit", `%E`, `^\d+\.\d{2}s\n$`},
		{"a star drops it", `%*E`, `^\d+\.\d{3}\n$`},
		{"m is whole milliseconds", `%mE`, `^\d+ms\n$`},
		{"u is whole microseconds", `%uE`, `^\d+us\n$`},
		// The same four letters over the other two figures, so nothing here
		// is passing because one clock was read for every verb.
		{"user and system take the modifiers too", `%U/%*U/%S/%*S`, `^\d+\.\d{2}s/\d+\.\d{3}/\d+\.\d{2}s/\d+\.\d{3}\n$`},
		// The percentage is whole, with the sign after it, and takes no
		// modifier at all.
		{"the percentage is whole", `%P`, `^\d+%\n$`},
		// The command as written, which is the field the default layout
		// already puts first.
		{"the command as written", `[%J]`, `^\[ext\]\n$`},
		// **An unrecognized directive is not a complaint.** The other
		// vocabulary refuses the whole report and names the character;
		// this one writes the `%` and carries on from the character after
		// it — which is why `%*P` comes out whole, the modifier being a
		// modifier only in front of `E`, `U` and `S`.
		{"an unknown letter is literal", `%x %Q`, `^%x %Q\n$`},
		{"a precision is literal here", `%9E %lE`, `^%9E %lE\n$`},
		{"a modifier that led nowhere is literal", `%*J %mP %*P`, `^%\*J %mP %\*P\n$`},
		{"a doubled percent is one", `%%`, `^%\n$`},
		// And a `%` at the very end writes nothing, where the other
		// vocabulary writes a literal `%`.
		{"a trailing percent writes nothing", `trailing %`, `^trailing \n$`},
		// No backslash expansion: the format holds the bytes the
		// assignment gave it.
		{"backslashes are not escapes", `a\nb %*E`, `^a\\nb \d+\.\d{3}\n$`},
		// An empty format is not an unset one: it writes the newline and
		// nothing else.
		{"an empty format is a bare newline", ``, `^\n$`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runZsh(t, timedDir(t), "TIMEFMT='"+tc.format+"'\ntime ext")
			if !regexp.MustCompile(tc.want).MatchString(out) {
				t.Errorf("got %q, want %s", out, tc.want)
			}
		})
	}
}

// TestTheFormatReplacesTheLineAndNotTheLayout — one line per pipeline
// element, which is the same element count the default layout writes.
//
// Measured in the same run: `time sleep 0.02 | cat` writes two lines through
// the format and `time ( sleep 0.02 | cat )` writes one, and `time :` writes
// nothing at all because nothing forked for it.
func TestTheFormatReplacesTheLineAndNotTheLayout(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a line per element", "TIMEFMT='[%J]'\ntime ext | other", "^\\[ext\\]\n\\[other\\]\n$"},
		{"a subshell is one element", "TIMEFMT='[%J]'\ntime ( ext | other )", `^\[\(.*\)\]\n$`},
		{"nothing forked, nothing written", "TIMEFMT='[%J]'\ntime :", `^$`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runZsh(t, timedDir(t), tc.src)
			if !regexp.MustCompile(tc.want).MatchString(out) {
				t.Errorf("got %q, want %s", out, tc.want)
			}
		})
	}
}

// TestTheDefaultValueRendersTheDefaultLayout — the control, and the row that
// says this change moves no output a script did not ask it to move.
//
// `$TIMEFMT` is always set in this shell, so the format path is the one a
// fresh shell takes. Its default value is the layout this shell was already
// writing, byte for byte, which is what makes that safe — and a run with the
// variable left alone has to keep producing it.
func TestTheDefaultValueRendersTheDefaultLayout(t *testing.T) {
	out, _ := runZsh(t, timedDir(t), "time ext")
	want := regexp.MustCompile(`^ext  \d+\.\d{2}s user \d+\.\d{2}s system \d+% cpu \d+\.\d{3} total\n$`)
	if !want.MatchString(out) {
		t.Errorf("got %q, want the default per-command line", out)
	}
	// And a bare `time`, whose two lines are the same line with a label
	// where `%J` goes — measured, `TIMEFMT='[%J]'` before a bare `time`
	// writes `[shell]` and `[children]`.
	out, _ = runZsh(t, t.TempDir(), "TIMEFMT='[%J]'\ntime")
	if out != "[shell]\n[children]\n" {
		t.Errorf("got %q, want the two labels through the format", out)
	}
}
