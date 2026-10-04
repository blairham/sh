// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A bad substitution is placed where the reader stood, and not where the
// command began: the line after the one the command was read to when a
// newline ended it, and the last line where the input ran out. A function's
// is placed at its call's, and a substitution body's at the command holding
// it.
//
// Measured 2026-10-04 on BusyBox ash 1.37.0 in the pinned Alpine image over
// script files. The `nosuch` row is the control: the same line places `not
// found` at the command and this report one below it (#5723). See
// Diagnostics.BadSubstitutionIsLocatedAtTheReader.
func TestABadSubstitutionIsPlacedWhereTheReaderStood(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the line after", "echo one\necho ${a[1]}\necho two\n", "s.sh: line 3: syntax error: bad substitution"},
		{"the last line at the end", "echo ${a[1]}", "s.sh: line 1: syntax error: bad substitution"},
		{"after a compound", "if true; then\n  echo ${a[1]}\nfi\necho after\n", "s.sh: line 4: syntax error: bad substitution"},
		{"a function at its call", "f() { echo ${a[1]}; }\necho mid\nf\n", "s.sh: line 4: syntax error: bad substitution"},
		{"a substitution body", "x=$(echo ${a[1]})\n:\n", "s.sh: line 2: syntax error: bad substitution"},
		{"beside a command placed at itself", ":\nnosuch; echo ${a[1]}\n", "s.sh: line 2: nosuch: not found\ns.sh: line 3: syntax error: bad substitution"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "s.sh")
			if err := os.WriteFile(path, []byte(tc.src), 0o600); err != nil {
				t.Fatal(err)
			}
			var o, e bytes.Buffer
			sh := shell()
			sh.SystemStartupDirectory = t.TempDir()
			sh.Stdout, sh.Stderr = &o, &e
			driver.MainArgs(sh, []string{"ash", path})
			got := strings.ReplaceAll(e.String(), dir+"/", "")
			if !strings.Contains(got, tc.want) {
				t.Errorf("stderr = %q, want %q in it", got, tc.want)
			}
		})
	}
}

// An unterminated `${x` whose name ran into a newline is a line earlier than
// the end of the input, as in dash. Measured 2026-10-04 on BusyBox ash 1.37.0
// in the pinned Alpine image: `echo ${x⏎echo after⏎` is `line 2: syntax
// error: missing '}'`, `echo ${x⏎⏎⏎` is `line 3`, and with no newline at all
// it is line 1 (#5723). See Diagnostics.UnmatchedBraceSubstDropsTheNameNewline.
func TestAnUnterminatedExpansionDropsTheNamesNewline(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a line after it", "echo ${x\necho after\n", "s.sh: line 2: syntax error: missing '}'"},
		{"blank lines after it", "echo ${x\n\n\n", "s.sh: line 3: syntax error: missing '}'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "s.sh")
			if err := os.WriteFile(path, []byte(tc.src), 0o600); err != nil {
				t.Fatal(err)
			}
			var o, e bytes.Buffer
			sh := shell()
			sh.SystemStartupDirectory = t.TempDir()
			sh.Stdout, sh.Stderr = &o, &e
			driver.MainArgs(sh, []string{"ash", path})
			got := strings.ReplaceAll(e.String(), dir+"/", "")
			if !strings.Contains(got, tc.want) {
				t.Errorf("stderr = %q, want %q in it", got, tc.want)
			}
		})
	}
}
