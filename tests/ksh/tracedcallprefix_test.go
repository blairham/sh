// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A prefix in front of a `function`-form function marked `typeset -ft` is
// worked through twice: ahead of the call as the shell's own, persisting, the
// way a POSIX-form call's is, and then again as the call's own cells, expanded
// a second time and written behind the command's line. Measured 2026-11-01
// against ksh93u+ 2012-08-01 (`/bin/ksh`), from script files and from `-c`
// under `env -i PATH=/usr/bin:/bin`. This shell made the front pass alone and
// stored it as the call's (#7092).
//
// The controls are the two calls that work the prefix once: the untraced
// `function`-form call, whose prefix is the call's own and is written behind
// it, and a POSIX-form function marked `-ft`, whose prefix is the shell's and
// is written ahead. See Semantics.PrefixToATracedKeywordFunctionIsWorkedTwice.
func TestATracedCallWorksItsPrefixTwice(t *testing.T) {
	t.Parallel()
	const g = `function g { echo in; }; typeset -ft g; `
	const v = `function g { echo "in x=$x y=$y"; }; typeset -ft g; `
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{"one entry", g + `set -x; x=1 g`, "+ x=1\n+ g\n+ x=1\n+ echo in\nin\n", 0},
		{"two entries", g + `set -x; x=1 y=2 g`, "+ x=1\n+ y=2\n+ g\n+ x=1\n+ y=2\n+ echo in\nin\n", 0},
		{
			"a substitution in the value",
			g + `set -x; x=$(echo s >&2) g`,
			"+ echo s\n+ 1>& 2\ns\n+ x=''\n+ g\n+ echo s\n+ 1>& 2\ns\n+ x=''\n+ echo in\nin\n",
			0,
		},
		{"an element", g + `set -x; b=(1 2); b[1]=z g`, "+ b=( 1 2 )\n+ b[1]=z\n+ g\n+ b[1]=z\n+ echo in\nin\n", 0},
		{"behind a redirection that takes the line", g + `set -x; x=1 g 2>/dev/null`, "+ x=1\n+ g\n+ 2> /dev/null\nin\n", 0},
		// Not a trace effect: the substitution runs twice untraced.
		{"expanded twice without set -x", g + `x=$(echo s >&2) g`, "s\ns\n+ echo in\nin\n", 0},
		{"the second pass is behind the redirections", g + `x=$(echo s >&2) g 2>/dev/null`, "s\nin\n", 0},
		{"the front pass persists", v + `x=1 g; echo "after x=$x"`, "+ echo 'in x=1 y='\nin x=1 y=\nafter x=1\n", 0},
		{
			"each pass expands the value",
			v + `n=0; x=$((n+=1)) y=$((n+=10)) g; echo "after n=$n x=$x y=$y"`,
			"+ echo 'in x=12 y=22'\nin x=12 y=22\nafter n=22 x=1 y=11\n",
			0,
		},
		{"the call's own cell is fresh", v + `x=old; x+=a g; echo "after x=$x"`, "+ echo 'in x=a y='\nin x=a y=\nafter x=olda\n", 0},
		{
			"the second pass reads the first one's stores",
			v + `x=old; y=$x x=new g; echo "after x=$x y=$y"`,
			"+ echo 'in x=new y=new'\nin x=new y=new\nafter x=new y=old\n",
			0,
		},
		{
			"and is written behind the line",
			v + `x=old; set -x; y=$x x=new g`,
			"+ y=old\n+ x=new\n+ g\n+ y=new\n+ x=new\n+ echo 'in x=new y=new'\nin x=new y=new\n",
			0,
		},
		{"the front pass takes the export off", v + `export x=old; x=new g; typeset -p x`, "+ echo 'in x=new y='\nin x=new y=\nx=new\n", 0},
		{
			"each pass holds its entries with its own export",
			v + `x=1 y=$(/usr/bin/printenv x) g; echo "after y=$y"`,
			"+ echo 'in x=1 y=1'\nin x=1 y=1\nafter y=\n",
			0,
		},
		{
			"a second pass that will not expand",
			v + `n=0; x=$(( n++ ? 1/0 : 1 )) g; echo after`,
			"ksh:  n++ ? 1/0 : 1 : divide by zero\n",
			1,
		},
		// Controls.
		{"an untraced call", `function g { echo in; }; set -x; x=1 g`, "+ g\n+ x=1\nin\n", 0},
		{"an untraced call untraced", `function g { echo in; }; x=$(echo s >&2) g`, "s\nin\n", 0},
		{
			"an untraced call keeps nothing",
			`function g { echo "in x=$x"; }; x=1 g; echo "after x=$x"`,
			"in x=1\nafter x=\n",
			0,
		},
		{"a POSIX-form function marked -ft", `g() { echo in; }; typeset -ft g; set -x; x=1 g`, "+ x=1\n+ g\n+ echo in\nin\n", 0},
		{"a POSIX-form function marked -ft untraced", `g() { echo in; }; typeset -ft g; x=$(echo s >&2) g`, "s\nin\n", 0},
		{"a mark taken off", `function g { echo in; }; typeset -ft g; typeset +ft g; set -x; x=1 g`, "+ g\n+ x=1\nin\n", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, st := runKsh(t, t.TempDir(), c.src)
			if out != c.want || st != c.status {
				t.Errorf("got %q (%d), want %q (%d)", out, st, c.want, c.status)
			}
		})
	}
}
