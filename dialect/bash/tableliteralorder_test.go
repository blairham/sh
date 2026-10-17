// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestAKeyedLiteralIsExpandedWhole is the No side of
// interp.Semantics.TableLiteralStoresEachElementAsItIsExpanded: every pair is
// expanded before any is stored, so a later pair reads the table as it was
// (#6770). Measured 2026-10-16 on bash 5.3.20, `-c`.
func TestAKeyedLiteralIsExpandedWhole(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `declare -A a=([k]=old); a=([k]=1 [x]=${a[k]}); echo "${a[x]}"`)
	if out != "old\n" {
		t.Errorf("got %q, want %q", out, "old\n")
	}
}
