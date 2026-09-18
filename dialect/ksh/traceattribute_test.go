// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// The `t` letter of a declaration in ksh93's column. The letter was accepted
// here and recorded nowhere, so the declaration landed and the listing wrote
// the plain form — a letter taken and dropped on the floor, which is the
// defect #3101 records.
//
// Measured 2026-09-18 on ksh93u+ 2012-08-01 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME.

func TestTheTraceLetterIsRecordedAndWrittenAsAWordOfItsOwn(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, row := range []struct{ src, want string }{
		{`typeset -t T=1; typeset -p T`, "typeset -t T=1\n"},
		{`Z=9; typeset -t Z; typeset -p Z`, "typeset -t Z=9\n"},
		{`typeset -t T=1; typeset +t T; typeset -p T`, "T=1\n"},
		// Behind export and readonly, in front of the kind letter and in
		// front of everything the kind letter is followed by.
		{
			`typeset -a B; B=(1 2); typeset -t B; typeset -p B`,
			"typeset -t -a B=(1 2)\n",
		},
		{
			`typeset -A M; M=([k]=v); typeset -t M; typeset -p M`,
			"typeset -t -A M=([k]=v)\n",
		},
		{`typeset -i I=5; typeset -t I; typeset -p I`, "typeset -t -i I=5\n"},
		{`typeset -u U=ab; typeset -t U; typeset -p U`, "typeset -t -u U=AB\n"},
		{`typeset -x X=1; typeset -t X; typeset -p X`, "typeset -x -t X=1\n"},
	} {
		out, st := runKsh(t, dir, row.src)
		if out != row.want || st != 0 {
			t.Errorf("%s\n got %q (status %d)\nwant %q", row.src, out, st, row.want)
		}
	}
}
