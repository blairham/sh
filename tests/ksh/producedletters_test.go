// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"regexp"
	"testing"
)

// A script's own letters reach a produced parameter: a float letter's places
// and format are what SECONDS is written and listed in, and a type letter
// other than the stated one — or the stated one taken off — ends the
// producer, leaving an ordinary variable holding the reading it gave at that
// moment through the new letters (#6854). See
// Semantics.RetypingEndsAProducedParameter.
//
// Measured 2026-10-27 on ksh93u+ 2012-08-01 under `-c`. The rows are
// patterns: a clock's fraction and a drawn RANDOM are not values to compare,
// and a row that must not tick reads the name twice instead of sleeping.
func TestAScriptsLettersReachAProducedParameter(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -F 5 SECONDS; echo ${#SECONDS}; typeset -p SECONDS`, "7\ntypeset -F 5 SECONDS=0\\.\\d{5}\n"},
		{`typeset -F 5 SECONDS; SECONDS=1; echo ${SECONDS%.*}; echo ${#SECONDS}`, "1\n7\n"},
		{`typeset -F 1 SECONDS; echo $SECONDS; typeset -F 3 SECONDS; echo $SECONDS`, "0\\.\\d\n0\\.\\d{3}\n"},
		{`typeset -F SECONDS; echo ${#SECONDS}`, "12\n"},
		{`typeset -F 5 SECONDS; a=$SECONDS; b=$SECONDS; typeset -F 2 SECONDS; typeset -p SECONDS`, "typeset -F 2 SECONDS=0\\.\\d{2}\n"},
		{`typeset -E 2 SECONDS; typeset -p SECONDS; SECONDS=1.23456; echo $SECONDS`, "typeset -E 2 SECONDS=0(\\.001)?\n1.2\n"},
		{`typeset +F SECONDS; a=$SECONDS; b=$SECONDS; [ "$a" = "$b" ] && echo frozen; SECONDS=3; echo "[$SECONDS]"`, "frozen\n\\[3\\]\n"},
		{`typeset -i SECONDS; typeset -p SECONDS; echo "[$SECONDS]"`, "typeset -i SECONDS=0\n\\[0\\]\n"},
		{`typeset -i16 SECONDS; typeset +i SECONDS; typeset -p SECONDS; SECONDS=5; echo "[$SECONDS]"`, "SECONDS=16#0\n\\[5\\]\n"},
		{`typeset +i RANDOM; a=$RANDOM; b=$RANDOM; [ "$a" = "$b" ] && echo frozen; RANDOM=7; echo $RANDOM; typeset -p RANDOM`, "frozen\n7\nRANDOM=7\n"},
		{`typeset -i8 RANDOM; typeset +i RANDOM; typeset -p RANDOM`, "RANDOM=\\d+\n"},
		{`typeset -u RANDOM; a=$RANDOM; b=$RANDOM; [ "$a" = "$b" ] && echo frozen; typeset -p RANDOM`, "frozen\ntypeset -u RANDOM=\\d+\n"},
		{"typeset +i LINENO\ntypeset -p LINENO\necho $LINENO\necho $LINENO", "LINENO=1\n1\n1\n"},
		{`typeset -F 5 SECONDS; function f { typeset -p SECONDS; }; SECONDS=1 f; typeset -p SECONDS`, "typeset -x -F 3 SECONDS=1\\.\\d{3}\ntypeset -F 5 SECONDS=0\\.\\d{5}\n"},
		{`typeset -i SECONDS; function f { typeset -p SECONDS; }; SECONDS=5 f; typeset -p SECONDS`, "typeset -x SECONDS=5\ntypeset -i SECONDS=0\n"},
		// The controls: the stated letter's own base keeps RANDOM drawing,
		// a letter that is not a type moves nothing, and the shapes nobody
		// declared are the stated ones.
		{`typeset -i16 RANDOM; a=$RANDOM; b=$RANDOM; [ "$a" != "$b" ] && echo drawing; typeset -p RANDOM`, "drawing\ntypeset -i 16 RANDOM=\\d+\n"},
		{`typeset -x SECONDS; typeset -p SECONDS`, "typeset -x -F 3 SECONDS=0\\.\\d{3}\n"},
		{`typeset -p SECONDS; typeset -p LINENO`, "typeset -F 3 SECONDS=0\\.\\d{3}\ntypeset -i LINENO=1\n"},
	} {
		out, _ := runKsh(t, t.TempDir(), tc.src)
		if !regexp.MustCompile("^" + tc.want + "$").MatchString(out) {
			t.Errorf("%s\n got %q\nwant %q", tc.src, out, tc.want)
		}
	}
}
