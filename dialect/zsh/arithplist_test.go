// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// TestANestedNameReferenceToAListInArithmeticIsABadSubstitution pins the
// wording of the nested `(P)` refusal where the text is read as a quoted
// string is: arithmetic, a range offset, a subscript and a key. As a value or
// an operand word it keeps the command line's sentence, and `(( … ))` goes on
// to the next line (#5609). Measured 2026-10-03 on zsh 5.9.2 under `-f`. Only
// the first line of what is written is compared: a second complaint after it
// is #5608's.
func TestANestedNameReferenceToAListInArithmeticIsABadSubstitution(t *testing.T) {
	const setup = "x1=abc; v=(x1 x2); s=hello\n"
	for _, tc := range []struct{ src, want string }{
		{`echo $(( ${#${(P)v}} ))`, "zsh:2: bad substitution"},
		{`echo $[ ${#${(P)v}} ]`, "zsh:2: bad substitution"},
		{`echo ${s:${#${(P)v}}}`, "zsh:2: bad substitution"},
		{`echo ${v[${#${(P)v}}]}`, "zsh:2: bad substitution"},
		{`a[${#${(P)v}}]=1`, "zsh:2: bad substitution"},
		{`typeset -A h; h[${${(P)v}}]=1`, "zsh:2: bad substitution"},
		{`(( ${#${(P)v}} )); echo after`, "zsh:2: bad substitution"},
		{`a[1]=${${(P)v}}`, "zsh:2: parameter name reference used with array"},
		{`print ${u:-${${(P)v}}}`, "zsh:2: parameter name reference used with array"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if first, _, _ := strings.Cut(got, "\n"); first != tc.want {
			t.Errorf("%s\n got %q\nwant first line %q", tc.src, got, tc.want)
		}
	}
	// And the arithmetic command alone is what the failure ends.
	got, _ := runZsh(t, t.TempDir(), setup+"(( ${#${(P)v}} )); echo after")
	if !strings.HasSuffix(got, "after\n") {
		t.Errorf("(( … )) ended the shell: %q", got)
	}
}
