// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAZipOperandMustBeAName pins that the zip and set operators refuse an
// operand that is not a name, in the characters it was written in, whatever
// the left side holds (#5151, a chunk of D04parameter.ztst). Measured
// 2026-10-03 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`).
func TestAZipOperandMustBeAName(t *testing.T) {
	const setup = "x=(p q); b=(1 2)\n"
	for _, tc := range []struct{ src, want string }{
		{`print ${x:^^^y}; echo after`, "zsh:2: not an identifier: ^y\n"},
		{`print ${x:|-y}`, "zsh:2: not an identifier: -y\n"},
		{`print ${x:*^y}`, "zsh:2: not an identifier: ^y\n"},
		{`y=b; print ${x:^$y}`, "zsh:2: not an identifier: $y\n"},
		{`print ${x:^"b"}`, "zsh:2: not an identifier: \"b\"\n"},
		{`print ${x:^ b}`, "zsh:2: not an identifier:  b\n"},
		{`print ${x:^b[1]}`, "zsh:2: not an identifier: b[1]\n"},
		{`print ${x:^@}`, "zsh:2: not an identifier: @\n"},
		{`print ${nope:^-y}`, "zsh:2: not an identifier: -y\n"},
		{`print "${x:^^-}"`, "zsh:2: not an identifier: -\n"},
		// A here-document body is expanded as one string, and is refused
		// without ending the script.
		{"cat <<E\n${x:^-y}\nE\necho after", "zsh:2: not an identifier: -y\nafter\n"},
		// An expansion's failure, not a parse's: an unrun one is no error.
		{`false && print ${x:^-y}; echo st=$?`, "st=1\n"},
		// The controls: names, an empty operand, and a leading digit.
		{`print ${x:^b} ${x:^^b} ${x:|b} ${x:*x} ${x:^} ${x:^12a}`, "p 1 q 2 p 1 q 2 p q p q p q p q\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// TestAZipReadsItsOperandAsAParameter pins the two ways the zips read the
// operand as a parameter: NO_UNSET refuses an unset one, and a positional
// parameter past the end is one empty element rather than no array. The set
// operators take neither. Measured 2026-10-03 on zsh 5.9.2.
func TestAZipReadsItsOperandAsAParameter(t *testing.T) {
	const setup = "x=(p q)\n"
	for _, tc := range []struct{ src, want string }{
		{`setopt nounset; print ${x:^nope}; echo after`, "zsh:2: nope: parameter not set\n"},
		{`setopt nounset; print "${x:^^nope}"`, "zsh:2: nope: parameter not set\n"},
		{`setopt nounset; print ${x:^}`, "zsh:2: : parameter not set\n"},
		{`setopt nounset; print ${nope:^nope2}`, "zsh:2: nope: parameter not set\n"},
		{`setopt nounset; print ${x:|nope} / ${x:*nope}.`, "p q / .\n"},
		{`print ${x:^nope}`, "p q\n"},
		{`print -l ${x:^1}; print -l "${x:^^1}"`, "p\np q\n\n"},
		{`set -- 7 8; print ${x:^2} ${x:^^1}`, "p 8 p 7 q 7\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
