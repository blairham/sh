// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// A declaration operand's `=` is looked for after its subscript, so an `=`
// inside the brackets is part of the key — however the operand arrived:
// written, quoted, or expanded from a parameter (#6781).
//
// Measured 2026-10-16 against GNU bash 5.3.20 from a script file; the rows
// are read back one key at a time, since a table lists in an order of its own.
func TestADeclarationOperandsSubscriptMayHoldAnEquals(t *testing.T) {
	for _, c := range []struct{ probe, want string }{
		{`declare -A m; declare m["a=b"]=w; echo "st=$? ${m[a=b]}"`, "st=0 w\n"},
		{`declare -A m; declare 'm[a=b]=w'; echo "${m[a=b]}"`, "w\n"},
		{`declare -A m; v='m[c=d]=x'; declare "$v"; echo "${m[c=d]}"`, "x\n"},
		{`declare -A m; declare 'm[b[1]=2]=v'; echo "${m[b[1]=2]}"`, "v\n"},
		{`declare -A m; declare 'm[a=b]+=w'; declare 'm[a=b]+=x'; echo "${m[a=b]}"`, "wx\n"},
		{`f(){ local -A n; local n[a=b]=1; echo "${n[a=b]}"; }; f`, "1\n"},
		{`declare -A m; declare m[e]=f=g; echo "${m[e]}"`, "f=g\n"},
		{`declare -A m; declare 'm[a=b'; echo st=$?`, "bash: line 1: declare: `m[a=b': not a valid identifier\nst=1\n"},
		{`export m2[a=b]=1; echo st=$?`, "bash: line 1: export: `m2[a=b]': not a valid identifier\nst=1\n"},
	} {
		got, _ := posixRouteStatus(t, c.probe, interp.RouteScriptFile)
		if got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.probe, got, c.want)
		}
	}
}
