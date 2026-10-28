// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// TestGetoptsRefusalNamesTheScript pins where ksh93 writes `getopts`'s
// refusal of one of its names, a freeze and a value a typed name will not
// take alike: the builtin's location and `$0` where the builtin's name goes,
// which is the function's own name inside a `function`-form body. Each row
// measured 2026-10-15 on ksh93u+ under `-c` (#6563).
func TestGetoptsRefusalNamesTheScript(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`readonly o; getopts x o -x; echo "st=$?"`, "ksh: ksh: o: is read only\nst=2\n"},
		{`typeset -i n=5; getopts x n -y 2>/dev/null; echo "st=$? [$n]"`, "st=2 [5]\n"},
		{`typeset -i n=5; getopts x n -y 2>&1; echo after`, "ksh: ksh: ?: arithmetic syntax error\nafter\n"},
		{`readonly o; function f { getopts x o -x; echo "st=$?"; }; f`, "ksh: f[1]: f: o: is read only\nst=2\n"},
		{`readonly o; g() { getopts x o -x; echo "st=$?"; }; g`, "ksh: ksh: o: is read only\nst=2\n"},
	} {
		got, _ := runKsh(t, t.TempDir(), tc.src)
		if !strings.HasSuffix(got, tc.want) {
			t.Errorf("%s\n got %q\nwant …%q", tc.src, got, tc.want)
		}
	}
}

// TestATypedNameRefusesGetoptsAsAFreezeDoes pins that a value `getopts`'s
// typed name will not take costs what a freeze costs: 2 where the scan found
// something and the end of the script where it had run out. Measured
// 2026-10-15 on ksh93u+ (#6563). See
// interp.Semantics.GetoptsTypedValueFailureIsAFreeze.
func TestATypedNameRefusesGetoptsAsAFreezeDoes(t *testing.T) {
	out, st := runKsh(
		t,
		t.TempDir(),
		`typeset -i n=5; getopts x n -y 2>/dev/null; echo "st=$? [$n]"; getopts x n 2>/dev/null; echo after`,
	)
	if out != "st=2 [5]\n" || st != 2 {
		t.Errorf("got %q (%d), want %q (2)", out, st, "st=2 [5]\n")
	}
}

// TestReadGivesUpAtATypedNameItCannotFill pins ksh93's `read` over a typed
// name and a value that will not evaluate: reported, status 1, the name and
// every name after it left as they were, and the script going on. Measured
// 2026-10-15 on ksh93u+ (#6563). See
// interp.Semantics.ReadTypedValueFailureIsAnAssignmentFailure.
func TestReadGivesUpAtATypedNameItCannotFill(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"typeset -i n=5; read n m <<E\n1+ b\nE\necho \"[$?] [$n] [$m]\"; echo after", "read: 1+: more tokens expected\n[1] [5] []\nafter\n"},
		{"typeset -i n=5; read m n <<E\nb 1+\nE\necho \"[$?] [$n] [$m]\"", "read: 1+: more tokens expected\n[1] [5] [b]\n"},
		{"typeset -i n; read n <<E\n?\nE\necho \"[$?] [$n]\"", "read: ?: arithmetic syntax error\n[1] []\n"},
		// The control: a value that evaluates is stored.
		{"typeset -i n=5; read n <<E\n3+4\nE\necho \"[$?] [$n]\"", "[0] [7]\n"},
	} {
		got, _ := runKsh(t, t.TempDir(), tc.src)
		if _, rest, ok := strings.Cut(got, "ksh: "); ok && strings.HasPrefix(rest, "read:") {
			got = rest
		}
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// TestADeclarationKeepsOptindAnInteger pins that a declaration over OPTIND
// leaves its integer attribute where an ordinary name loses it, at the top
// and in a `function`-form body. Measured 2026-10-15 on ksh93u+ (#6563). See
// interp.Runner.KeepOptindThroughDeclarations.
func TestADeclarationKeepsOptindAnInteger(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`function f { typeset OPTIND=x; echo $OPTIND; }; f; typeset OPTIND=y; echo $OPTIND`, "0\n0\n"},
		{`typeset OPTIND=3+4; typeset -p OPTIND`, "typeset -i OPTIND=7\n"},
		{`export OPTIND=3+4; typeset -p OPTIND`, "typeset -x -i OPTIND=7\n"},
		{`function f { OPTIND=5; typeset OPTIND; echo "[$OPTIND]"; }; f`, "[5]\n"},
		{`function f { unset OPTIND; typeset OPTIND; echo "[${OPTIND-unset}]"; }; f`, "[1]\n"},
		{`unset OPTIND; function f { typeset OPTIND=3+4; echo $OPTIND; }; f; echo "[${OPTIND-unset}]"`, "7\n[unset]\n"},
		{`typeset -Z3 OPTIND=5; typeset -p OPTIND`, "typeset -Z 3 -R 3 OPTIND=005\n"},
		// After the attribute is taken away a declaration is an ordinary one,
		// and an ordinary integer name loses the attribute to it.
		{`typeset +i OPTIND; typeset OPTIND=3+4; echo $OPTIND`, "3+4\n"},
		{`unset OPTIND; typeset OPTIND=3+4; echo $OPTIND`, "3+4\n"},
		{`typeset -i n=1; typeset n=3+4; echo $n`, "3+4\n"},
	} {
		if got, _ := runKsh(t, t.TempDir(), tc.src); got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
