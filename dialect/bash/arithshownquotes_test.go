// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// An expression the script wrote inside `(( ))`, `$(( ))` or a `for (( ))`
// head is shown without its double quotes, in the trace and in a refusal,
// as it is evaluated; single quotes stay, and `let`'s operand, which arrives
// as text, keeps both (#6777).
//
// Measured 2026-10-16 against GNU bash 5.3.20 from a script file, byte for
// byte.
func TestAWrittenExpressionIsShownWithoutItsDoubleQuotes(t *testing.T) {
	src := "declare -A a; set -x\n" +
		"(( a[\"x\"]=3 ))\n" +
		"(( c = \"1\" ))\n" +
		"(( a['y']=4 ))\n" +
		"(( a['x\"y']=4 ))\n" +
		"(( c = \"1\"+\"2\" ))\n" +
		"for (( i=\"0\"; i<\"1\"; i++ )); do :; done\n" +
		"set +x\n" +
		"echo $(( \"1\"+ ))\n" +
		"(( \"1\"+ ))\n" +
		"let 'c=\"1\"+'\n"
	want := "+ ((  a[x]=3  ))\n" +
		"+ ((  c = 1  ))\n" +
		"+ ((  a['y']=4  ))\n" +
		"+ ((  a['xy']=4  ))\n" +
		"+ ((  c = 1+2  ))\n" +
		"+ (( i=0 ))\n" +
		"+ (( i<1 ))\n" +
		"+ :\n" +
		"+ (( i++  ))\n" +
		"+ (( i<1 ))\n" +
		"+ set +x\n" +
		"bash: line 9: 1+ : arithmetic syntax error: operand expected (error token is \"+ \")\n" +
		"bash: line 10: ((: 1+ : arithmetic syntax error: operand expected (error token is \"+ \")\n" +
		"bash: line 11: let: c=\"1\"+: arithmetic syntax error: operand expected (error token is \"\"1\"+\")\n"
	if got, _ := posixRouteStatus(t, src, interp.RouteScriptFile); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
