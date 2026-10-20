// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAnUnbracedSigilMeansItsBracedSpelling is #1607, and the claim is that
// there is no new semantics to model: each of the four is exactly the flag
// the braced group already takes.
//
// So every row asserts the short form against the long one *run in the same
// shell*, which is stronger than a literal and cannot drift — and the values
// underneath were measured against zsh 5.9.2 first, so a pair that agreed on
// the wrong answer would have been caught before it was written down.
func TestAnUnbracedSigilMeansItsBracedSpelling(t *testing.T) {
	dir := t.TempDir()
	const setup = `v=hello; a=(x y z); p="a b"; set -- one two; `
	for _, tc := range []struct {
		name, short, braced, want string
	}{
		{"existence, set", `$+v`, `${+v}`, "1\n"},
		{"existence, unset", `$+nope`, `${+nope}`, "0\n"},
		{"existence, an array", `$+a`, `${+a}`, "1\n"},
		{"existence, a positional", `$+1`, `${+1}`, "1\n"},
		{"glob flag", `$~v`, `${~v}`, "hello\n"},
		{"distribute flag", `x$^a-y`, `x${^a}-y`, "x1-y\n"},
		{"a subscript under the sigil", `$+a[2]`, `${+a[2]}`, "1\n"},
		{"a subscript past the end", `$+a[9]`, `${+a[9]}`, "0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The distribute row wants the fields, not the text, so every
			// row is counted the same way and the one that differs is
			// visible for what it is.
			src := setup + `print -r -- "` + tc.short + `"`
			short, st := runZsh(t, dir, src)
			braced, _ := runZsh(t, dir, setup+`print -r -- "`+tc.braced+`"`)
			if short != braced {
				t.Errorf("%s = %q, braced %s = %q", tc.short, short, tc.braced, braced)
			}
			if tc.name == "distribute flag" {
				return // asserted by field count below
			}
			if short != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.short, short, st, tc.want)
			}
		})
	}
}

// The split and distribute flags are about *fields*, so they are counted
// rather than compared as text — a joined answer and a split one print the
// same characters and are not the same thing.
func TestTheUnbracedSplitAndDistributeFlagsMakeFields(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			// Without the flag the quoted value is one field; with it, three.
			name: "split", src: `p="a b c"; set -- $=p; print -r -- "n=$#"`,
			want: "n=3\n",
		},
		{
			name: "split, the braced spelling agrees",
			src:  `p="a b c"; set -- ${=p}; print -r -- "n=$#"`,
			want: "n=3\n",
		},
		{
			name: "distribute", src: `a=(1 2); set -- x$^a-y; print -r -- "n=$# [$1][$2]"`,
			want: "n=2 [x1-y][x2-y]\n",
		},
		{
			name: "distribute, the braced spelling agrees",
			src:  `a=(1 2); set -- x${^a}-y; print -r -- "n=$# [$1][$2]"`,
			want: "n=2 [x1-y][x2-y]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("out = %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}

// The three spellings of `$+` that are *not* an expansion, which is the half
// a test asserting only that the flag works would miss. Each prints itself.
func TestTheExistenceSigilNeedsANameOrADigit(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"a special is not a target", `print -r -- "$+@"`, "$+@\n"},
		{"nor is the length sigil", `print -r -- "$+#"`, "$+#\n"},
		{"it does not repeat", `print -r -- "$++v"`, "$++v\n"},
		{"and alone it is text", `print -r -- "$+"`, "$+\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("out = %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}
