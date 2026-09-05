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

// runFdSubscript runs src where the descriptor a `{name}` token holds may be
// an element, which is the flag this file is about.
func runFdSubscript(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	d := syntax.Core()
	d.FdVariableSubscript = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	// Answered because these cases leave a hole below the element they use,
	// and whether a hole is an element is a different axis from the one
	// under test — an unanswered one would refuse the read before the
	// descriptor was ever looked at.
	sem.ArraysAreSparse = Yes
	var out strings.Builder
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "testsh",
		Dir: dir, Stdout: &out, Stderr: &out,
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), st
}

// `exec {a[1]}>f` opens the file and leaves the number in the element, which
// is the plain `{fd}>f` rule with the name being an element instead of a
// scalar.
func TestAPickedDescriptorLandsInTheElementThatNamedIt(t *testing.T) {
	dir := t.TempDir()
	out, st := runFdSubscript(t, dir, `exec {a[1]}>f
echo "held=${a[1]}"
echo written >&${a[1]}
exec {a[1]}>&-`)
	if st != 0 {
		t.Errorf("status = %d, output %q", st, out)
	}
	// Ten is where the shell starts picking, clear of the single digits a
	// script addresses itself; the number matters less than that the element
	// and not a variable called `a[1]` received it.
	if !strings.Contains(out, "held=10") {
		t.Errorf("the element did not receive the picked number: %q", out)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "f")); string(b) != "written\n" {
		t.Errorf("file = %q, want what was written through the element", b)
	}
}

// And `exec {a[1]}>&-` closes it, which is the form that matters: it is how a
// coprocess is told its input has ended, and a write afterwards is what
// proves the close happened rather than being reported.
func TestClosingThroughAnElementReallyCloses(t *testing.T) {
	dir := t.TempDir()
	out, _ := runFdSubscript(t, dir, `exec 3>f
a[1]=3
exec {a[1]}>&-
echo "st=$?"
echo x >&3
echo "after=$?"`)
	if !strings.Contains(out, "st=0") {
		t.Errorf("the close itself failed: %q", out)
	}
	if strings.Contains(out, "after=0") {
		t.Errorf("a write to the closed descriptor should fail: %q", out)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "f")); len(b) != 0 {
		t.Errorf("file = %q, want nothing written after the close", b)
	}
}

// The subscript is read the way every other subscript is, so an expression is
// one and a declared associative name takes a key instead. Two readings of
// one syntax, decided by the attribute rather than here.
func TestAnFdElementsSubscriptIsReadLikeAnyOther(t *testing.T) {
	dir := t.TempDir()
	out, _ := runFdSubscript(t, dir, `exec 3>f
a[2]=3
i=1
exec {a[i+1]}>&-
echo x >&3
echo "expr=$?"`)
	if strings.Contains(out, "expr=0") {
		t.Errorf("an expression subscript did not reach the element: %q", out)
	}

	out, _ = runFdSubscript(t, dir, `declare -A m
exec 3>g
m[key]=3
exec {m[key]}>&-
echo x >&3
echo "key=$?"`)
	if strings.Contains(out, "key=0") {
		t.Errorf("a key subscript did not reach the element: %q", out)
	}
}

// An element that holds no descriptor is the same complaint a scalar holding
// none gets, and by the same axis — the name in it is the element as written.
func TestAnElementHoldingNoDescriptorIsTheSameComplaint(t *testing.T) {
	dir := t.TempDir()
	out, st := runFdSubscript(t, dir, `exec 3>f; exec {a[1]}>&-`)
	if st == 0 {
		t.Errorf("closing through an element that holds nothing should fail: %q", out)
	}
	if !strings.Contains(out, "a[1]") {
		t.Errorf("the complaint should name the element as written: %q", out)
	}
}
