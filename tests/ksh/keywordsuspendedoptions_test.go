// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// Two options a keyword-defined body is not handed — #3860, where a script
// that runs to the end in ksh93 ended at the first failure here.
//
// #3308 gave the same word an option *scope*: what the body moves is put back
// at the return. This is the other half of that table and a restore cannot
// say it — `-e` and `-x` are turned off before the body runs, so the body
// never reads them at all. Every `want` below is AT&T ksh93u+ 2012-08-01's own
// output, measured 2026-09-20 over a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with standard input on the null device.
func TestAKeywordFunctionRunsWithoutErrexitAndXtrace(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			// The row that decides whether a script survives: the body's
			// failure is not fatal and the script runs on.
			name:   "errexit is off inside a keyword body",
			src:    "set -e\nfunction f { false; echo after; }\nf\necho tail\n",
			want:   "after\ntail\n",
			status: 0,
		},
		{
			// **The control.** The same body written the other way is the
			// ordinary `-e`, which is what keys the rule to the definition
			// form rather than to the call.
			name:   "and on inside a POSIX one",
			src:    "set -e\ng() { false; echo after; }\ng\necho tail\n",
			want:   "",
			status: 1,
		},
		{
			// It is a suspension and not a loss: the caller is under `-e`
			// again the moment the call returns.
			name:   "the caller has errexit back",
			src:    "set -e\nfunction f { false; echo after; }\nf\nfalse\necho unreached\n",
			want:   "after\n",
			status: 1,
		},
		{
			name:   "xtrace is off inside a keyword body",
			src:    "set -x\nfunction f { echo A; echo B; }\nf\necho tail\n",
			want:   "+ f\nA\nB\n+ echo tail\ntail\n",
			status: 0,
		},
		{
			// The control again, on the second option.
			name:   "and on inside a POSIX one",
			src:    "set -x\ng() { echo A; }\ng\n",
			want:   "+ g\n+ echo A\nA\n",
			status: 0,
		},
		{
			// What the body asks for is its own and does not leak out, which
			// is #3308's half answering on the same option.
			name:   "a body may turn xtrace on for itself",
			src:    "function f { set -x; echo A; }\nf\necho tail\n",
			want:   "+ echo A\nA\ntail\n",
			status: 0,
		},
		{
			// A function the body calls is not traced either, however it was
			// written: what was suspended is the option, for everything the
			// call reaches.
			name:   "a POSIX function the body calls",
			src:    "set -x\ng() { echo P; }\nfunction f { g; }\nf\n",
			want:   "+ f\nP\n",
			status: 0,
		},
		{
			// **The third control, and the one this shell gives the tracing
			// back through.** `typeset -ft` marks the name, and a marked body
			// traces with the option off — so a suspension reaching
			// Runner.xtraceByMark would pass every row above and fail this.
			name:   "a marked function still traces",
			src:    "function f { echo A; }\ntypeset -ft f\nf\necho tail\n",
			want:   "+ echo A\nA\ntail\n",
			status: 0,
		},
		{
			// And with the option on as well, where the mark is what the
			// body traces by.
			name:   "a marked function traces under set -x too",
			src:    "set -x\nfunction f { echo A; }\ntypeset -ft f\nf\n",
			want:   "+ typeset -ft f\n+ f\n+ echo A\nA\n",
			status: 0,
		},
		{
			// **The fourth control, and the widest.** Two options and not
			// the table: `-u` and `-f` are both in force inside the body.
			name:   "every other option is still the caller's",
			src:    "set -uf\nfunction f { echo \"[$-]\"; echo *; }\nf\n",
			want:   "[fhuB]\n*\n",
			status: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("out %q status %d, want %q status %d", out, st, tc.want, tc.status)
			}
		})
	}
}
