// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

const recursiveFunction = `f() { echo "in v=$v"; n=$((n-1)); [ "$n" -ge 0 ] && v=$n f; echo "out v=$v"; ` +
	`}; n=3; v=x f; echo "top v=$v"`

const recursiveSpecialBuiltin = `f() { v=$n eval 'echo "eval v=$v"; n=$((n-1)); [ "$n" -ge 0 ] && f'; ` +
	`echo "after v=$v"; }; n=3; f; echo "top v=$v"`

const recursiveRegularBuiltin = `f() { v=$n builtin eval 'echo "eval v=$v"; n=$((n-1)); [ "$n" -ge 0 ] && f'; ` +
	`echo "after v=$v"; }; n=3; f; echo "top v=$v"`

const twoFunctionsOneCallSite = `g() { echo "g v=$v"; }; h() { echo "h v=$v"; d=$((d+1)); ` +
	`[ "$d" -lt 3 ] && run; echo "h after v=$v"; }; run() { for fn in g h; ` +
	`do v=$fn$d $fn; done; }; d=0; run; echo "top v=$v"`

// A command's assignment prefix is expanded again in every run of the command,
// however many runs of it are under way at once. Measured 2026-10-29 against
// zsh 5.9.2 under `env -i PATH=/usr/bin:/bin` with `-c`. This dialect records a
// call's prefix values only for the trace, so the untraced rows were right already
// and pin that; under `set -x` a function calling itself through `v=$n f` wrote
// and stored the outer run's value in every frame (#6989). The regular-builtin
// row is untraced because its line leaves out `builtin` for a reason of its own
// (#6967).
//
// The last two rows are the same mechanism by other routes: the walk in front
// of a declaration's operand recorded before the records were taken aside, so
// a loop ran the substitution once; and an element literal whose own words
// re-enter its assignment through qualifier code lost its words from the open
// trace line, because the inner run cleared the literal the outer one was
// waiting on. See interp/commandrecords.go.
func TestAPrefixIsExpandedInEveryRunOfItsCommand(t *testing.T) {
	t.Parallel()
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
			want: "+zsh:1> n=3 \n" +
				"+zsh:1> v=x +zsh:1> f\n" +
				"+f:0> echo 'in v=x'\n" +
				"in v=x\n" +
				"+f:0> n=2 \n" +
				"+f:0> [ 2 -ge 0 ']'\n" +
				"+f:0> v=2 +f:0> f\n" +
				"+f:0> echo 'in v=2'\n" +
				"in v=2\n" +
				"+f:0> n=1 \n" +
				"+f:0> [ 1 -ge 0 ']'\n" +
				"+f:0> v=1 +f:0> f\n" +
				"+f:0> echo 'in v=1'\n" +
				"in v=1\n" +
				"+f:0> n=0 \n" +
				"+f:0> [ 0 -ge 0 ']'\n" +
				"+f:0> v=0 +f:0> f\n" +
				"+f:0> echo 'in v=0'\n" +
				"in v=0\n" +
				"+f:0> n=-1 \n" +
				"+f:0> [ -1 -ge 0 ']'\n" +
				"+f:0> echo 'out v=0'\n" +
				"out v=0\n" +
				"+f:0> echo 'out v=1'\n" +
				"out v=1\n" +
				"+f:0> echo 'out v=2'\n" +
				"out v=2\n" +
				"+f:0> echo 'out v=x'\n" +
				"out v=x\n" +
				"+zsh:1> echo 'top v='\n" +
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
			want: "+zsh:1> n=3 \n" +
				"+zsh:1> f\n" +
				"+f:0> v=3 +f:0> eval 'echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f'\n" +
				"+(eval):1> echo 'eval v=3'\n" +
				"eval v=3\n" +
				"+(eval):1> n=2 \n" +
				"+(eval):1> [ 2 -ge 0 ']'\n" +
				"+(eval):1> f\n" +
				"+f:0> v=2 +f:0> eval 'echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f'\n" +
				"+(eval):1> echo 'eval v=2'\n" +
				"eval v=2\n" +
				"+(eval):1> n=1 \n" +
				"+(eval):1> [ 1 -ge 0 ']'\n" +
				"+(eval):1> f\n" +
				"+f:0> v=1 +f:0> eval 'echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f'\n" +
				"+(eval):1> echo 'eval v=1'\n" +
				"eval v=1\n" +
				"+(eval):1> n=0 \n" +
				"+(eval):1> [ 0 -ge 0 ']'\n" +
				"+(eval):1> f\n" +
				"+f:0> v=0 +f:0> eval 'echo \"eval v=$v\"; n=$((n-1)); [ \"$n\" -ge 0 ] && f'\n" +
				"+(eval):1> echo 'eval v=0'\n" +
				"eval v=0\n" +
				"+(eval):1> n=-1 \n" +
				"+(eval):1> [ -1 -ge 0 ']'\n" +
				"+f:0> echo 'after v=1'\n" +
				"after v=1\n" +
				"+f:0> echo 'after v=2'\n" +
				"after v=2\n" +
				"+f:0> echo 'after v=3'\n" +
				"after v=3\n" +
				"+f:0> echo 'after v='\n" +
				"after v=\n" +
				"+zsh:1> echo 'top v='\n" +
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
			want: "+zsh:1> d=0 \n" +
				"+zsh:1> run\n" +
				"+run:0> fn=g\n" +
				"+run:0> v=g0 +run:0> g\n" +
				"+g:0> echo 'g v=g0'\n" +
				"g v=g0\n" +
				"+run:0> fn=h\n" +
				"+run:0> v=h0 +run:0> h\n" +
				"+h:0> echo 'h v=h0'\n" +
				"h v=h0\n" +
				"+h:0> d=1 \n" +
				"+h:0> [ 1 -lt 3 ']'\n" +
				"+h:0> run\n" +
				"+run:0> fn=g\n" +
				"+run:0> v=g1 +run:0> g\n" +
				"+g:0> echo 'g v=g1'\n" +
				"g v=g1\n" +
				"+run:0> fn=h\n" +
				"+run:0> v=h1 +run:0> h\n" +
				"+h:0> echo 'h v=h1'\n" +
				"h v=h1\n" +
				"+h:0> d=2 \n" +
				"+h:0> [ 2 -lt 3 ']'\n" +
				"+h:0> run\n" +
				"+run:0> fn=g\n" +
				"+run:0> v=g2 +run:0> g\n" +
				"+g:0> echo 'g v=g2'\n" +
				"g v=g2\n" +
				"+run:0> fn=h\n" +
				"+run:0> v=h2 +run:0> h\n" +
				"+h:0> echo 'h v=h2'\n" +
				"h v=h2\n" +
				"+h:0> d=3 \n" +
				"+h:0> [ 3 -lt 3 ']'\n" +
				"+h:0> echo 'h after v=h2'\n" +
				"h after v=h2\n" +
				"+h:0> echo 'h after v=h1'\n" +
				"h after v=h1\n" +
				"+h:0> echo 'h after v=h0'\n" +
				"h after v=h0\n" +
				"+zsh:1> echo 'top v='\n" +
				"top v=\n",
			status: 0,
		},
		{
			name: "a loop over a declaration",
			src: `for i in 1 2 3; do v=$(echo "sub $i" >&2; echo $i) typeset x=1; ` +
				`echo "after v=$v"; done`,
			want: "sub 1\n" +
				"after v=\n" +
				"sub 2\n" +
				"after v=\n" +
				"sub 3\n" +
				"after v=\n",
			status: 0,
		},
		{
			name: "an element literal through qualifier code",
			src: "a=(1 2); n=2\ng() { (( n-- > 0 )) || return 0; set -x; " +
				"a[1]=( q$n /bin/s[h](Ne:'g':) ); set +x; print -r -- \"a=${a[*]}\" }\ng",
			want: "+g:0> a[1]=+g:0> g\n" +
				"+g:0> ((  n-- > 0  ))\n" +
				"+g:0> set -x\n" +
				"+g:0> a[1]=+g:0> g\n" +
				"+g:0> ((  n-- > 0  ))\n" +
				"+g:0> return 0\n" +
				"( q0 /bin/sh ) \n" +
				"+g:0> set +x\n" +
				"a=q0 /bin/sh 2\n" +
				"( q1 /bin/sh ) \n" +
				"+g:0> set +x\n" +
				"a=q1 /bin/sh /bin/sh 2\n",
			status: 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != c.want || st != c.status {
				t.Errorf("got %q status %d, want %q status %d", out, st, c.want, c.status)
			}
		})
	}
}
