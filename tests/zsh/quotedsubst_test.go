// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAQuotedSubstitutionSplitsByLine — `${(@f)"$(cmd)"}` is how a command's
// output is split into an array by line here, and it is common real-world
// code rather than a corner.
//
// The fields are printed with markers rather than through `echo`, because
// `echo` cannot tell them apart: two fields `a` and `b` print as `a b` on one
// line, and one field holding a newline prints as two lines.
func TestAQuotedSubstitutionSplitsByLine(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`printf "[%s]" ${(@f)"$(printf "a\nb")"}`, "[a][b]"},
		{`printf "[%s]" ${(f)"$(printf "a\nb")"}`, "[a][b]"},
		// The quotes are load-bearing: quoted, the inner is one field and
		// the flag splits *that* on newlines, so a space inside a line
		// survives. Measured on zsh 5.9.2.
		{`printf "[%s]" ${(@f)"$(printf "a b\nc")"}`, "[a b][c]"},
		// A trailing newline does not make an empty last field, because the
		// substitution took it off before the flag saw anything.
		{`printf "[%s]" ${(@f)"$(printf "a\nb\n")"}`, "[a][b]"},
		// A quoted *parameter* expansion stands in the same position.
		{`v=abc; printf "[%s]" ${"${v}"}`, "[abc]"},
		// And the whole point of the idiom: an array built from it.
		{`a=(${(@f)"$(printf "a b\nc")"}); printf "[%s]" "${a[@]}"; printf "n=%d" ${#a}`, "[a b][c]n=2"},
	} {
		if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 0 {
			t.Errorf("%s: out %q status %d, want %q", tc.src, out, st, tc.want)
		}
	}
}

// TestAQuotedNonSubstitutionIsStillRefused — the quotes wrap the shape rather
// than being one: what may stand in the name position is a substitution, and
// quoting a string does not make it one. Each of these is a bad substitution
// in the real shell, measured, and the refusal is what a script meeting one
// should get rather than a plausible value.
func TestAQuotedNonSubstitutionIsStillRefused(t *testing.T) {
	dir := t.TempDir()
	for _, src := range []string{
		`echo ${"abc"}`,
		`v=abc; echo ${"$v"}`,
		`echo ${(@f)'$(printf "a\nb")'}`,
	} {
		out, st := runZsh(t, dir, src)
		if st == 0 {
			t.Errorf("%s: out %q status 0, want a refusal", src, out)
		}
	}
}
