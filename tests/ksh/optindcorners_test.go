// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"regexp"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **OPTIND's listing and attribute corners, and the shell's other integer
// parameters** (#6633). Measured 2026-10-15 on ksh93u+ 2012-08-01 under `-c`,
// `env -i PATH=/bin`; a PPID is written as N.
func TestOptindCornersAndTheOtherIntegerParameters(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// A valueless declaration that makes the call's cursor local, with
		// no integer letter, lists a base of 0 until the script assigns it.
		{`function f { typeset OPTIND; typeset -p OPTIND; }; f`, "typeset -i 0 OPTIND=1\n"},
		{`function f { typeset -x OPTIND; getopts a o -a; typeset -p OPTIND; }; f`, "typeset -x -i 0 OPTIND=2\n"},
		{`function f { typeset OPTIND; OPTIND=2+3; typeset -p OPTIND; }; f`, "typeset -i OPTIND=5\n"},
		{`function f { typeset -i OPTIND; typeset -p OPTIND; }; f`, "typeset -i OPTIND=1\n"},
		{`function f { typeset OPTIND; g; }; function g { typeset -p OPTIND; }; f`, "typeset -i OPTIND=1\n"},
		{`function f { typeset OPTIND; p; }; p() { typeset -p OPTIND; }; f`, "typeset -i 0 OPTIND=1\n"},
		// A base is listed and the number is still written in decimal.
		{`typeset -i16 OPTIND=20; echo $OPTIND; typeset -p OPTIND`, "20\ntypeset -i 16 OPTIND=20\n"},
		{`typeset -i16 OPTIND=20; OPTIND=1; getopts a o -a; echo $OPTIND`, "2\n"},
		// `+i` on the line that makes the cursor local takes nothing off.
		{`function f { typeset +i OPTIND; OPTIND=x; echo $OPTIND; }; f; typeset -p OPTIND`, "0\ntypeset -i OPTIND=1\n"},
		{`function f { typeset +i OPTIND=7; typeset -p OPTIND; }; f`, "OPTIND=7\n"},
		{`function f { typeset OPTIND; typeset +i OPTIND; typeset -p OPTIND; }; f`, "OPTIND=1\n"},
		{`typeset +i OPTIND; OPTIND=x; echo $OPTIND`, "x\n"},
		// A call that unsets OPTIND leaves the caller what it writes after.
		{`function f { unset OPTIND; OPTIND=3+4; }; f; echo "[${OPTIND-unset}]"`, "[3+4]\n"},
		{`function f { OPTIND=3; }; f; echo "[${OPTIND-unset}]"`, "[1]\n"},
		// PPID, TMOUT and MAILCHECK.
		{`typeset -p PPID TMOUT MAILCHECK`, "typeset -i PPID=N\ntypeset -i TMOUT=0\ntypeset -i MAILCHECK=600\n"},
		{`typeset PPID=7+1; echo $PPID; TMOUT=x; echo $TMOUT; MAILCHECK=1+1; echo $MAILCHECK`, "8\n0\n2\n"},
		{`typeset TMOUT=2+3; typeset -p TMOUT`, "typeset -i TMOUT=5\n"},
		{`function f { typeset MAILCHECK=2+3; typeset -p MAILCHECK; }; f`, "MAILCHECK=2+3\n"},
		{`unset TMOUT; TMOUT=1+1; echo $TMOUT`, "1+1\n"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{
			Name: "ksh", Dir: t.TempDir(), Env: []string{"PATH=/bin"},
		}, tc.src+"\n")
		if err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		if got := ppid.ReplaceAllString(out, "PPID=N"); got != tc.want {
			t.Errorf("%s:\ngot  %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

var ppid = regexp.MustCompile(`PPID=[0-9]+`)

// An inherited TMOUT or MAILCHECK is read as one integer constant and kept
// exported; anything else is 0. Measured the same way, one name per shell.
func TestAnInheritedIntegerParameterIsReadAsAConstant(t *testing.T) {
	for in, want := range map[string]string{
		"7": "7", "-1": "-1", "08": "8", "010": "10", "0x10": "16", "16#10": "16",
		"2x": "0", "1+1": "0", " 2 ": "0", "3.9": "0", "abc": "0", "": "0",
		"+5": "5", "-0x10": "-16", "--1": "0", "16#-1": "0",
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{
			Name: "ksh", Dir: t.TempDir(), Env: []string{"PATH=/bin", "TMOUT=" + in},
		}, "typeset -p TMOUT\n")
		if err != nil {
			t.Fatal(err)
		}
		if w := "typeset -x -i TMOUT=" + want + "\n"; out != w {
			t.Errorf("TMOUT=%q: got %q, want %q", in, out, w)
		}
	}
}
