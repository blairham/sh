// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package asttree

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// A node reached twice is refused: a literal would write it out twice, and
// the two copies compare equal by value to the one shared node, so nothing
// downstream could tell the tree had changed shape.
func TestANodeReachedTwiceIsRefused(t *testing.T) {
	f, err := syntax.Parse("echo hi\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Emit(f, "echo hi\n", "p", "v", "", nil); err != nil {
		t.Fatalf("the plain tree: %v", err)
	}
	f.Stmts = append(f.Stmts, f.Stmts[0])
	if _, err := Emit(f, "echo hi\n", "p", "v", "", nil); err == nil || !strings.Contains(err.Error(), "reached twice") {
		t.Errorf("a shared statement was written out: %v", err)
	}
}

// A constant stretch is spelled as its expression, and so is every offset
// after it — and a column only on the stretch's own line.
func TestAConstantStretchIsSpelledAsItsExpression(t *testing.T) {
	src := "echo arm64-darwin x\necho y\n"
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	out, err := Emit(f, src, "p", "v", "", []Constant{{At: 5, Text: "arm64-darwin", Expr: "machine"}})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, want := range []string{
		"Value: machine,",
		// x, after the stretch on its line: offset and column both move.
		"syntax.Pos{Offset: 6 + int32(len(machine)), Line: 1, Col: 7 + int32(len(machine))}",
		// y, on the next line: the offset moves and the column does not.
		"syntax.Pos{Offset: 13 + int32(len(machine)), Line: 2, Col: 6}",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "arm64-darwin") {
		t.Errorf("the build's own spelling survived:\n%s", got)
	}
	if _, err := Emit(f, src, "p", "v", "", []Constant{{At: 4, Text: "arm64-darwin", Expr: "machine"}}); err == nil {
		t.Error("a constant that is not at its offset was accepted")
	}
}
