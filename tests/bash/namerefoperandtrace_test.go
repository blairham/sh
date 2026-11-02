// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A declaration's operand through a name reference is traced as the script
// spelled it here, on the command's own line, where ksh93 writes the variable
// it lands in (#7102). Measured 2026-11-02 against bash 5.3.20 from `-c`
// under `env -i PATH=/usr/bin:/bin`. See interp/xtracenameref.go.
func TestADeclarationOperandThroughAReferenceKeepsItsWord(t *testing.T) {
	t.Parallel()
	const refs = `x=0; declare -n m=x; `
	for _, c := range []struct{ name, src, want string }{
		{"declare", refs + `set -x; declare m=3`, "+ declare m=3\n"},
		{"export", refs + `set -x; export m=4`, "+ export m=4\n+ m=4\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runBash(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
