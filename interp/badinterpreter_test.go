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

// A dialect with nothing to say about it says what it says about a name that
// was not found, which is what the columns that do not look inside the file
// do — the wording *and* the 127, where an unstartable file is otherwise 126.
//
// It was Go's `fork/exec <abs path>: no such file or directory` at 126 here,
// which is a sentence no shell writes, about a file that is plainly there,
// numbered so that a script testing `$? -eq 127` for "not found" is told 126
// (#4454).
func TestADialectThatNamesNoBadInterpreterWordingSaysNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(path, []byte("#!nosuchfile\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	out, status := run(t, path+"\n", func(r *Runner) {
		r.Diagnostics = &Diagnostics{PathNotFound: "%[1]s: not found"}
	})
	if strings.Contains(out, "bad interpreter") {
		t.Errorf("got %q, want no wording this dialect does not have", out)
	}
	if strings.Contains(out, "fork/exec") {
		t.Errorf("got %q, want no Go error in a shell diagnostic", out)
	}
	if want := path + ": not found\n"; !strings.HasSuffix(out, want) {
		t.Errorf("got %q, want it to end in %q", out, want)
	}
	if status != 127 {
		t.Errorf("status %d, want 127", status)
	}
}
