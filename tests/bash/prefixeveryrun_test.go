// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

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
// bash 5.3.20 under `env -i PATH=/usr/bin:/bin` with `-c`: a function that calls
// itself through `v=$n f` hands each frame its own `$n`, and so does a builtin
// whose prefix recursion goes through, traced or not. This shell found the
// outer run's value under the same assignment node and handed it on (#6989).
//
// The last three rows are the same records outliving their command with no
// recursion at all: the walk in front of a frozen name recorded before the
// records were taken aside, so the next pass of a loop read the first one's
// value; and a command run inside a value cleared the outer command's
// "already refused" flag, so its refusal was written a second time. See
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
				"+ v=x\n" +
				"+ f\n" +
				"+ echo 'in v=x'\n" +
				"in v=x\n" +
				"+ n=2\n" +
				"+ '[' 2 -ge 0 ']'\n" +
				"+ v=2\n" +
				"+ f\n" +
				"+ echo 'in v=2'\n" +
				"in v=2\n" +
				"+ n=1\n" +
				"+ '[' 1 -ge 0 ']'\n" +
				"+ v=1\n" +
				"+ f\n" +
				"+ echo 'in v=1'\n" +
				"in v=1\n" +
				"+ n=0\n" +
				"+ '[' 0 -ge 0 ']'\n" +
				"+ v=0\n" +
				"+ f\n" +
				"+ echo 'in v=0'\n" +
				"in v=0\n" +
				"+ n=-1\n" +
				"+ '[' -1 -ge 0 ']'\n" +
				"+ echo 'out v=0'\n" +
				"out v=0\n" +
				"+ echo 'out v=1'\n" +
				"out v=1\n" +
				"+ echo 'out v=2'\n" +
				"out v=2\n" +
				"+ echo 'out v=x'\n" +
				"out v=x\n" +
				"+ echo 'top v='\n" +
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
				"after v=1\n" +
				"after v=2\n" +
				"after v=3\n" +
				"after v=\n" +
				"top v=\n",
			status: 0,
		},
		{
			name: "a special builtin, traced",
			src:  "set -x; " + recursiveSpecialBuiltin,
			want: "+ n=3\n" +
				"+ f\n" +
				"+ v=3\n" +
				"+ eval 'echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f'\n" +
				"++ echo 'eval v=3'\n" +
				"eval v=3\n" +
				"++ n=2\n" +
				"++ '[' 2 -ge 0 ']'\n" +
				"++ f\n" +
				"++ v=2\n" +
				"++ eval 'echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f'\n" +
				"+++ echo 'eval v=2'\n" +
				"eval v=2\n" +
				"+++ n=1\n" +
				"+++ '[' 1 -ge 0 ']'\n" +
				"+++ f\n" +
				"+++ v=1\n" +
				"+++ eval 'echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f'\n" +
				"++++ echo 'eval v=1'\n" +
				"eval v=1\n" +
				"++++ n=0\n" +
				"++++ '[' 0 -ge 0 ']'\n" +
				"++++ f\n" +
				"++++ v=0\n" +
				"++++ eval 'echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f'\n" +
				"+++++ echo 'eval v=0'\n" +
				"eval v=0\n" +
				"+++++ n=-1\n" +
				"+++++ '[' -1 -ge 0 ']'\n" +
				"++++ echo 'after v=1'\n" +
				"after v=1\n" +
				"+++ echo 'after v=2'\n" +
				"after v=2\n" +
				"++ echo 'after v=3'\n" +
				"after v=3\n" +
				"+ echo 'after v='\n" +
				"after v=\n" +
				"+ echo 'top v='\n" +
				"top v=\n",
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
				"+ v=3\n" +
				"+ command eval 'echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f'\n" +
				"++ echo 'eval v=3'\n" +
				"eval v=3\n" +
				"++ n=2\n" +
				"++ '[' 2 -ge 0 ']'\n" +
				"++ f\n" +
				"++ v=2\n" +
				"++ command eval 'echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f'\n" +
				"+++ echo 'eval v=2'\n" +
				"eval v=2\n" +
				"+++ n=1\n" +
				"+++ '[' 1 -ge 0 ']'\n" +
				"+++ f\n" +
				"+++ v=1\n" +
				"+++ command eval 'echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f'\n" +
				"++++ echo 'eval v=1'\n" +
				"eval v=1\n" +
				"++++ n=0\n" +
				"++++ '[' 0 -ge 0 ']'\n" +
				"++++ f\n" +
				"++++ v=0\n" +
				"++++ command eval 'echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f'\n" +
				"+++++ echo 'eval v=0'\n" +
				"eval v=0\n" +
				"+++++ n=-1\n" +
				"+++++ '[' -1 -ge 0 ']'\n" +
				"++++ echo 'after v=1'\n" +
				"after v=1\n" +
				"+++ echo 'after v=2'\n" +
				"after v=2\n" +
				"++ echo 'after v=3'\n" +
				"after v=3\n" +
				"+ echo 'after v='\n" +
				"after v=\n" +
				"+ echo 'top v='\n" +
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
				"+ for fn in g h\n" +
				"+ v=g0\n" +
				"+ g\n" +
				"+ echo 'g v=g0'\n" +
				"g v=g0\n" +
				"+ for fn in g h\n" +
				"+ v=h0\n" +
				"+ h\n" +
				"+ echo 'h v=h0'\n" +
				"h v=h0\n" +
				"+ d=1\n" +
				"+ '[' 1 -lt 3 ']'\n" +
				"+ run\n" +
				"+ for fn in g h\n" +
				"+ v=g1\n" +
				"+ g\n" +
				"+ echo 'g v=g1'\n" +
				"g v=g1\n" +
				"+ for fn in g h\n" +
				"+ v=h1\n" +
				"+ h\n" +
				"+ echo 'h v=h1'\n" +
				"h v=h1\n" +
				"+ d=2\n" +
				"+ '[' 2 -lt 3 ']'\n" +
				"+ run\n" +
				"+ for fn in g h\n" +
				"+ v=g2\n" +
				"+ g\n" +
				"+ echo 'g v=g2'\n" +
				"g v=g2\n" +
				"+ for fn in g h\n" +
				"+ v=h2\n" +
				"+ h\n" +
				"+ echo 'h v=h2'\n" +
				"h v=h2\n" +
				"+ d=3\n" +
				"+ '[' 3 -lt 3 ']'\n" +
				"+ echo 'h after v=h2'\n" +
				"h after v=h2\n" +
				"+ echo 'h after v=h1'\n" +
				"h after v=h1\n" +
				"+ echo 'h after v=h0'\n" +
				"h after v=h0\n" +
				"+ echo 'top v='\n" +
				"top v=\n",
			status: 0,
		},
		{
			name: "a loop past a frozen name",
			src: `readonly r=1; f() { echo "B=$B"; }; for i in 1 2 3; do B=$i r=2 f; done; ` +
				`echo "top B=$B"`,
			want: "bash: line 1: r: readonly variable\n" +
				"B=1\n" +
				"bash: line 1: r: readonly variable\n" +
				"B=2\n" +
				"bash: line 1: r: readonly variable\n" +
				"B=3\n" +
				"top B=\n",
			status: 0,
		},
		{
			name: "a loop past a frozen name, traced",
			src:  `readonly r=1; f() { echo "B=$B"; }; set -x; for i in 1 2; do B=$i r=2 f; done`,
			want: "+ for i in 1 2\n" +
				"+ B=1\n" +
				"bash: line 1: r: readonly variable\n" +
				"+ f\n" +
				"+ echo B=1\n" +
				"B=1\n" +
				"+ for i in 1 2\n" +
				"+ B=2\n" +
				"bash: line 1: r: readonly variable\n" +
				"+ f\n" +
				"+ echo B=2\n" +
				"B=2\n",
			status: 0,
		},
		{
			name: "a command inside the value does not take the refusal",
			src: "readonly r=1; f() { echo \"f B=$B\"; }\nr=2 B=${ r=3 /bin/echo inner >&2; " +
				"echo b; } f\necho \"st=$?\"",
			want: "bash: line 2: r: readonly variable\n" +
				"bash: line 2: r: readonly variable\n" +
				"inner\n" +
				"f B=b\n" +
				"st=0\n",
			status: 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), c.src)
			if out != c.want || st != c.status {
				t.Errorf("got %q status %d, want %q status %d", out, st, c.want, c.status)
			}
		})
	}
}
