// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAnElementLiteralIsTracedAsItWasBuilt: the line holds the target alone
// while the literal's own substitution runs, and the expanded words by the
// time the subscript's does, where the words were written as spelled on every
// line (#6764). Measured 2026-10-15 on zsh 5.9.2, `-fc`.
func TestAnElementLiteralIsTracedAsItWasBuilt(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"substitutions on both sides", `a=(1 2); set -x; a[$(echo SUB >&2; echo 1)]=(p $(echo VAL >&2))`,
			"+zsh:1> a[$(echo SUB >&2; echo 1)]=+zsh:1> echo VAL\nVAL\n" +
				"+zsh:1> a[$(echo SUB >&2; echo 1)]=( p ) +zsh:1> echo SUB\nSUB\n" +
				"+zsh:1> echo 1\n" +
				"+zsh:1> a[$(echo SUB >&2; echo 1)]=( p ) \n",
		},
		{"quoted words", `a=(1 2); set -x; x=1 a[1]=(p "q r") y=2`, "+zsh:1> x=1 a[1]=( p 'q r' ) y=2 \n"},
		{"empty", `a=(1 2); set -x; a[1]=()`, "+zsh:1> a[1]=( ) \n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
