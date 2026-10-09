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

// Whether `cd` takes a `..` out of the path it is building without looking at
// the component it cancels — Semantics.CdCancelsADotDot, three readings across
// six columns, and #4627 and #4668 are the same mechanism read from two ends.
//
// The rows name an axis and never a shell; which dialect holds which is
// asserted in the dialect packages.

// canceledTree is cdphysicaldotdot_test.go's tree — `real/deep` and a
// `sub/fake` pointing at `../real` — with the three components this file's
// rows cancel added to it: a directory to arrive at, an ordinary file, and a
// dangling symbolic link.
//
// Shared rather than built again, because the two files are asking about the
// same `..` from opposite sides and a second fixture is how the `-P` rows and
// the `-L` rows would come to be measured in different trees.
func canceledTree(t *testing.T) string {
	t.Helper()
	root := dotDotTree(t)
	if err := os.Mkdir(filepath.Join(root, "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "afile"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("nowhere", filepath.Join(root, "dangling")); err != nil {
		t.Fatal(err)
	}
	return root
}

// cdDotDot runs one `cd` in that tree under one reading of the axis and hands
// back the status and where the shell ended up.
func cdDotDot(t *testing.T, root string, p CdDotDotCancellationPolicy, src string) (status int, dir, errs string) {
	t.Helper()
	sem := PosixSemantics()
	sem.CdCancelsADotDot = p
	out := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Dir: root, Stderr: out,
	})
	runPart(t, r, src+"\n")
	return r.ExitStatus(), r.Dir, out.String()
}

// TestADotDotIsCanceledUnseenOrNot is the table the panel splits on, asked of
// both readings that the operand alone can tell apart.
//
// The last three rows are the control, and they are why this is about the
// component being looked at rather than about a `..` being refused: an
// ordinary `a/../b` where `a` is there is 0 under both readings.
func TestADotDotIsCanceledUnseenOrNot(t *testing.T) {
	for _, tc := range []struct {
		src            string
		unseen, looked int
	}{
		{"cd nosuch/..", 0, 1},
		{"cd real/nosuch/..", 0, 1},
		{"cd nosuch/../real", 0, 1},
		{"cd ./nosuch/..", 0, 1},
		{"cd afile/../b", 0, 1},
		{"cd dangling/../b", 0, 1},
		// The controls.
		{"cd real/..", 0, 0},
		{"cd ..", 0, 0},
		{"cd real/deep/../..", 0, 0},
	} {
		t.Run(tc.src, func(t *testing.T) {
			root := canceledTree(t)
			if st, _, errs := cdDotDot(t, root, CdDotDotCanceledUnseen, tc.src); st != tc.unseen {
				t.Errorf("canceled unseen: %s = %d, want %d — %s", tc.src, st, tc.unseen, errs)
			}
			if st, _, errs := cdDotDot(t, root, CdDotDotLooksAtEveryCanceledComponent, tc.src); st != tc.looked {
				t.Errorf("looking: %s = %d, want %d — %s", tc.src, st, tc.looked, errs)
			}
		})
	}
}

// TestAnAbsoluteOperandReadsTheAxisToo is the row this shell used to answer
// both ways at once: an absolute operand went to the kernel with its `..`
// intact and was refused, and a relative one was cleaned first and accepted,
// so one construct got the looking answer and the canceling answer out of one
// Semantics (#4627).
func TestAnAbsoluteOperandReadsTheAxisToo(t *testing.T) {
	root := canceledTree(t)
	// Written out rather than joined: filepath.Join cleans, so the `..` this
	// row is about would be gone before the shell ever saw it — which is the
	// same cancellation the axis is named for, applied to the test.
	absent := root + "/nosuch/.."
	if st, dir, errs := cdDotDot(t, root, CdDotDotCanceledUnseen, "cd "+absent); st != 0 || dir != root {
		t.Errorf("canceled unseen: cd %s = %d in %s, want 0 in %s — %s", absent, st, dir, root, errs)
	}
	if st, _, _ := cdDotDot(t, root, CdDotDotLooksAtEveryCanceledComponent, "cd "+absent); st != 1 {
		t.Errorf("looking: cd %s = %d, want 1", absent, st)
	}
}

