// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// A subscript's flag group holds the assignment together, and it is its own
// flag that says so.
//
// `b[(r)y]=Q` is one word: the `(` at the front of a subscript belongs to it,
// whatever the dialect says about *pattern* groups. That distinction is not
// academic — it worked only by accident where the dialect happened to have
// bare groups as well, which is a different flag answering a question that is
// not its.
func TestASubscriptFlagGroupHoldsAnAssignmentTogether(t *testing.T) {
	d := Core()
	d.ArraySubscript = true
	d.ArraySubscriptFlags = true
	f, err := Parse(`b[(r)y]=Q`, d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if len(sc.Assigns) != 1 || len(sc.Args) != 0 {
		t.Fatalf("%d assignments and %d words, want one assignment and no words", len(sc.Assigns), len(sc.Args))
	}
	a := sc.Assigns[0]
	if a.IndexFlags == nil {
		t.Fatalf("no flag group on the subscript: index is %q", wordText(a.Index))
	}
	if a.IndexFlags.Flags != "r" || wordText(a.IndexFlags.Arg) != "y" {
		t.Errorf("flags=%q operand=%q, want %q and %q",
			a.IndexFlags.Flags, wordText(a.IndexFlags.Arg), "r", "y")
	}
	// The whole subscript is still there as written, which is what a
	// diagnostic naming it needs.
	if got := wordText(a.Index); got != "(r)y" {
		t.Errorf("index is %q, want the subscript as written", got)
	}

	// Without the flag the group is not one, so the word ends at the paren
	// and the assignment is not an assignment.
	plain := Core()
	plain.ArraySubscript = true
	if _, err := Parse(`b[(r)y]=Q`, plain); err == nil {
		t.Error("parsed without the flag, want the paren to end the word")
	}

	// And it goes back out as it came: escaping the parentheses would make
	// the reparse read arithmetic where the source named an element.
	if got := Print(f); got != "b[(r)y]=Q" {
		t.Errorf("printed %q, want the group's parentheses bare", got)
	}
}

// wordText is a word's spans run together, for the assertions above.
func wordText(w *Word) string {
	if w == nil {
		return ""
	}
	var b strings.Builder
	for _, s := range w.Spans {
		b.WriteString(s.Value)
	}
	return b.String()
}
