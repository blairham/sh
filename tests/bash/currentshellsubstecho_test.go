// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestARefusedCurrentShellBodyEchoesTheLine — a token refused inside a
// `${ …;}` or `${| …;}` body is followed by the whole line, as one inside
// `$( … )` is. Measured 2026-10-04 on bash 5.3.20; see substitutions.md.
func TestARefusedCurrentShellBodyEchoesTheLine(t *testing.T) {
	for _, src := range []string{
		`echo "[${ | REPLY=hi;}]"`,
		`x=${|;}`,
		`echo "[${ )x;}]"`,
		`echo "[$(|x)]"`, // the control: the parenthesized spelling already did
	} {
		_, errs, _ := runAlias(t, src)
		if !strings.HasSuffix(errs, "line 1: `"+src+"'\n") {
			t.Errorf("%s: said %q, want the line echoed under it", src, errs)
		}
	}
}
