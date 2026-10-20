// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// TestAnOmittedWhoGrantsOnlyWhatTheMaskAllows, measured 2026-10-03 in the
// pinned image: the grant of a clause with no who goes through the mask the
// command started from, and what a clause clears does not.
func TestAnOmittedWhoGrantsOnlyWhatTheMaskAllows(t *testing.T) {
	out, _ := runUmask(t, `umask 022; umask =w; umask; umask 022; umask a=r,+w; umask; umask 022; umask a+w,=r; umask; umask 027; umask =rx; umask`)
	if want := "0577\n0133\n0333\n0227\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestAnUnsetLocaleStillCountsCharacters: `LC_ALL=` and nothing else is a
// locale nothing names, and the length is still five.
func TestAnUnsetLocaleStillCountsCharacters(t *testing.T) {
	out, _, err := preset.Combined(t, dialecttest.Base{
		Name: "ash", Env: []string{"PATH=/usr/bin:/bin", "LC_ALL="},
	}, `s=héllo; echo "len=${#s}"`)
	if err != nil || out != "len=5\n" {
		t.Errorf("got %q (%v), want len=5", out, err)
	}
}
