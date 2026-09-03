// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// The snapshot is a copy, not a view of the stack it was taken from.
//
// The stack is reused as the parser unwinds and carries on, so a snapshot
// sharing its array would report whatever happened to be written over it
// later. Nothing does that today — the parser stops at the first failure —
// which is exactly why this is worth pinning: the copy is what keeps a later
// change from making the answer wrong quietly, and a test that only parses
// cannot tell the two apart.
func TestTheSnapshotIsACopy(t *testing.T) {
	p := NewParser("if true\nthen\n", Core())
	p.Parse()
	before := p.Open()
	if len(before) != 2 {
		t.Fatalf("open = %v, want two", before)
	}
	// Write over the stack the snapshot was taken from.
	p.open = p.open[:0]
	p.open = append(p.open, opener{word: "something", line: 99, construct: true})
	p.open = append(p.open, opener{word: "else", line: 99})

	after := p.Open()
	if len(after) != 2 || after[0].Word != "if" || after[1].Word != "then" {
		t.Errorf("open = %v after the stack moved on, want %v", after, before)
	}
}
