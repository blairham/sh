// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// The word a substitution substitutes is a value, and the two list spellings
// join in it exactly as they join anywhere else a value is wanted: `*` on the
// first character of IFS, `@` on a space.
//
// Measured 2026-09-20 from script files under `env -i PATH=/usr/bin:/bin
// LC_ALL=C`, with `set -- 'a:b' c`, `arr=('p:q' r)` and `IFS=:`:
//
//	spelling              bash 5.3.20   bash 3.2.57   ksh93u+   zsh 5.9.2
//	v=${u=$*}             a:b:c         a:b:c         a:b:c     a:b:c
//	v=${u=$@}             a:b c         a b c         a:b c     a:b:c
//	v=${u=${arr[@]}}      p:q r         p q r         p:q r     p:q:r
//
// The `*` row is unanimous and is the one this shell had wrong — it answered
// `a b c` there, a space between the elements, because the word was expanded
// into fields and rejoined afterwards. The `@` rows are the axis that already
// decides an unquoted `@` where nothing is split, and this preset's answer to
// it was right before and after.
func TestASubstitutedWordIsAValueAndJoinsLikeOne(t *testing.T) {
	const set = `set -- 'a:b' c; arr=('p:q' r); IFS=:; `
	for _, c := range []struct{ name, src, want string }{
		{"a star in the assigning word", `v=${u=$*}; printf "[%s]" "$v"`, "[a:b:c]"},
		{"a star in the default word", `v=${u-$*}; printf "[%s]" "$v"`, "[a:b:c]"},
		{"a star subscript in the word", `v=${u-${arr[*]}}; printf "[%s]" "$v"`, "[p:q:r]"},
		// The other half of the pair, unchanged by the row above it: this
		// preset rejoins an unquoted `@` on a space where nothing is split.
		{"an at in the assigning word", `v=${u=$@}; printf "[%s]" "$v"`, "[a:b c]"},
		{"an at in the default word", `v=${u-$@}; printf "[%s]" "$v"`, "[a:b c]"},
		{"an at subscript in the word", `v=${u-${arr[@]}}; printf "[%s]" "$v"`, "[p:q r]"},
		// Where the value is used unquoted it is joined first and split
		// after, which is two stages and not one: three fields come out of
		// the joined string, where the list it was made from had two.
		{"joined first, then split where it is used", `printf "[%s]" ${u=$*}`, "[a][b][c]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), set+c.src)
			if out != c.want || st != 0 {
				t.Errorf("%q said %q (status %d), want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}

// The `?` word is the exception, and it is measured rather than inherited.
//
// The same script file, the same `set -- 'a:b' c` under `IFS=:`: `${e?$*}`
// says `e: a b c` in bash 5.3.20 and bash 3.2.57 — the word taken as fields
// and the sentence made of them with a space between — where zsh 5.9.2,
// ksh93u+ and dash 0.5.12 all say `e: a:b:c`, the value. One spelling, two
// readings. This preset answers bash's, which is what it answered before the
// assigning and default words stopped taking that route.
func TestTheDiagnosticWordKeepsTheSpaceJoin(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `set -- 'a:b' c; IFS=:; echo ${e?$*}`)
	if !strings.Contains(out, "e: a b c") {
		t.Errorf("said %q, want a sentence holding %q", out, "e: a b c")
	}
	if st == 0 {
		t.Errorf("status 0, want a failure")
	}
}
