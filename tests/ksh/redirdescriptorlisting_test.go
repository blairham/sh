// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// `typeset -f` here says a redirection back **exactly as it was written**,
// which is the third of the three answers the panel gives and the one that
// makes syntax.Layout.RedirectDescriptor's zero value the conservative
// choice: a printer that neither adds nor removes cannot be wrong about a
// descriptor nobody asked it to think about.
//
// The rows that matter are the ones where the other two columns *change*
// something and this one does not. Measured 2026-09-29 on ksh93u+.
func TestTypesetSaysTheDescriptorBackAsWritten(t *testing.T) {
	for _, tc := range []struct{ name, written, want string }{
		// zsh drops these, bash adds to them, this column leaves both alone.
		{"a bare duplication stays bare", "echo x >&2", ">&2"},
		{"and a written one stays written", "echo x 1>&2", "1>&2"},
		{"a bare read duplication", "read v <&3", "<&3"},
		{"and a written one", "read v 0<&3", "0<&3"},
		// Both of the others drop this; this column keeps it.
		{"a file redirection keeps its default", "echo x 1>out", "1>out"},
		// bash rewrites this operator; this column does not.
		{"a close keeps the operator it was written with", "read v <&-", "<&-"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), "f() { "+tc.written+"; }\ntypeset -f f\n")
			if st != 0 {
				t.Errorf("status %d", st)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("listing %q, want it to contain %q", out, tc.want)
			}
		})
	}
}
