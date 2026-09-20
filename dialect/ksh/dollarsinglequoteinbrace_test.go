// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A `$'…'` written inside a `${…}` body keeps its escapes, so the script runs
// rather than dying at the end of the input (#3896).
//
// Measured 2026-09-20 from script files under `env -i PATH=/usr/bin:/bin
// LC_ALL=C`, every column that has the construct:
//
//	case                        bash 5.3.20  bash 3.2.57  zsh 5.9.2  ksh93u+
//	echo ${x:-$'\''}            '            '            '          '
//	echo ${x+$'\''}             (empty)      (empty)      (empty)    (empty)
//	x=abc; echo ${x#$'\''}      abc          abc          abc        abc
//	echo ${x:-$'a}b'}           a}b          a}b          a}b        a}b
//	echo ${x:-$'a\tb'}          a<TAB>b      a<TAB>b      a<TAB>b    a<TAB>b
//	echo ${x:-$'\'}             refused      refused      refused    refused
//
// Unanimous, so there is no axis here: this preset was simply wrong along
// with bash's and zsh's, and the script *terminated* — status 3 here,
// 2 in bash and 1 in zsh — where every reference printed the quote and carried on.
//
// The escapes themselves are the substrate's, and syntax's own
// TestADollarSingleQuoteInsideABraceBodyKeepsItsEscapes names the flag. What
// this file adds is that the value comes out and the next command runs.
func TestADollarSingleQuoteInsideABraceBodyRuns(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a default word", `printf "[%s]" ${x:-$'\''}`, `[']`},
		{"an alternate word", `printf "[%s]" ${x+$'\''}`, `[]`},
		{"a prefix pattern", `x=abc; printf "[%s]" ${x#$'\''}`, `[abc]`},
		// A `}` inside the run is the run's and not the expansion's, which
		// worked before the fix and must still: it is the control that says
		// what moved was the escape rather than where a quoted run ends.
		{"a brace inside the run", `printf "[%s]" ${x:-$'a}b'}`, `[a}b]`},
		// So was every escape that does not end the run early.
		{"an ordinary escape", `printf "[%s]" ${x:-$'a\tb'}`, "[a\tb]"},
		// And the script carries on afterwards, which is the half the issue
		// is about — the old failure was fatal, so nothing behind it ran.
		{"the script carries on", `printf "[%s]" ${x:-$'\''}; printf "[end]"`, `['][end]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("%q said %q (status %d), want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}
