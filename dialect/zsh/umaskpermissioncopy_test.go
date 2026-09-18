// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// This shell takes no permission copy at all, which is what the unanswered
// verdict on Semantics.UmaskPermissionCopyBesideLetters stands on: the copy is
// refused before anything can look at what is beside it, so a clause holding a
// copy and a letter never reaches that question.
//
// Measured 2026-09-18 from a script file under `LC_ALL=C`, from `umask 222`.
func TestUmaskRefusesAPermissionCopy(t *testing.T) {
	for _, src := range []string{"umask -S g=u", "umask -S g=uw", "umask -S g=wu"} {
		out, st := runZsh(t, t.TempDir(), "umask 222\n"+src)
		if st == 0 || !strings.Contains(out, "u") {
			t.Errorf("%s: out %q status %d, want the copy refused", src, out, st)
		}
	}
}
