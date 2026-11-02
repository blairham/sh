// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// The empty-key warning a declaration's literal operand draws is located as
// the plain statement's, `p.sh: line 1:`, with an assignment prefix in front
// of the declaration as without one. Measured 2026-11-01 against ksh93u+
// 2012-08-01 (`/bin/ksh`), script files under `env -i PATH=/usr/bin:/bin` with
// a scratch HOME, byte for byte. The prefixed rows were located `p.sh[1]:`,
// the builtin's form, until #7076 moved the literal's trace lines to where the
// elements expand (#7081); nothing pinned them until now.
//
// The last row is the control: no prefix, the same location.
func TestAPrefixedDeclarationsEmptyKeyWarningIsLocatedAsTheStatement(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"one element",
			`set -x; v=1 typeset -A m=([""]=1)`,
			"+ v=1\np.sh: line 1: warning: adding empty subscript\n+ m['']=1\n+ typeset -A m\n",
		},
		{
			"two elements",
			`set -x; v=1 typeset -A m=([""]=1 [k]=2)`,
			"+ v=1\np.sh: line 1: warning: adding empty subscript\n+ m['']=1\n+ m[k]=2\n+ typeset -A m\n",
		},
		{
			"no prefix",
			`set -x; typeset -A m=([""]=1)`,
			"p.sh: line 1: warning: adding empty subscript\n+ m['']=1\n+ typeset -A m\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got, st := runKshFile(t, c.src+"\n"); got != c.want || st != 0 {
				t.Errorf("%q at %d, want %q at 0", got, st, c.want)
			}
		})
	}
}
