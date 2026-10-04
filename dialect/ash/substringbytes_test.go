// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// TestASubstringCountsBytes: a range's numbers are bytes, `${#s}` is
// characters. Measured 2026-10-03 in the pinned image under LC_ALL=C.UTF-8.
// See interp.Semantics.SubstringCountsBytes.
func TestASubstringCountsBytes(t *testing.T) {
	out, _, err := preset.Combined(t, dialecttest.Base{
		Name: "ash", Env: []string{"PATH=/usr/bin:/bin", "LC_ALL=C.UTF-8"},
	}, `s=héllo; echo "${#s}[${s:1:2}][${s:3}][${s: -4}][${s: -4:2}][${s:1:-1}][${s:0:1}]"`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "5[é][llo][\xa9llo][\xa9l][éll][h]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
