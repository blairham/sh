// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// A name spelled as a namespace — a separator in front, as a dialect's own
// private store is named — is no evidence that a member exists, so it must
// not turn on the walk every local declaration then pays for. That walk reads
// every name the shell holds, and the flag is sticky: one dialect writes its
// option store on the first `setopt`, so before this every `local` in every
// session of it cost about thirty microseconds with two thousand parameters
// in the shell (2026-10-05, the maintainer's real configuration). See
// noteMemberName.
//
// The control is the point of the second half: a name that *is* a member has
// to turn it on, or the first half is a gate that never opens.
func TestANameSpelledAsANamespaceIsNoEvidenceOfAMember(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.SetVar(".private.store", "x")
	r.SetArray(".private.list", []string{"a", "b"})
	if r.MemberNamesInUseForTest() {
		t.Error("a name spelled as a namespace turned the member walk on")
	}
	r.SetVar("c.a", "1")
	if !r.MemberNamesInUseForTest() {
		t.Error("a member name did not turn the walk on: the row above proves nothing")
	}
}
