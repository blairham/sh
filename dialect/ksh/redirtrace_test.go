// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **A command's redirections, traced on a line of their own** (#5558).
// Measured 2026-10-03 on ksh93u+ under `-c`. See
// interp.Semantics.TraceRedirectionsOnALineOfTheirOwn.
func TestARedirectionIsTracedOnALineOfItsOwn(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`set -x; true >/dev/null 2>/dev/null`, "+ true\n+ 1> /dev/null 2> /dev/null\n"},
		// The piece and its newline go out before the install, so the
		// last redirection of standard error still shows whole, and one
		// after it does not.
		{`set -x; true 2>/dev/null; echo z`, "+ true\n+ 2> /dev/null\n+ echo z\nz\n"},
		{`set -x; true 2>/dev/null 1>&2; echo z`, "+ true\n+ 2> /dev/null + echo z\nz\n"},
		{`set -x; true 3>/dev/null 2>/dev/null; echo z`, "+ true\n+ 3> /dev/null 2> /dev/null\n+ echo z\nz\n"},
		// A redirection that fails writes nothing, and no newline follows.
		{`set -x; true >/dev/null 3>/nope/f; echo $?`, "+ true\n+ 1> /dev/null ksh: /nope/f: cannot create [No such file or directory]\n+ echo 1\n1\n"},
		{`set -x; true 2>/dev/null 3>/nope/f; echo $?`, "+ true\n+ 2> /dev/null + echo 1\n1\n"},
		{`set -x; { echo a; } 2>&1 >/dev/null`, "+ 2>& 1 1> /dev/null\n+ echo a\n"},
		{`set -x; true <>/dev/null; true 3<>/dev/null`, "+ true\n+ 1<> /dev/null\n+ true\n+ 3<> /dev/null\n"},
		{`set -x; true 3>/dev/null 4<&3; true 3<&0-`, "+ true\n+ 3> /dev/null 4<& 3\n+ true\n+ 3<& 0-\n"},
		{`set -x; true >| /dev/null 2>>/dev/null`, "+ true\n+ 1>| /dev/null 2>> /dev/null\n"},
		{`set -x; exec 3>/dev/null; exec 3>&-; echo z`, "+ exec\n+ 3> /dev/null\n+ exec\n+ 3>& -\n+ echo z\nz\n"},
		{`set -x; true {fd}>/dev/null; echo $fd`, "+ true\n+ {fd}> /dev/null\n+ echo 10\n10\n"},
		{`set -x; a=/dev/null; true >$a`, "+ a=/dev/null\n+ true\n+ 1> /dev/null\n"},
		{`set -x; x=$(</dev/null)`, "+ 0< /dev/null\n+ x=''\n"},
		{`set -x; x=1 true >/dev/null`, "+ true\n+ 1> /dev/null\n+ x=1\n"},
		// PS4 goes out before the first target is expanded.
		{`set -x; true >/dev/null 2>$(echo x >&2); echo ok`, "+ true\n+ 1> /dev/null + echo x\n+ 1>& 2\nx\nksh: : cannot open\n+ echo ok\nok\n"},
		// A here-string's piece and a here-document's end their own line.
		{`set -x; true 3<<<"a b" <&3`, "+ true\n+ 3<<< a b\n0<& 3\n"},
		{`set -x; true 4<<<x 2>/dev/null; echo z`, "+ true\n+ 4<<< x\n2> /dev/null\n+ echo z\nz\n"},
		{"set -x; x=1; true <<E >/dev/null\na $x\nE", "+ x=1\n+ true\n+ 0<< \\E\na 1\nE\n1> /dev/null\n"},
		{"set -x; true <<-E 3<<'F'\n\tt\n\tE\n$f\nF", "+ true\n+ 0<< \\E\nt\nE\n3<< \\F\n$f\nF\n"},
		{"set -x; true <<E 3>/dev/null\nE\necho z", "+ true\n+ 0<<3> /dev/null\n+ echo z\nz\n"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
		if err != nil {
			t.Fatal(err)
		}
		if out != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}

// **Where a command's trace line stands against its redirections** (#5547):
// ahead of them, so a redirection that will not open still leaves the line.
// Measured 2026-10-02 under `-c`. See
// interp.Semantics.TraceLineFollowsTheRedirections.
func TestATraceLineFollowsTheRedirections(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`set -x; true >/nope/f`, "+ true\n+ ksh: /nope/f: cannot create [No such file or directory]\n"},
		{`set -x; z=$(echo s >&2) true >/nope/f`, "+ true\n+ ksh: /nope/f: cannot create [No such file or directory]\n"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
		if err != nil {
			t.Fatal(err)
		}
		if out != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}
