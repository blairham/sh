// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A listing spells bytes above ASCII out where the locale has no characters
// there, and writes the character where it has. Measured 2026-10-03 on bash
// 5.3.20; see Semantics.ListedNonAsciiFollowsTheLocale.
func TestAListingSpellsHighBytesByTheLocale(t *testing.T) {
	const src = "v1=$(printf '\\303\\251'); v2=$(printf 'a\\tb\\303\\251'); set | grep '^v[12]='; declare -p v1"
	for _, tc := range []struct{ locale, want string }{
		{"C", "v1=$'\\303\\251'\nv2=$'a\\tb\\303\\251'\ndeclare -- v1=$'\\303\\251'\n"},
		{"en_US.UTF-8", "v1=é\nv2=$'a\\tbé'\ndeclare -- v1=\"é\"\n"},
	} {
		t.Run(tc.locale, func(t *testing.T) {
			out, _, err := preset.Combined(t, dialecttest.Base{
				Name: "bash", Env: []string{"PATH=/usr/bin:/bin", "LC_ALL=" + tc.locale},
			}, src)
			if err != nil || out != tc.want {
				t.Errorf("= %q (%v), want %q", out, err, tc.want)
			}
		})
	}
}
