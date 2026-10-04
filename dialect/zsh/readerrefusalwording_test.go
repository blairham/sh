// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// Three refusals where the reading was already a refusal and only the
// sentence was another shell's. Every row measured 2026-10-04 on zsh 5.9.2,
// and each group has a control that keeps the old sentence.
func TestReaderRefusalsAreWordedAsThisShellWordsThem(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// `=` and `^` are no flags inside the group.
		{"an equals sign in a flag group", `x=ab; echo ${(=)x}`, "error in flags near position 4 in '${(=)x}'"},
		{"a caret in one", `x=ab; echo ${(^)x}`, "error in flags near position 4 in '${(^)x}'"},
		{"blamed where it stands", `echo ${(v=2; echo x)}`, "error in flags near position 5"},
		// A point and a digit are a whole literal where an operator belonged.
		{"a fraction behind a hex numeral", `echo $((0x10.5))`, "operator expected at `.5'"},
		{"behind a blank", `echo $((1 .5))`, "operator expected at `.5'"},
		{"behind a name", `echo $((a.5))`, "operator expected at `.5'"},
		{"control: no digit after the point", `echo $((0xf.f))`, "bad floating point constant"},
		{"control: a decimal numeral reads it", `echo $((1.5.5))`, "bad floating point constant"},
		{"control: so does one that begins with its point", `echo $((.5.5))`, "bad floating point constant"},
		// The `e` qualifier's string has to close.
		{"an e with nothing after it", `echo x(e)`, "missing end of string"},
		{"a mid-word group read as qualifiers", `echo x=(echo hi)`, "missing end of string"},
		{"control: another letter", `echo x(a)`, "number expected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, errs := runZshSplit(t, t.TempDir(), tc.src)
			if !strings.Contains(errs, tc.want) {
				t.Errorf("stderr = %q, want it to contain %q", errs, tc.want)
			}
		})
	}
}
