// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

const recursiveFunction = `f() { echo "in v=$v"; n=$((n-1)); [ "$n" -ge 0 ] && v=$n f; echo "out v=$v"; ` +
	`}; n=3; v=x f; echo "top v=$v"`

const recursiveSpecialBuiltin = `f() { v=$n eval 'echo "eval v=$v"; n=$((n-1)); [ "$n" -ge 0 ] && f'; ` +
	`echo "after v=$v"; }; n=3; f; echo "top v=$v"`

const recursiveRegularBuiltin = `f() { v=$n command eval 'echo "eval v=$v"; n=$((n-1)); [ "$n" -ge 0 ] && f'; ` +
	`echo "after v=$v"; }; n=3; f; echo "top v=$v"`

const twoFunctionsOneCallSite = `g() { echo "g v=$v"; }; h() { echo "h v=$v"; d=$((d+1)); ` +
	`[ "$d" -lt 3 ] && run; echo "h after v=$v"; }; run() { for fn in g h; ` +
	`do v=$fn$d $fn; done; }; d=0; run; echo "top v=$v"`

// A command's assignment prefix is expanded again in every run of the command,
// however many runs of it are under way at once. Measured 2026-10-29 against
// ksh93u+ 2012-08-01 (`/bin/ksh`) under `env -i PATH=/usr/bin:/bin` with `-c`: a
// function that calls itself through `v=$n f` hands each frame its own `$n`,
// and so does a builtin whose prefix recursion goes through, traced or not.
// This shell found the outer run's value under the same assignment node and
// handed it on (#6989). A traced `command eval` row is left out: its lines are
// in the wrong order for a reason of their own (#6984).
//
// The last two rows are the same records outliving their command with no
// recursion at all: the walk in front of a declaration's operand recorded
// before the records were taken aside, so a loop ran the substitution once;
// and the speaker one command chose for a failing value was left for the next,
// which located line 2's failure the builtin's way. See
// interp/commandrecords.go.
func TestAPrefixIsExpandedInEveryRunOfItsCommand(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{
			name: "a function, four calls deep",
			src:  recursiveFunction,
			want: "in v=x\n" +
				"in v=2\n" +
				"in v=1\n" +
				"in v=0\n" +
				"out v=0\n" +
				"out v=0\n" +
				"out v=0\n" +
				"out v=0\n" +
				"top v=0\n",
			status: 0,
		},
		{
			name: "traced",
			src:  "set -x; " + recursiveFunction,
			want: "+ n=3\n" +
				"+ v=x\n" +
				"+ f\n" +
				"+ echo 'in v=x'\n" +
				"in v=x\n" +
				"+ n=2\n" +
				"+ [ 2 -ge 0 ]\n" +
				"+ v=2\n" +
				"+ f\n" +
				"+ echo 'in v=2'\n" +
				"in v=2\n" +
				"+ n=1\n" +
				"+ [ 1 -ge 0 ]\n" +
				"+ v=1\n" +
				"+ f\n" +
				"+ echo 'in v=1'\n" +
				"in v=1\n" +
				"+ n=0\n" +
				"+ [ 0 -ge 0 ]\n" +
				"+ v=0\n" +
				"+ f\n" +
				"+ echo 'in v=0'\n" +
				"in v=0\n" +
				"+ n=-1\n" +
				"+ [ -1 -ge 0 ]\n" +
				"+ echo 'out v=0'\n" +
				"out v=0\n" +
				"+ echo 'out v=0'\n" +
				"out v=0\n" +
				"+ echo 'out v=0'\n" +
				"out v=0\n" +
				"+ echo 'out v=0'\n" +
				"out v=0\n" +
				"+ echo 'top v=0'\n" +
				"top v=0\n",
			status: 0,
		},
		{
			name: "a special builtin",
			src:  recursiveSpecialBuiltin,
			want: "eval v=3\n" +
				"eval v=2\n" +
				"eval v=1\n" +
				"eval v=0\n" +
				"after v=0\n" +
				"after v=0\n" +
				"after v=0\n" +
				"after v=0\n" +
				"top v=0\n",
			status: 0,
		},
		{
			name: "a special builtin, traced",
			src:  "set -x; " + recursiveSpecialBuiltin,
			want: "+ n=3\n" +
				"+ f\n" +
				"+ v=3\n" +
				"+ eval 'echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f'\n" +
				"+ echo 'eval v=3'\n" +
				"eval v=3\n" +
				"+ n=2\n" +
				"+ [ 2 -ge 0 ]\n" +
				"+ f\n" +
				"+ v=2\n" +
				"+ eval 'echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f'\n" +
				"+ echo 'eval v=2'\n" +
				"eval v=2\n" +
				"+ n=1\n" +
				"+ [ 1 -ge 0 ]\n" +
				"+ f\n" +
				"+ v=1\n" +
				"+ eval 'echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f'\n" +
				"+ echo 'eval v=1'\n" +
				"eval v=1\n" +
				"+ n=0\n" +
				"+ [ 0 -ge 0 ]\n" +
				"+ f\n" +
				"+ v=0\n" +
				"+ eval 'echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f'\n" +
				"+ echo 'eval v=0'\n" +
				"eval v=0\n" +
				"+ n=-1\n" +
				"+ [ -1 -ge 0 ]\n" +
				"+ echo 'after v=0'\n" +
				"after v=0\n" +
				"+ echo 'after v=0'\n" +
				"after v=0\n" +
				"+ echo 'after v=0'\n" +
				"after v=0\n" +
				"+ echo 'after v=0'\n" +
				"after v=0\n" +
				"+ echo 'top v=0'\n" +
				"top v=0\n",
			status: 0,
		},
		{
			name: "a regular builtin",
			src:  recursiveRegularBuiltin,
			want: "eval v=3\n" +
				"eval v=2\n" +
				"eval v=1\n" +
				"eval v=0\n" +
				"after v=0\n" +
				"after v=0\n" +
				"after v=0\n" +
				"after v=0\n" +
				"top v=0\n",
			status: 0,
		},
		{
			name: "two functions sharing one call site",
			src:  twoFunctionsOneCallSite,
			want: "g v=g0\n" +
				"h v=h0\n" +
				"g v=g1\n" +
				"h v=h1\n" +
				"g v=g2\n" +
				"h v=h2\n" +
				"h after v=h2\n" +
				"h after v=h2\n" +
				"h after v=h2\n" +
				"top v=h2\n",
			status: 0,
		},
		{
			name: "two functions sharing one call site, traced",
			src:  "set -x; " + twoFunctionsOneCallSite,
			want: "+ d=0\n" +
				"+ run\n" +
				"+ v=g0\n" +
				"+ g\n" +
				"+ echo 'g v=g0'\n" +
				"g v=g0\n" +
				"+ v=h0\n" +
				"+ h\n" +
				"+ echo 'h v=h0'\n" +
				"h v=h0\n" +
				"+ d=1\n" +
				"+ [ 1 -lt 3 ]\n" +
				"+ run\n" +
				"+ v=g1\n" +
				"+ g\n" +
				"+ echo 'g v=g1'\n" +
				"g v=g1\n" +
				"+ v=h1\n" +
				"+ h\n" +
				"+ echo 'h v=h1'\n" +
				"h v=h1\n" +
				"+ d=2\n" +
				"+ [ 2 -lt 3 ]\n" +
				"+ run\n" +
				"+ v=g2\n" +
				"+ g\n" +
				"+ echo 'g v=g2'\n" +
				"g v=g2\n" +
				"+ v=h2\n" +
				"+ h\n" +
				"+ echo 'h v=h2'\n" +
				"h v=h2\n" +
				"+ d=3\n" +
				"+ [ 3 -lt 3 ]\n" +
				"+ echo 'h after v=h2'\n" +
				"h after v=h2\n" +
				"+ echo 'h after v=h2'\n" +
				"h after v=h2\n" +
				"+ echo 'h after v=h2'\n" +
				"h after v=h2\n" +
				"+ echo 'top v=h2'\n" +
				"top v=h2\n",
			status: 0,
		},
		{
			name: "a loop over a declaration",
			src: `for i in 1 2 3; do v=$(echo "sub $i" >&2; echo $i) typeset x=1; ` +
				`echo "after v=$v"; done`,
			want: "sub 1\n" +
				"after v=1\n" +
				"sub 2\n" +
				"after v=2\n" +
				"sub 3\n" +
				"after v=3\n",
			status: 0,
		},
		{
			name: "the next command chooses its own speaker",
			src:  "b=$((1)) echo hi\na=$((1/0)) typeset x=1\necho \"next st=$?\"",
			want: "hi\n" +
				"ksh: line 2: 1/0: divide by zero\n",
			status: 1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if out != c.want || st != c.status {
				t.Errorf("got %q status %d, want %q status %d", out, st, c.want, c.status)
			}
		})
	}
}
