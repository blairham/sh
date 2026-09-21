// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A subscript that reaches `unset` as **text** holds an expansion, and an
// expansion in it can run a command. Which route the text arrived by decides
// whether it runs, and `unset` is the route that looks the name up first: a
// name this shell has not got takes its brackets with it, so nothing in them
// is expanded at all (#4037).
//
// The shape is a script passing text it did not write into a subscript, and
// the difference is whether a command in that text runs — which is why this
// is a defect rather than an edge, and why the row that matters is the
// **absence** of the marker rather than the value of any variable.
//
// Measured 2026-09-21 on bash 5.3.20 and 3.2.57 alike, `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, with `$(echo INJECTION! >&2 ; echo 0)` as the
// subscript's text. The routes were enumerated from the shells rather than
// guessed at, and only two of them refuse:
//
//	unset "$v", nothing of that name         nothing runs, status 0
//	unset "$v", the name frozen              refused by name, nothing runs
//	unset "$v", `a=(x y z)`                  it runs, element 0 goes
//	unset "$v", `a` a scalar or a table      it runs
//	read "$v" / printf -v "$v"               it runs
//	declare "$v=hi" / typeset / local        it runs
//	test -v "$v" / [[ -v $v ]] / let "$v=1"  it runs
//	export "$v=hi" / readonly / mapfile      `not a valid identifier`, nothing runs
//
// So the refusal is not general and the text is not what decides it. The two
// refusing rows were already written ahead of the arithmetic here; what stood
// in front of both of them was the *word-expansion* round, which is the one
// thing that had to move.

const injected = "a[$(echo INJECTION! >&2 ; echo 0)]"

func runUnsetText(t *testing.T, src string) (string, string, int) {
	t.Helper()
	var out, errs bytes.Buffer
	code := driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-c", src})
	return out.String(), errs.String(), code
}

func TestASubscriptArrivingAsTextIsExpandedOnlyOnceTheNameIsThere(t *testing.T) {
	for _, c := range []struct {
		name, src, out string
		ran            bool
	}{
		// The issue's own reproducer: no variable of that name, so the
		// brackets are never read and the command inside them never runs.
		{
			"a name the shell has not got",
			`v='` + injected + `'; unset "$v"; echo "st=$?"`,
			"st=0\n", false,
		},
		// The `-v` spelling is the same route and takes the same answer.
		{
			"the -v spelling",
			`v='` + injected + `'; unset -v "$v"; echo "st=$?"`,
			"st=0\n", false,
		},
		// A frozen name is refused by the *name*, which is the second
		// measured row that never reaches the brackets.
		{
			"a frozen name",
			`a=(x y z); readonly a; v='` + injected + `'; unset "$v"; echo "st=$?"`,
			"st=1\n", false,
		},
		// Operands are answered one at a time, so a name that is there
		// still reads its own brackets beside one that is not.
		{
			"a neighbor that is there",
			`b=(1 2); v='` + injected + `'; unset "$v" "b[$(echo 0)]"; declare -p b`,
			`declare -a b=([1]="2")` + "\n", false,
		},
		// And the controls, because the refusal is not general: once the
		// name holds something the brackets are read, and the command in
		// them runs, in bash as here.
		{
			"an array that is there",
			`a=(x y z); v='` + injected + `'; unset "$v"; declare -p a`,
			`declare -a a=([1]="y" [2]="z")` + "\n", true,
		},
		{
			"a table that is there",
			`declare -A a; a[0]=q; v='` + injected + `'; unset "$v"; declare -p a`,
			"declare -A a=()\n", true,
		},
		// Three other routes for the same text, each of which expands it in
		// both shells. They are here so that the fix cannot be read as "a
		// subscript arriving as text is never expanded".
		{
			"read is not this route",
			`v='` + injected + `'; read "$v" <<< hi; declare -p a`,
			`declare -a a=([0]="hi")` + "\n", true,
		},
		{
			"printf -v is not this route",
			`v='` + injected + `'; printf -v "$v" hi; declare -p a`,
			`declare -a a=([0]="hi")` + "\n", true,
		},
		{
			"a declaration is not this route",
			`v='` + injected + `'; declare "$v=hi"; declare -p a`,
			`declare -a a=([0]="hi")` + "\n", true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runUnsetText(t, c.src)
			ran := strings.Contains(errs, "INJECTION!")
			if out != c.out || ran != c.ran || code != 0 {
				t.Errorf("%s\n got %q ran=%v %d\nwant %q ran=%v 0",
					c.src, out, ran, code, c.out, c.ran)
			}
		})
	}
}
