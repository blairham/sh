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

// runWithOpenFileLimit runs src in a shell that reports soft as its limit on
// open files, and answers the axis about what to do at that limit the given
// way.
//
// The limit is the hook's answer and never this process's. A test that lowered
// the real one would be changing the state every other test runs in, and the
// question here is what the interpreter does with the number it is told —
// which is the whole reason reading it is a hook at all.
func runWithOpenFileLimit(t *testing.T, src string, soft int64, answer Answer) (errOut string, status int) {
	t.Helper()
	d := syntax.Core()
	d.MultiDigitFdNumber = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	sem := permissive()
	sem.FdNumberBoundedByOpenFileLimit = answer
	r := &Runner{
		Stdout: &o, Stderr: &e, Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: t.TempDir(), Name: "testsh",
		GetRlimit: func(res Resource) (int64, int64, error) {
			if res != ResourceOpenFiles {
				return RlimitInfinity, RlimitInfinity, nil
			}
			return soft, soft, nil
		},
	}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatal(rerr)
	}
	return e.String(), st
}

// A descriptor number at the process's limit on open files is refused where
// the dialect says so, and the number below it is not. There is no ceiling of
// anybody's own here: move the limit and the boundary moves with it, which is
// what says this is the kernel's number rather than one the shell chose.
func TestADescriptorNumberAtTheOpenFileLimitIsRefused(t *testing.T) {
	errOut, st := runWithOpenFileLimit(t, `exec 20>f`, 20, Yes)
	if !strings.Contains(errOut, "20: Bad file descriptor") {
		t.Errorf("stderr = %q, want the number and the reason", errOut)
	}
	if st != 1 {
		t.Errorf("status = %d, want a failed redirection", st)
	}

	errOut, st = runWithOpenFileLimit(t, `exec 19>f`, 20, Yes)
	if errOut != "" || st != 0 {
		t.Errorf("the number below the limit was refused: %q, status %d", errOut, st)
	}

	// And with the limit at eight the same eight is refused, so the boundary
	// followed the limit rather than sitting at twenty.
	if errOut, _ := runWithOpenFileLimit(t, `exec 8>f`, 8, Yes); !strings.Contains(errOut, "8: Bad") {
		t.Errorf("stderr = %q, want the boundary to have moved with the limit", errOut)
	}
}

// It is the number that could not be had and not the file. The open happens
// first and the file is there afterwards, which is what the shells do and what
// a script that goes looking for its log will find.
func TestARefusedDescriptorNumberStillOpensTheFile(t *testing.T) {
	d := syntax.Core()
	d.MultiDigitFdNumber = true
	f, err := syntax.Parse(`exec 20>fresh`, d)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var o, e bytes.Buffer
	sem := permissive()
	sem.FdNumberBoundedByOpenFileLimit = Yes
	r := &Runner{
		Stdout: &o, Stderr: &e, Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: dir, Name: "testsh",
		GetRlimit: func(Resource) (int64, int64, error) { return 20, 20, nil },
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "fresh")); err != nil {
		t.Errorf("the file the refused redirection named is not there: %v", err)
	}
}

// Where the dialect does not look, the number is taken as written and nothing
// is said — which is a different answer rather than a missing check, and is
// what two of the panel do.
func TestADialectThatDoesNotLookTakesTheNumber(t *testing.T) {
	errOut, st := runWithOpenFileLimit(t, `exec 20>f`, 20, No)
	if errOut != "" || st != 0 {
		t.Errorf("stderr = %q, status %d, want the number taken in silence", errOut, st)
	}
}

// And with no answer at all it is refused as any unanswered axis is, rather
// than guessed at in either direction.
func TestAnUnansweredLimitAxisRefusesTheRedirection(t *testing.T) {
	errOut, st := runWithOpenFileLimit(t, `exec 20>f`, 20, Unspecified)
	if !strings.Contains(errOut, "the shells disagree") {
		t.Errorf("stderr = %q, want the unanswered-axis refusal", errOut)
	}
	if st != 2 {
		t.Errorf("status = %d, want the refusal's status", st)
	}
}

// A shell that was handed no limits has none to enforce. A Runner embedded in
// another program takes what it is given, and inventing a bound for one that
// was given nothing would be the borrowing of process state this package
// declines everywhere else.
func TestWithNoLimitsHookNoNumberIsRefused(t *testing.T) {
	d := syntax.Core()
	d.MultiDigitFdNumber = true
	f, err := syntax.Parse(`exec 1000000>f`, d)
	if err != nil {
		t.Fatal(err)
	}
	var o, e bytes.Buffer
	sem := permissive()
	sem.FdNumberBoundedByOpenFileLimit = Yes
	r := &Runner{
		Stdout: &o, Stderr: &e, Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: t.TempDir(), Name: "testsh",
	}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if e.String() != "" || st != 0 {
		t.Errorf("stderr = %q, status %d, want no bound where no limit was offered", e.String(), st)
	}
}

// The number the shell picks for `{name}>f` is checked like any the script
// wrote. Measured: under a limit of six, every shell that has the construct
// fails, because the number it picks is over the limit too — they simply word
// it three different ways. Reachable only under a limit below ten, which is
// where the picking starts.
func TestADescriptorTheShellPicksIsCheckedToo(t *testing.T) {
	errOut, st := runWithOpenFileLimit(t, `exec {v}>f`, 6, Yes)
	if errOut == "" || st == 0 {
		t.Errorf("stderr = %q, status %d, want the picked number refused as well", errOut, st)
	}

	// And with room for it, the pick stands and the name receives it.
	errOut, st = runWithOpenFileLimit(t, `exec {v}>f; echo "v=$v" >&2`, 64, Yes)
	if st != 0 || !strings.Contains(errOut, "v=1") {
		t.Errorf("stderr = %q, status %d, want a descriptor picked from ten up", errOut, st)
	}
}
