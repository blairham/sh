// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// What each preset fills when `read`'s array letter is written with no name
// after it — the question interp.ReadArrayDefaultStyle asks, graded against
// the shipped vectors rather than a synthetic one.
//
// Both parameters are read back on every row, and both are pre-set with data
// neither read could produce. The rule is keyed on the **parameter** and the
// two candidates differ only in case, so a row that read back the one it
// expected to be filled could not tell a shell that filled the other from a
// shell that filled nothing at all.
//
// Measured 2026-09-26, `reply=(k1 k2); REPLY=ks` ahead of `read -A <<<'hello
// world'`, zsh under `-f` and ksh93 from a script file:
//
//	zsh 5.9.2           reply becomes (hello world); REPLY left as ks — and
//	                    with no REPLY pre-set, `typeset -p REPLY` afterwards
//	                    says no such variable, so it is not created either
//	ksh93u+ 2012-08-01  REPLY='hello world', one scalar; reply left as (k1 k2)
//
// zsh's is the array neighbour of the `REPLY` a bare `read` fills. ksh93's is
// not a second default name: the letter with no name to apply to contributes
// nothing there, and `reply` is a parameter ksh93 has never heard of.
func TestEachDialectFillsItsOwnArrayReadDefault(t *testing.T) {
	const src = `reply=(k1 k2); REPLY=ks
printf 'hello world\n' | { read -A
printf 'n=%s' "${#reply[@]}"; for e in "${reply[@]}"; do printf '[%s]' "$e"; done
printf ' REPLY=[%s]\n' "$REPLY"; }`
	for _, c := range []struct{ dialect, want string }{
		{"zsh", "n=2[hello][world] REPLY=[ks]\n"},
		{"ksh", "n=2[k1][k2] REPLY=[hello world]\n"},
	} {
		t.Run(c.dialect, func(t *testing.T) {
			out, _, err := presets[c.dialect].Combined(t, dialecttest.Base{}, src)
			if err != nil {
				t.Fatal(err)
			}
			if out != c.want {
				t.Errorf("said %q, want %q", out, c.want)
			}
		})
	}
}

// The other three cannot be asked, which is why they carry the axis in
// internal/axissweep/testdata/unanswered.txt rather than an answer.
//
// bash's spelling is the lowercase `-a`, whose name is the option's own
// argument: `read -A` is not its letter at all, and `read -a` with nothing
// after it is refused by the option parser before any of this. dash and
// BusyBox ash have no array letter and no arrays. Measured 2026-09-26 —
// bash 5.3.20 `read -A <<<'x y'` is `read: -A: invalid option` at 2, and
// `read -a <<<'x y'` is `read: -a: option requires an argument` at 2.
func TestTheArrayReadDefaultCannotBeAskedOfTheOtherThree(t *testing.T) {
	for _, c := range []struct{ dialect, src string }{
		{"bash", `read -A; echo "st=$?"`},
		{"bash", `read -a; echo "st=$?"`},
		{"dash", `read -A; echo "st=$?"`},
		{"ash", `read -A; echo "st=$?"`},
	} {
		t.Run(c.dialect+" "+c.src, func(t *testing.T) {
			out, _, err := presets[c.dialect].Combined(t, dialecttest.Base{}, c.src)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "st=2") {
				t.Errorf("said %q, want the option refused at 2", out)
			}
			if !strings.Contains(out, "read") {
				t.Errorf("said %q, want the builtin named", out)
			}
		})
	}
}
