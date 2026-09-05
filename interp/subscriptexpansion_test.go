// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// A subscript being assigned through is a word, so an expansion stands in one
// and the element it names is written.
//
// This is what a loop does, and it did not work: the word became a command
// name, so the shell reported `command not found` and the array kept its old
// contents — noisy at the moment of failure and silent for every read
// afterwards.
func TestAssigningThroughASubscriptThatExpands(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(x y); i=1; a[$i]=Q; printf "[%s]" "${a[@]}"`, "[x][Q]"},
		{`a=(x y); i=1; a[${i}]=Q; printf "[%s]" "${a[@]}"`, "[x][Q]"},
		{`a=(x y); i=1; a[$((i))]=Q; printf "[%s]" "${a[@]}"`, "[x][Q]"},
		{`a=(x y); a[$(echo 1)]=Q; printf "[%s]" "${a[@]}"`, "[x][Q]"},
		// The expansion is a piece of the expression rather than the whole
		// of it, which is the write path's arithmetic reading of a subscript.
		{`a=(x y z); i=1; a[$i+1]=Q; printf "[%s]" "${a[@]}"`, "[x][y][Q]"},
		{`a=(x y); a["1"]=Q; printf "[%s]" "${a[@]}"`, "[x][Q]"},
		// `+=` joins the element the subscript names, and the `]+=` is the
		// part of the word the old scan never reached.
		{`a=(x y); i=1; a[$i]+=Q; printf "[%s]" "${a[@]}"`, "[x][yQ]"},
		// Both halves of the assignment expand, and the value of a
		// subscripted assignment is not field-split.
		{`v="p q"; a=(x y); i=1; a[$i]=$v; printf "[%s]" "${a[@]}"`, "[x][p q]"},
		// A loop writing every element, which is the shape the report was
		// filed about.
		{`for i in 0 1 2; do a[$i]=v$i; done; printf "[%s]" "${a[@]}"`, "[v0][v1][v2]"},
	} {
		out, st := runArray(t, c.src)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("%s = %q, want %q", c.src, got, c.want)
		}
		if st != 0 {
			t.Errorf("%s: status = %d, want 0", c.src, st)
		}
	}
}

// A name broken by an expansion is not a name, so the word stays a command
// name and the shell reports what the expansion made of it.
func TestANameBrokenByAnExpansionIsNotAnAssignment(t *testing.T) {
	out, st := runArray(t, `b=X; a$b=c`)
	if !strings.Contains(out, "aX=c") {
		t.Errorf("got %q, want the expanded word reported as a command", out)
	}
	if st != 127 {
		t.Errorf("status = %d, want 127", st)
	}
}
