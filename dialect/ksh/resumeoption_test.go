// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// **`fg` and `bg` refuse an option word before saying anything about job
// control** — Semantics.JobResumeRefusesAnOptionFirst. Measured 2026-10-03 on
// ksh93u+ under -c: `bg --version` is the unknown option and the usage line
// at 2, and `bg %2` is still the silent 1 of a shell with no job control.
func TestFgAndBgRefuseAnOptionFirst(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`bg --version; print "st=$?"`, "ksh: bg: --version: unknown option\nUsage: bg [ options ] [job ...]\nst=2\n"},
		{`fg -x; print "st=$?"`, "ksh: fg: -x: unknown option\nUsage: fg [ options ] [job ...]\nst=2\n"},
		{`bg %2; print "st=$?"`, "st=1\n"},
		{`bg -- %1; print "st=$?"`, "st=1\n"},
	} {
		out, _ := runKsh(t, t.TempDir(), c.src)
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.src, out, c.want)
		}
	}
}
