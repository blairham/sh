// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"io"
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
			"name and reason",
			Diagnostics{CannotOpen: "%[1]s: %[2]s", CannotCreate: "%[1]s: %[2]s"},
			"nope: No such file or directory", "d: Is a directory",
		},
		{
			"a verb and its own errno names",
			Diagnostics{
				CannotOpen: "cannot open %[1]s: %[2]s", CannotCreate: "cannot create %[1]s: %[2]s",
				FileNotFound: "No such file", DirectoryNotFound: "Directory nonexistent",
			},
			"cannot open nope: No such file", "cannot create d: Is a directory",
		},
		{
			"the reason bracketed after a verb",
			Diagnostics{CannotOpen: "%[1]s: cannot open [%[2]s]", CannotCreate: "%[1]s: cannot create [%[2]s]"},
			"nope: cannot open [No such file or directory]", "d: cannot create [Is a directory]",
		},
		{
			"the reason first and lowercased",
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

// oneWayRun runs src with streams that go one way only, so that a descriptor
// duplicated onto the other one fails. Everything the runner writes lands in
// the returned text.
func oneWayRun(t *testing.T, dir string, dg Diagnostics, src string) string {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	r := &Runner{
		Stdin:     readerOnly{strings.NewReader("")},
		Stdout:    writerOnly{&buf},
		Stderr:    writerOnly{&buf},
		Semantics: &sem, Diagnostics: &dg, Dir: dir, Name: "testsh",
	}
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String()
}

type readerOnly struct{ r io.Reader }

func (o readerOnly) Read(p []byte) (int, error) { return o.r.Read(p) }

type writerOnly struct{ w io.Writer }

func (o writerOnly) Write(p []byte) (int, error) { return o.w.Write(p) }

// A duplication that names a descriptor nothing opened reports an errno like
// any other redirection failure, so LowercaseReason reaches it too.
//
// It did not: the text was written out lowercase at the three places that
// raise it, which is one dialect's spelling of it and nobody else's. The
// substrate capitalizes what the C string capitalizes.
func TestADupOfAnUnopenedDescriptorQuotesTheReasonTheDialectsWay(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name string
		diag Diagnostics
		want string
	}{
		{"capitalized by default", Diagnostics{}, "6: Bad file descriptor"},
		{"lowercased where the axis says so", Diagnostics{LowercaseReason: true}, "6: bad file descriptor"},
	} {
		t.Run(c.name, func(t *testing.T) {
			// A number nothing ever opened, reached as output and as input.
			for _, src := range []string{`echo hi >&6`, `cat <&6`} {
				out, _ := redirRun(t, dir, c.diag, src)
				if !strings.Contains(out, c.want) {
					t.Errorf("%s: said %q, want %q", src, out, c.want)
				}
			}
			// And a number that is open the other way. An embedder's streams
			// need not go both ways — a *bytes.Buffer happens to, which is
			// why this builds its own rather than using the shared runner —
			// so a descriptor parked on one of them can be duplicated onto a
			// stream it cannot serve. Two more places the reason is quoted.
			for _, src := range []string{`exec 6>&1; read v <&6`, `exec 6<&0; echo x >&6`} {
				if out := oneWayRun(t, dir, c.diag, src); !strings.Contains(out, c.want) {
					t.Errorf("%s: said %q, want %q", src, out, c.want)
				}
			}
		})
	}
}

// FileNotFound and DirectoryNotFound name the same errno two ways depending on
// which direction the file was being opened, where the operating system names
// it once — one measured dialect wants exactly that.
func TestADialectCanNameOneErrnoTwoWays(t *testing.T) {
	dir := t.TempDir()
	dg := Diagnostics{
		CannotOpen: "cannot open %[1]s: %[2]s", CannotCreate: "cannot create %[1]s: %[2]s",
		FileNotFound: "No such file", DirectoryNotFound: "Directory nonexistent",
	}
	// Exactly, not as a prefix: "No such file" is the front of the operating
	// system's own "No such file or directory", so a Contains check here
	// passes whether the field's text is used or not.
	if out, _ := redirRun(t, dir, dg, `cat < nodir/in`); !strings.HasSuffix(strings.TrimSpace(out), "cannot open nodir/in: No such file") {
		t.Errorf("read: said %q, want the field's own open text and nothing more", out)
	}
	if out, _ := redirRun(t, dir, dg, `echo x > nodir/out`); !strings.HasSuffix(strings.TrimSpace(out), "cannot create nodir/out: Directory nonexistent") {
		t.Errorf("create: said %q, want the field's own create text and nothing more", out)
	}
	// Without either, the operating system's own string, capitalized — which
	// is what the fields' absence means and what Go's errno does not give.
	plain := Diagnostics{CannotOpen: "%[1]s: %[2]s", CannotCreate: "%[1]s: %[2]s"}
	if out, _ := redirRun(t, dir, plain, `cat < nodir/in`); !strings.Contains(out, "No such file or directory") {
		t.Errorf("read: said %q, want the OS string capitalized", out)
	}
}

// The status, which is a separate question from the wording:
// RedirectFailureStatus carries a dialect's own number over the default 1,
// and the failure does not stop the script either way.
func TestARedirectFailureStatus(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name string
		diag Diagnostics
		want int
	}{
		{"the substrate's own", Diagnostics{}, 1},
		{"a dialect's own number", Diagnostics{RedirectFailureStatus: 2}, 2},
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
