// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A duplicate is a second *name* for one open file, so closing one name ends
// the name and not the file. The file ends when the last name goes.
//
// These run their standard output into a real file rather than the string
// builder the rest of this package uses, and that is the whole reason they
// catch anything: a strings.Builder is not an io.Closer, so the close that
// was wrong here was a no-op against it and every existing test passed while
// `exec {s}>&1; exec {s}>&-` was leaving a real shell with no stdout at all
// (#2127).
func TestClosingADuplicateLeavesTheOriginalOpen(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			// The shape a prompt theme runs on every start: save the stream,
			// put something else there, restore, then drop the save.
			"a save is dropped after it is restored",
			`exec {s}>&1
exec {s}>&-
echo alive`,
			"alive\n",
		},
		{
			"two saves, only one dropped",
			`exec {a}>&1
exec {b}>&1
exec {a}>&-
echo via-b >&$b
echo alive`,
			"via-b\nalive\n",
		},
		{
			// The same rule with neither name being a standard stream: `y` is
			// a duplicate of `x`, so closing `x` must leave `y` writing to the
			// file they share.
			"a duplicate of a file outlives the name it came from",
			`exec {x}>f
exec {y}>&$x
exec {x}>&-
echo kept >&$y
exec {y}>&-
cat f`,
			"kept\n",
		},
		{
			// And the half that says this is not "never close": the last name
			// for a file really does end it, which is what lets a reader see
			// end-of-file rather than waiting for the shell to exit.
			"the last name for a file still ends it",
			`exec {x}>f
echo once >&$x
exec {x}>&-
cat f`,
			"once\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runWithFileStdout(t, tc.src); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// runWithFileStdout runs src with standard output pointed at a real file, and
// returns what the file holds afterwards.
func runWithFileStdout(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	sem := PosixSemantics()
	sem.RedirectErrorOnSpecialBuiltinFatal = No
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
		Stdout: out, Stderr: &strings.Builder{},
		Vars: map[string]string{"PATH": lookBinPath(t)},
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
