// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "testing"

// The view hands out a copy, so a caller that writes through the answer
// cannot rewrite the table for every shell in the process.
//
// Nothing in the expansion machinery does that today and the parameter is
// read-only besides, which is exactly why this is worth a test: the property
// has no user, so nothing else would notice the day it stopped holding.
func TestReswordsViewDoesNotHandOutItsBackingArray(t *testing.T) {
	first := zshReservedWordsView(nil)
	if len(first) != len(zshReservedWords) {
		t.Fatalf("view returned %d words, want %d", len(first), len(zshReservedWords))
	}
	first[0] = "clobbered"
	if zshReservedWords[0] != "if" {
		t.Errorf("writing the view's answer changed the package list to %q", zshReservedWords[0])
	}
	if second := zshReservedWordsView(nil); second[0] != "if" {
		t.Errorf("a second read answered %q, want if", second[0])
	}
}
