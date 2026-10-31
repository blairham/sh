// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestAnElementReadFromAnUnsetArrayEvaluatesItsSubscriptOnce: the subscript is
// arithmetic, and a name holding nothing runs it once, as a name holding
// anything does. Measured 2026-10-31 on bash 5.3.20 under `-c` (#7189); every
// row here is byte for byte what that shell prints. It was 2 for the first
// three, and the negative subscript's warning was written once over a count
// of 2.
func TestAnElementReadFromAnUnsetArrayEvaluatesItsSubscriptOnce(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`i=0; : ${x[i++]}; echo $i`, "1\n"},
		{`i=0; echo "${x[i++]}"; echo $i`, "\n1\n"},
		{`i=0; : ${x[i++]:-d}; echo $i`, "1\n"},
		{`i=0; echo "[${x[-1+0*i++]}]"; echo $i`, "bash: line 1: x: bad array subscript\n[]\n1\n"},
		// The controls: a name holding anything was already read once.
		{`x=(); i=0; : ${x[i++]}; echo $i`, "1\n"},
		{`x=s; i=0; : ${x[i++]}; echo $i`, "1\n"},
	} {
		if out, _ := runBash(t, t.TempDir(), c.src); out != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}

// TestARefusalInsideArithmeticTextIsMadeOnce: a frozen name refused once is
// not refused again. The subscript of `${x[a[1]=5]}` was evaluated twice, and
// the `${a[1]:=x}` in an arithmetic text left text that still read as an
// expression, which then ran — so the store it named was refused a second
// time, and `b = 15 + x0` assigned 15. bash stops at the expansion's refusal
// and ends the `-c` string at 1. Measured 2026-10-31 on bash 5.3.20 (#7189).
func TestARefusalInsideArithmeticTextIsMadeOnce(t *testing.T) {
	const refused = "bash: line 1: a: readonly variable\n"
	for _, src := range []string{
		`declare -a a; readonly a; echo ${x[a[1]=5]}; echo next`,
		`declare -a a; readonly a; (( a[${a[1]:=x}]=5 )); echo next`,
		`readonly a; (( b = 15 + ${a[1]:=x}0 )); echo next`,
		`readonly a; echo $(( b = 1${a[1]:=x}5 )); echo next`,
	} {
		out, st := runBash(t, t.TempDir(), src)
		if out != refused || st != 1 {
			t.Errorf("%s\n got %q (status %d)\nwant %q (status 1)", src, out, st, refused)
		}
	}
	// And the store it never reached leaves the frozen name's listing as a
	// refused store would not: no `=()` (see
	// TestARefusedExpressionStoreSetsADeclaredOnlyArray, #6847).
	out, _ := runBash(t, t.TempDir(),
		`trap 'declare -p a' EXIT; declare -a a; readonly a; (( a[${a[1]:=x}]=5 ))`)
	if want := refused + "declare -ar a\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestAFailedExpansionInAnArithmeticCommandGivesUpTheLine: the expansion's
// failure and not the construct's, so the line is given up with 1 behind it
// and the construct is never traced — where its own failure, `(( b = 1/0 ))`,
// is traced and the line carries on. Measured 2026-10-31 on bash 5.3.20 under
// `-c`. See interp.Semantics.FailedExpansionEscapesAnArithmeticCommand (#7189).
func TestAFailedExpansionInAnArithmeticCommandGivesUpTheLine(t *testing.T) {
	const div = `bash: line 1: 1/0: division by 0 (error token is "0")` + "\n"
	for _, c := range []struct{ src, want string }{
		{"(( $((1/0)) + 1 )); echo \"same=$?\"\necho \"next=$?\"", div + "next=1\n"},
		{"(( b = $((1/0)) )); echo \"same=$?\"\necho \"next=$? b=$b\"", div + "next=1 b=\n"},
		{"set -x; (( b = $((1/0)) + 1 )); echo same\necho next", div + "+ echo next\nnext\n"},
		// The control: the construct's own failure.
		{"(( b = 1/0 )); echo \"same=$?\"", `bash: line 1: ((: b = 1/0 : division by 0 (error token is "0 ")` +
			"\nsame=1\n"},
	} {
		out, _ := runBash(t, t.TempDir(), c.src)
		if out != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
		}
		if strings.Contains(c.src, "$((1/0))") && strings.Contains(out, "((  b") {
			t.Errorf("%s\n got %q, traced a construct whose text did not expand", c.src, out)
		}
	}
}
