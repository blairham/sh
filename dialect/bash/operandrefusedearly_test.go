// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A declaration's array literal that its store will refuse is refused ahead
// of the command here: the command's own redirections never apply to the
// sentence, the sentence names no builtin, and the rest of the line is given
// up. Measured 2026-10-03 on bash 5.3.20; see
// Semantics.ArrayOperandRefusedBeforeTheCommand.
func TestADeclarationsLiteralIsRefusedBeforeTheCommand(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"readonly q=1; typeset q=(b) 2>/dev/null; echo st=$?\necho next $?", "q: readonly variable\nnext 1\n"},
		{"readonly q=1; export q=(b) >/dev/null 2>&1; echo st=$?", "q: readonly variable\n"},
		{"typeset -A h; typeset -a h=(x) 2>/dev/null; echo st=$?", "h: cannot convert associative to indexed array\n"},
		{"typeset -a c=(x); typeset -A c=([k]=v) 2>/dev/null; echo st=$?", "c: cannot convert indexed to associative array\n"},
		// The controls: a scalar operand is the utility's own, under its
		// redirection, and a literal without the kind letter lands in the
		// table.
		{"readonly q=1; typeset q=b 2>/dev/null; echo st=$?", "st=1\n"},
		{"typeset -A h; typeset h=(x) 2>/dev/null; echo st=$?", "st=0\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.HasSuffix(out, tc.want) || strings.Contains(out, "typeset:") {
				t.Errorf("= %q, want it to end %q with no builtin named", out, tc.want)
			}
		})
	}
}
