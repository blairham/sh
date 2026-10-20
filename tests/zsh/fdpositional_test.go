// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `exec {1}>&-` closes the descriptor a *positional parameter* holds, which
// is how a function hands a descriptor back and how a prompt theme's
// scheduler closes the one it was handed.
//
// Both halves are asserted, because the read and the write are separate
// code and the read alone looked like it worked: `{1}` resolving to `$1`
// without the write is a shell that closes the right descriptor and then
// tells the caller the wrong number.
//
// zsh alone. Measured 2026-09-10: bash 5.3 answers `exec: {1}: not found`
// — it read the braces as a word — and ksh93 answers `1: invalid variable
// name`, which is why this is a dialect flag rather than a loosening of
// the lexer for everyone.
func TestAPositionalParameterCanNameADescriptor(t *testing.T) {
	// Which *number* the shell picks is not asserted: real zsh answers 11
	// where this shell answers 10 for the same script, because the two
	// have different descriptors already open, and that difference is
	// older than this feature. What is asserted is that the position was
	// written at all — before this it kept the value it came in with —
	// and that closing through it reaches the descriptor.
	out, st := runZsh(t, t.TempDir(), `set -- 9
exec {1}</dev/null
[[ $1 == 9 ]] && print -r -- "NOT WRITTEN"
[[ $1 == <-> && $1 -ge 10 ]] && print -r -- "written=a descriptor"
takes() { exec {1}>&-; print -r -- "closed=$?"; }
exec {v}</dev/null
takes $v
set -- a b c d e f g h i j k l
exec {12}</dev/null
[[ ${12} == <-> ]] && print -r -- "twelfth=a descriptor"`)
	want := "written=a descriptor\nclosed=0\ntwelfth=a descriptor\n"
	if out != want || st != 0 {
		t.Errorf("positional descriptors = %q (status %d), want %q", out, st, want)
	}
}

// And a name that merely *starts* with a digit is not one: the token is
// taken and the refusal comes from whoever resolves it, which is measured
// — `{1a}` is `not an identifier: 1a` in real zsh rather than a word.
// Asserted because the alternative reading, "digits anywhere make a
// positional", would quietly accept it and close nothing.
func TestADigitLeadingNameThatIsNotAllDigitsIsNoPosition(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `exec {v}</dev/null
print -r -- "opened=[${v:+yes}]"
exec {1a}</dev/null
print -r -- "after=$?"`)
	if want := "opened=[yes]\n"; out[:len(want)] != want {
		t.Errorf("output = %q, want it to start %q", out, want)
	}
	if st != 0 && st != 1 {
		t.Errorf("status %d, want the run to survive the refusal", st)
	}
}
