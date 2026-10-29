// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

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
// dash 0.5.12 under `env -i PATH=/usr/bin:/bin` with `-c`. This dialect records
// a prefix's values only for the trace, so the untraced rows were right
// already and pin that; under `set -x` a function calling itself through
// `v=$n f` wrote and stored the outer run's value in every frame (#6989). See
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
				"out v=1\n" +
				"out v=2\n" +
				"out v=x\n" +
				"top v=\n",
			status: 0,
		},
		{
			name: "traced",
			src:  "set -x; " + recursiveFunction,
			want: "+ n=3\n" +
				"+ v=x f\n" +
				"+ echo in v=x\n" +
				"in v=x\n" +
				"+ n=2\n" +
				"+ [ 2 -ge 0 ]\n" +
				"+ v=2 f\n" +
				"+ echo in v=2\n" +
				"in v=2\n" +
				"+ n=1\n" +
				"+ [ 1 -ge 0 ]\n" +
				"+ v=1 f\n" +
				"+ echo in v=1\n" +
				"in v=1\n" +
				"+ n=0\n" +
				"+ [ 0 -ge 0 ]\n" +
				"+ v=0 f\n" +
				"+ echo in v=0\n" +
				"in v=0\n" +
				"+ n=-1\n" +
				"+ [ -1 -ge 0 ]\n" +
				"+ echo out v=0\n" +
				"out v=0\n" +
				"+ echo out v=1\n" +
				"out v=1\n" +
				"+ echo out v=2\n" +
				"out v=2\n" +
				"+ echo out v=x\n" +
				"out v=x\n" +
				"+ echo top v=\n" +
				"top v=\n",
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
				"+ v=3 eval echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f\n" +
				"+ echo eval v=3\n" +
				"eval v=3\n" +
				"+ n=2\n" +
				"+ [ 2 -ge 0 ]\n" +
				"+ f\n" +
				"+ v=2 eval echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f\n" +
				"+ echo eval v=2\n" +
				"eval v=2\n" +
				"+ n=1\n" +
				"+ [ 1 -ge 0 ]\n" +
				"+ f\n" +
				"+ v=1 eval echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f\n" +
				"+ echo eval v=1\n" +
				"eval v=1\n" +
				"+ n=0\n" +
				"+ [ 0 -ge 0 ]\n" +
				"+ f\n" +
				"+ v=0 eval echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f\n" +
				"+ echo eval v=0\n" +
				"eval v=0\n" +
				"+ n=-1\n" +
				"+ [ -1 -ge 0 ]\n" +
				"+ echo after v=0\n" +
				"after v=0\n" +
				"+ echo after v=0\n" +
				"after v=0\n" +
				"+ echo after v=0\n" +
				"after v=0\n" +
				"+ echo after v=0\n" +
				"after v=0\n" +
				"+ echo top v=0\n" +
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
				"after v=1\n" +
				"after v=2\n" +
				"after v=3\n" +
				"after v=\n" +
				"top v=\n",
			status: 0,
		},
		{
			name: "a regular builtin, traced",
			src:  "set -x; " + recursiveRegularBuiltin,
			want: "+ n=3\n" +
				"+ f\n" +
				"+ v=3 command eval echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f\n" +
				"+ echo eval v=3\n" +
				"eval v=3\n" +
				"+ n=2\n" +
				"+ [ 2 -ge 0 ]\n" +
				"+ f\n" +
				"+ v=2 command eval echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f\n" +
				"+ echo eval v=2\n" +
				"eval v=2\n" +
				"+ n=1\n" +
				"+ [ 1 -ge 0 ]\n" +
				"+ f\n" +
				"+ v=1 command eval echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f\n" +
				"+ echo eval v=1\n" +
				"eval v=1\n" +
				"+ n=0\n" +
				"+ [ 0 -ge 0 ]\n" +
				"+ f\n" +
				"+ v=0 command eval echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f\n" +
				"+ echo eval v=0\n" +
				"eval v=0\n" +
				"+ n=-1\n" +
				"+ [ -1 -ge 0 ]\n" +
				"+ echo after v=1\n" +
				"after v=1\n" +
				"+ echo after v=2\n" +
				"after v=2\n" +
				"+ echo after v=3\n" +
				"after v=3\n" +
				"+ echo after v=\n" +
				"after v=\n" +
				"+ echo top v=\n" +
				"top v=\n",
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
				"h after v=h1\n" +
				"h after v=h0\n" +
				"top v=\n",
			status: 0,
		},
		{
			name: "two functions sharing one call site, traced",
			src:  "set -x; " + twoFunctionsOneCallSite,
			want: "+ d=0\n" +
				"+ run\n" +
				"+ v=g0 g\n" +
				"+ echo g v=g0\n" +
				"g v=g0\n" +
				"+ v=h0 h\n" +
				"+ echo h v=h0\n" +
				"h v=h0\n" +
				"+ d=1\n" +
				"+ [ 1 -lt 3 ]\n" +
				"+ run\n" +
				"+ v=g1 g\n" +
				"+ echo g v=g1\n" +
				"g v=g1\n" +
				"+ v=h1 h\n" +
				"+ echo h v=h1\n" +
				"h v=h1\n" +
				"+ d=2\n" +
				"+ [ 2 -lt 3 ]\n" +
				"+ run\n" +
				"+ v=g2 g\n" +
				"+ echo g v=g2\n" +
				"g v=g2\n" +
				"+ v=h2 h\n" +
				"+ echo h v=h2\n" +
				"h v=h2\n" +
				"+ d=3\n" +
				"+ [ 3 -lt 3 ]\n" +
				"+ echo h after v=h2\n" +
				"h after v=h2\n" +
				"+ echo h after v=h1\n" +
				"h after v=h1\n" +
				"+ echo h after v=h0\n" +
				"h after v=h0\n" +
				"+ echo top v=\n" +
				"top v=\n",
			status: 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runDash(t, t.TempDir(), c.src)
			if out != c.want || st != c.status {
				t.Errorf("got %q status %d, want %q status %d", out, st, c.want, c.status)
			}
		})
	}
}
