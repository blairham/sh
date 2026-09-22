// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `#!` line naming a program that is not there — #4159.
//
// The kernel refuses the start with ENOENT and the file itself is perfectly
// present, so a shell that reported the errno alone would say "No such file or
// directory" about a file it had just found. Two of the four look inside and
// name what they could not find; two report a command that was not there.
//
// Measured 2026-09-22 on a file whose first line is `#!nosuchfile`, run from a
// script: bash writes `<$0>: ./x: nosuchfile: bad interpreter: No such file or
// directory` and zsh `<$0>:<line>: ./x: bad interpreter: nosuchfile: no such
// file or directory`, where ksh93 and dash both write `./x: not found`.
//
// Before this the line read `./x: fork/exec /abs/path/x: no such file or
// directory` — Go's own wrapper, with a path nobody wrote and a sentence no
// shell prints.
func TestAShebangNamingNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(path, []byte("#!nosuchfile\necho hi\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	out, status := run(t, path+"\n", func(r *Runner) {
		r.Diagnostics = &Diagnostics{
			BadInterpreter:                   "%[1]s: %[2]s: bad interpreter: %[3]s",
			BadInterpreterLocatedByNameAlone: true,
			Location:                         LocationLineWord,
		}
	})
	// `sh: ` is the shell's own name, which is what a runner with no script
	// file to name reports under.
	want := "sh: " + path + ": nosuchfile: bad interpreter: No such file or directory\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if status != 126 {
		t.Errorf("status %d, want 126", status)
	}
}

// The location is dropped for this one message and for no other, which is what
// says it is a rule about the message rather than about the route: measured,
// the same script reports a command that is not there as `./s.sh: line 2:
// ./nosuch: No such file or directory` and this one with no line at all.
func TestTheBadInterpreterLineIsLocatedByNameAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(path, []byte("#!nosuchfile\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	out, _ := run(t, path+"\n", func(r *Runner) {
		r.Diagnostics = &Diagnostics{
			BadInterpreter: "%[1]s: %[2]s: bad interpreter: %[3]s",
			Location:       LocationLineWord,
		}
	})
	if !strings.Contains(out, "line 1:") {
		t.Errorf("got %q, want the dialect that keeps its location to keep it", out)
	}
}

// A dialect with nothing to say about it reports the start failure it already
// had, which is what the two columns that do not look inside the file do.
func TestADialectThatNamesNoBadInterpreterWordingSaysWhatItSaid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(path, []byte("#!nosuchfile\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	out, _ := run(t, path+"\n", nil)
	if strings.Contains(out, "bad interpreter") {
		t.Errorf("got %q, want no wording this dialect does not have", out)
	}
}