// TestTheReasonNamesWhatTheComponentIs: a component that is an ordinary file
// is ENOTDIR and one that is not there at all is ENOENT, which is what says
// the question is whether the component is a **directory** rather than whether
// the name is taken.
func TestTheReasonNamesWhatTheComponentIs(t *testing.T) {
	root := canceledTree(t)
	for _, tc := range []struct{ src, want string }{
		{"cd afile/../b", "not a directory"},
		{"cd nosuch/..", "no such file or directory"},
		{"cd dangling/../b", "no such file or directory"},
	} {
		_, _, errs := cdDotDot(t, root, CdDotDotLooksAtEveryCanceledComponent, tc.src)
		if !strings.Contains(strings.ToLower(errs), tc.want) {
			t.Errorf("%s said %q, want %q in it", tc.src, errs, tc.want)
		}
	}
}

// TestAComponentPutBackAfterReachingIntoTheDirectoryIsLookedAt is the ksh93
// half of #6081. Under the within-the-operand reading a `..` that reaches
// past the operand cancels a component of the directory the shell is in
// unseen — and that directory is one component shorter afterwards, so what
// the operand puts back is the operand's own. The mark was not lowered, so
// `../nosuch/..` read its second `..` as reaching into base too and moved;
// ksh93u+ refuses it, measured 2026-10-05, as it refuses `nosuch/..`.
func TestAComponentPutBackAfterReachingIntoTheDirectoryIsLookedAt(t *testing.T) {
	root := canceledTree(t)
	from := filepath.Join(root, "b")
	for _, src := range []string{"cd ../nosuch/..", "cd ../b/../nosuch/.."} {
		if st, dir, _ := cdDotDot(t, from, CdDotDotLooksWithinTheOperand, src); st == 0 || dir != from {
			t.Errorf("%s = %d in %s, want a refusal staying in %s", src, st, dir, from)
		}
	}
	// The control: the component the operand puts back is there.
	if st, dir, errs := cdDotDot(t, from, CdDotDotLooksWithinTheOperand, "cd ../real/.."); st != 0 || dir != root {
		t.Errorf("cd ../real/.. = %d in %s, want 0 in %s — %s", st, dir, root, errs)
	}
}

// TestALinkedComponentIsADirectory: the looking follows links, so a `..`
// behind a symbolic link to a directory cancels — which is what keeps #4590's
// whole grid unmoved under the looking reading.
func TestALinkedComponentIsADirectory(t *testing.T) {
	root := canceledTree(t)
	st, dir, errs := cdDotDot(t, root, CdDotDotLooksAtEveryCanceledComponent, "cd sub/fake/..")
	if st != 0 || dir != filepath.Join(root, "sub") {
		t.Errorf("cd sub/fake/.. = %d in %s, want 0 in %s/sub — %s", st, dir, root, errs)
	}
}

// TestAPhysicalResolutionThatFailedKeepsItsDotDot is the other half of #4627
// and is not on the axis at all: under `-P` the whole path is walked with its
// `..` in place, and all six columns refuse `cd -P nosuch/..`.
//
// This shell answered 0 in every dialect, because a path the walk could not
// resolve fell through to the lexical join the walk existed to avoid — so the
// `..` was canceled against a component nobody had looked at and the shell
// arrived where it already was.
func TestAPhysicalResolutionThatFailedKeepsItsDotDot(t *testing.T) {
	root := canceledTree(t)
	for _, p := range []CdDotDotCancellationPolicy{
		CdDotDotCanceledUnseen,
		CdDotDotLooksAtEveryCanceledComponent,
		CdDotDotLooksWithinTheOperand,
	} {
		if st, _, _ := cdDotDot(t, root, p, "cd -P nosuch/.."); st == 0 {
			t.Errorf("%v: cd -P nosuch/.. was 0, want a refusal whatever the axis holds", p)
		}
		// The control: a `-P` that can resolve still arrives.
		if st, dir, errs := cdDotDot(t, root, p, "cd -P real/.."); st != 0 || dir != root {
			t.Errorf("%v: cd -P real/.. = %d in %s, want 0 in %s — %s", p, st, dir, root, errs)
		}
	}
}
