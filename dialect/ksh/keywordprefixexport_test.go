// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestAPrefixScopedToAKeywordCallIsExportedForIt pins what a child started
// from a `function`-form body is handed when the call had a prefix, each row
// measured 2026-10-15 on ksh93u+ under `-c` and `env -i PATH=/usr/bin:/bin`.
// The prefix is the call's environment, so the body's children get it,
// whichever way they are started, and so does a POSIX-form function it calls;
// a nested `function`-form call reads the value and hands its children
// nothing (#6538).
func TestAPrefixScopedToAKeywordCallIsExportedForIt(t *testing.T) {
	const pe = "/usr/bin/printenv"
	for _, tc := range []struct{ src, want string }{
		{`function f { ` + pe + ` v || echo noenv; echo "[$v]"; }; v=4 f`, "4\n[4]\n"},
		{`function f { (` + pe + ` v); echo "$(` + pe + ` v)"; }; v=4 f`, "4\n4\n"},
		{`function f { /usr/bin/env | while read -r kv; do case $kv in v=*) echo "$kv";; esac; done; }; v=4 f`, "v=4\n"},
		{`function f { exec ` + pe + ` v; }; v=4 f`, "4\n"},
		{`function f { typeset -p v; }; v=4 f`, "typeset -x v=4\n"},
		{`export v=1; function f { ` + pe + ` v; }; v=4 f; ` + pe + ` v`, "4\n1\n"},
		{`function f { ` + pe + ` v; }; v=4 f; v=5; ` + pe + ` v || echo noenv`, "4\nnoenv\n"},
		{`v=base; function f { g; }; g() { ` + pe + ` v; }; v=4 f`, "4\n"},
		{`v=base; function f { g; }; function g { echo "[$v]"; ` + pe + ` v || echo noenv; }; v=4 f`, "[4]\nnoenv\n"},
		{`export v=base; function f { g; }; function g { ` + pe + ` v || echo noenv; }; v=4 f; ` + pe + ` v`, "noenv\nbase\n"},
		// The POSIX form is the control: its prefix is the shell's own and
		// not exported.
		{`f() { ` + pe + ` v || echo noenv; }; v=4 f`, "noenv\n"},
	} {
		if got, _ := runKsh(t, t.TempDir(), tc.src); got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// TestAPrefixedOptindIsTheKeywordCallsCursor pins that a `function`-form
// call given OPTIND by a prefix starts its scan there rather than at the 1 the
// call is otherwise handed, and that the caller's cursor — its place inside a
// word included — comes back. Measured 2026-10-15 on ksh93u+ (#6538).
func TestAPrefixedOptindIsTheKeywordCallsCursor(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`function f { echo $OPTIND; getopts a o -a; echo $OPTIND; }; OPTIND=4 f; echo $OPTIND`, "4\n4\n1\n"},
		{`function f { /usr/bin/printenv OPTIND; }; OPTIND=4 f; /usr/bin/printenv OPTIND || echo none`, "4\nnone\n"},
		{`function f { typeset -p OPTIND; }; OPTIND=4 f`, "typeset -x -i OPTIND=4\n"},
		{`function f { getopts ab o -a -b; echo "$o $OPTIND"; }; OPTIND=2 f; echo $OPTIND`, "b 3\n1\n"},
		{`function f { echo $OPTIND; }; OPTIND=x f`, "0\n"},
		{`set -- -ab; getopts ab o; function f { getopts cd o -c -d; echo "$o $OPTIND"; }; OPTIND=2 f; getopts ab o; echo "$o $OPTIND"`, "d 3\nb 2\n"},
		// A call nested in the prefixed one has no prefix of its own and is
		// handed 1; a POSIX-form one shares the prefixed call's cursor.
		{`function f { g; }; function g { echo $OPTIND; }; OPTIND=5 f`, "1\n"},
		{`function f { g; }; g() { echo $OPTIND; }; OPTIND=5 f`, "5\n"},
		// The control: with no prefix the call is handed 1.
		{`OPTIND=3; function f { echo $OPTIND; }; f; echo $OPTIND`, "1\n3\n"},
	} {
		if got, _ := runKsh(t, t.TempDir(), tc.src); got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
