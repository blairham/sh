// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

func redirRun(t *testing.T, dir string, dg Diagnostics, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	r := &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Dir: dir, Name: "testsh"}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}

// The four shapes, each written as its dialect writes it. Two of them use a
// verb, two of them do not; two distinguish opening from creating, two do not;
// one puts the reason first.
func TestARedirectFailureIsWordedFourWays(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		diag Diagnostics
		read string
		make string
	}{
		{
			"bash",
			Diagnostics{CannotOpen: "%[1]s: %[2]s", CannotCreate: "%[1]s: %[2]s"},
			"nope: No such file or directory", "d: Is a directory",
		},
		{
			"dash",
			Diagnostics{
				CannotOpen: "cannot open %[1]s: %[2]s", CannotCreate: "cannot create %[1]s: %[2]s",
				FileNotFound: "No such file", DirectoryNotFound: "Directory nonexistent",
			},
			"cannot open nope: No such file", "cannot create d: Is a directory",
		},
		{
			"ksh93",
			Diagnostics{CannotOpen: "%[1]s: cannot open [%[2]s]", CannotCreate: "%[1]s: cannot create [%[2]s]"},
			"nope: cannot open [No such file or directory]", "d: cannot create [Is a directory]",
		},
		{
			"zsh",
			Diagnostics{CannotOpen: "%[2]s: %[1]s", CannotCreate: "%[2]s: %[1]s", LowercaseReason: true},
			"no such file or directory: nope", "is a directory: d",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, _ := redirRun(t, dir, c.diag, `cat < nope`); !strings.Contains(out, c.read) {
				t.Errorf("read: said %q, want %q", out, c.read)
			}
			if out, _ := redirRun(t, dir, c.diag, `echo x > d`); !strings.Contains(out, c.make) {
				t.Errorf("create: said %q, want %q", out, c.make)
			}
		})
	}
}

// dash names the same errno two ways depending on which direction the file was
// being opened, where the operating system names it once.
func TestDashNamesOneErrnoTwoWays(t *testing.T) {
	dir := t.TempDir()
	dg := Diagnostics{
		CannotOpen: "cannot open %[1]s: %[2]s", CannotCreate: "cannot create %[1]s: %[2]s",
		FileNotFound: "No such file", DirectoryNotFound: "Directory nonexistent",
	}
	// Exactly, not as a prefix: "No such file" is the front of the operating
	// system's own "No such file or directory", so a Contains check here
	// passes whether dash's text is used or not.
	if out, _ := redirRun(t, dir, dg, `cat < nodir/in`); !strings.HasSuffix(strings.TrimSpace(out), "cannot open nodir/in: No such file") {
		t.Errorf("read: said %q, want dash's own open text and nothing more", out)
	}
	if out, _ := redirRun(t, dir, dg, `echo x > nodir/out`); !strings.HasSuffix(strings.TrimSpace(out), "cannot create nodir/out: Directory nonexistent") {
		t.Errorf("create: said %q, want dash's own create text and nothing more", out)
	}
	// Without either, the operating system's own string, capitalized — which
	// is what the other three print and what Go's errno does not give.
	plain := Diagnostics{CannotOpen: "%[1]s: %[2]s", CannotCreate: "%[1]s: %[2]s"}
	if out, _ := redirRun(t, dir, plain, `cat < nodir/in`); !strings.Contains(out, "No such file or directory") {
		t.Errorf("read: said %q, want the OS string capitalized", out)
	}
}

// The status, which is a separate question from the wording: dash says 2 where
// the other three say 1, and does not stop.
func TestARedirectFailureStatus(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name string
		diag Diagnostics
		want int
	}{
		{"the substrate's own", Diagnostics{}, 1},
		{"dash's", Diagnostics{RedirectFailureStatus: 2}, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := redirRun(t, dir, c.diag, "cat < nope\necho st=$?\necho after")
			if !strings.Contains(out, "st="+string(rune('0'+c.want))) {
				t.Errorf("said %q, want st=%d", out, c.want)
			}
			// And the script carries on either way — the failure is not fatal
			// in any of the four.
			if !strings.Contains(out, "after") {
				t.Errorf("said %q, want the script to carry on", out)
			}
		})
	}
}

// A redirect is set up before the command runs, so one that cannot be opened
// means the command does not run at all.
func TestAFailedRedirectDoesNotRunTheCommand(t *testing.T) {
	dir := t.TempDir()
	out, _ := redirRun(t, dir, Diagnostics{CannotOpen: "%[1]s: %[2]s"}, `echo reached < nope`)
	if strings.Contains(out, "reached") {
		t.Errorf("said %q, want the command not to have run", out)
	}
}
