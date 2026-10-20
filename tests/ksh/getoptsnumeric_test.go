// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// The `letter#` numeric argument type, which is this shell's alone (#2947).
//
// interp proves what the axis does; this file pins that this preset answers
// it yes, and the wording that goes with it. Measured 2026-09-16 on ksh93u+
// 2012-08-01, as a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`
// with stdin on /dev/null and a fresh directory — the preset aliases make an
// interactive probe of this shell say something else.

func getoptsRun(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Name: "ksh", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// The scan, over both spellings and over the failures.
//
// stderr goes to /dev/null here and the wording is asserted on its own below,
// because what the scan does and what it says are two claims. The location in
// front of it was a third and is now the same as the shell's — this builtin's
// complaint carries no line (#3438) — and getoptslocation_test.go is where
// that is pinned.
func TestGetoptsReadsTheNumericArgumentType(t *testing.T) {
	for _, tc := range []struct{ name, words, want string }{
		{"the next word", "-n 5", "[n:5] end=3\n"},
		{"attached", "-n5", "[n:5] end=2\n"},
		{"a negative number", "-n -3 rest", "[n:-3] end=3\n"},
		{"digits then an operand", "-n7 rest", "[n:7] end=2\n"},
		// The rest of the word goes into OPTARG and the scan carries on
		// after the numeral, so the `x` is reported as an option too.
		{"digits and a letter", "-n5x", "[n:5x][?:unset] end=2\n"},
		{"nothing there", "-n", "[?:unset] end=2\n"},
		{"not a number", "-n abc", "[?:unset] end=3\n"},
		{"not a number, attached", "-nabc", "[?:unset] end=2\n"},
		// A `#` the type has consumed is not an option of its own.
		{"the marker is not an option", "-#", "[?:unset] end=2\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "set -- " + tc.words + "\n" +
				`while getopts 'n#' o 2>/dev/null; do printf '[%s:%s]' "$o" "${OPTARG-unset}"; done` + "\n" +
				`printf ' end=%s\n' "$OPTIND"`
			out, _ := getoptsRun(t, src)
			if out != tc.want {
				t.Errorf("getopts 'n#' over %s = %q, want %q", tc.words, out, tc.want)
			}
		})
	}
}

// The wording, which the numeric type brings with it: a `#` letter says
// `numeric argument expected` where a `:` letter says `argument expected`,
// and it says it for a missing argument as much as for an unreadable one.
//
// Measured 2026-09-16 on ksh93u+ 2012-08-01, and matched on the message so
// that the location it is written with stays one claim in one place — see
// getoptslocation_test.go, which pins that this builtin writes no line.
func TestGetoptsNumericArgumentWording(t *testing.T) {
	for _, tc := range []struct{ words, want string }{
		{"-n abc", "-n: numeric argument expected\n"},
		{"-nabc", "-n: numeric argument expected\n"},
		{"-n", "-n: numeric argument expected\n"},
		{"-#", "-#: unknown option\n"},
	} {
		src := "set -- " + tc.words + "\ngetopts 'n#' o"
		out, _ := getoptsRun(t, src)
		if !strings.HasSuffix(out, tc.want) {
			t.Errorf("getopts 'n#' over %s = %q, want it to end with %q", tc.words, out, tc.want)
		}
	}
	// And the `:` type in the same string keeps its own wording.
	out, _ := getoptsRun(t, "set -- -s\ngetopts 'n#s:' o")
	if !strings.HasSuffix(out, "-s: argument expected\n") {
		t.Errorf("getopts 'n#s:' over -s = %q, want it to end with the string wording", out)
	}
}

// The two types in one string, which is the case the suite file pairs them
// in: the `:` type is unchanged by the `#` type existing.
func TestGetoptsMixesTheTwoArgumentTypes(t *testing.T) {
	for _, tc := range []struct{ words, want string }{
		{"-n 5 -s hello rest", "[n:5][s:hello] end=5\n"},
		{"-s hello -n 5", "[s:hello][n:5] end=5\n"},
		{"-n5 -shello", "[n:5][s:hello] end=3\n"},
	} {
		src := "set -- " + tc.words + "\n" +
			`while getopts 'n#s:' o; do printf '[%s:%s]' "$o" "${OPTARG-unset}"; done` + "\n" +
			`printf ' end=%s\n' "$OPTIND"`
		out, _ := getoptsRun(t, src)
		if out != tc.want {
			t.Errorf("getopts 'n#s:' over %s = %q, want %q", tc.words, out, tc.want)
		}
	}
}

// What the argument may be spelled as. It is a numeral and not an expression:
// `16#ff` and `0x1f` are read where `1+1` and `3.5` are refused, so nothing
// here is evaluating anything.
func TestGetoptsNumericArgumentSpellings(t *testing.T) {
	taken := []string{"007", "08", "+4", "0x10", "16#FF", "64#_"}
	refused := []string{"1+1", "3.5", "2#12", "99#5", "0b101"}
	check := func(word, want string) {
		t.Helper()
		src := "set -- -n '" + word + "'\n" +
			`getopts 'n#' o 2>/dev/null; printf '[%s:%s]\n' "$o" "${OPTARG-unset}"`
		out, _ := getoptsRun(t, src)
		if out != want {
			t.Errorf("-n %s = %q, want %q", word, out, want)
		}
	}
	for _, w := range taken {
		check(w, "[n:"+w+"]\n")
	}
	for _, w := range refused {
		check(w, "[?:unset]\n")
	}
}
