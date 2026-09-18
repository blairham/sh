// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"fmt"
	"strings"
	"testing"
)

// Two builtins whose refusals were dash's rather than this shell's, measured
// 2026-09-17 inside the pinned image
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b,
// BusyBox v1.37.0, a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`
// and the same case through `cmd/ash` in the same container (#3278, #3367).

// `test` and its two bracket spellings report as the applet — the shell's own
// name and nothing else — where every other complaint from this shell opens
// `<file>: <builtin>: line N:`. And an unknown operator in the *two-word* form
// leaves an operand behind, so the word named is the one after it.
func TestTheTestBuiltinReportsAsTheShellAndNamesTheLeftoverWord(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want string
		why  string
	}{
		{
			`n=5; [ n -eq 5 ]`, "ash: n: out of range",
			"the control: a complaint really about the operand names the operand",
		},
		{`[ 5 -eq n ]`, "ash: n: out of range", "either side of it"},
		{
			`[ -Q g.f ]`, "ash: g.f: unknown operand",
			"an operator this shell has not got is a string, and the word after it is one too many",
		},
		{`[ -N n.f ]`, "ash: n.f: unknown operand", "and any other spelling of one"},
		{`[ a b c ]`, "ash: b: unknown operand", "three words already named the middle one"},
		{`[ 1 -eq 2`, "ash: missing ]", "and a bracket that never closed is the applet's too"},
		{`test -Q g.f`, "ash: g.f: unknown operand", "the other spelling answers alike"},
	} {
		out, status := run(t, tc.src+"\n")
		if !strings.HasPrefix(out, tc.want) {
			t.Errorf("%s = %q, want it to open %q — %s", tc.src, out, tc.want, tc.why)
		}
		if status != 2 {
			t.Errorf("%s = status %d, want 2", tc.src, status)
		}
	}
	// The control that says the applet form is this builtin's and not the
	// shell's: another builtin's refusal keeps the script, the name and the
	// line.
	out, _ := run(t, "unset -Z\n")
	if !strings.Contains(out, "unset: line 1:") {
		t.Errorf("unset -Z = %q, want the located refusal", out)
	}
}

// `read`'s option complaints were dash's in four separate ways: the
// capitalisation, one generic sentence where this shell has three, and both
// statuses inverted.
func TestReadOptionComplaintsAreThisShells(t *testing.T) {
	for _, tc := range []struct {
		src    string
		want   string
		status int
	}{
		{"read -d", "read: line 1: no arg for -d option", 2},
		{"read -p", "read: line 1: no arg for -p option", 2},
		{"echo hi | read -u x v", "read: line 1: invalid file descriptor", 2},
		{"echo hi | read -n x v", "read: line 1: invalid count", 2},
		{"echo hi | read -t x v", "read: line 1: invalid timeout", 2},
	} {
		out, _ := run(t, "( "+tc.src+" ) 2>&1\n( "+tc.src+" ) >/dev/null 2>&1\necho st=$?\n")
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s said %q, want %q in it", tc.src, out, tc.want)
		}
		if want := fmt.Sprintf("st=%d", tc.status); !strings.Contains(out, want) {
			t.Errorf("%s said %q, want %s", tc.src, out, want)
		}
	}
	// The operand is nowhere in any of the three, which is what parts them
	// from the generic sentence they replace.
	out, _ := run(t, "( echo hi | read -t zzz v ) 2>&1\n")
	if strings.Contains(out, "zzz") {
		t.Errorf("got %q, want the word not echoed back", out)
	}
}

// And a name that is not an identifier is the applet form, at 1 — where every
// other builtin here refuses the same operand located and at 2.
func TestReadsBadNameIsTheAppletFormAtOne(t *testing.T) {
	for _, name := range []string{"1bad", "a[0]", "a-b"} {
		out, _ := run(t, "( echo hi | read '"+name+"' ) 2>&1\n"+
			"( echo hi | read '"+name+"' ) >/dev/null 2>&1\necho st=$?\n")
		want := "ash: read: '" + name + "': bad variable name"
		if !strings.Contains(out, want) {
			t.Errorf("read %q said %q, want %q", name, out, want)
		}
		if !strings.Contains(out, "st=1") {
			t.Errorf("read %q said %q, want st=1", name, out)
		}
	}
	// The controls: five other builtins refuse the same operand with the
	// script, their own name and the line, at 2.
	for _, tc := range []struct{ src, want string }{
		{"export 'e[0]=x'", "export: line 1: e[0]: bad variable name"},
		{"readonly 'r[0]'", "readonly: line 1: r[0]: bad variable name"},
		{"f() { local 'l[0]'; }; f", "local: line 1: l[0]: bad variable name"},
	} {
		out, _ := run(t, "( "+tc.src+" ) 2>&1\n( "+tc.src+" ) >/dev/null 2>&1\necho st=$?\n")
		if !strings.Contains(out, tc.want) || !strings.Contains(out, "st=2") {
			t.Errorf("%s said %q, want %q at 2", tc.src, out, tc.want)
		}
	}
}

// The rest of that parse, which is what the argument-count reading could not
// give: the word named is the one the parse **stopped at**, and an operator
// standing where its right operand belonged is missing an argument rather
// than leaving one behind.
//
// Measured 2026-09-18 inside the same pinned image, BusyBox v1.37.0, a script
// file under `env -i PATH=/usr/bin:/bin LC_ALL=C` (#3550).
func TestTheTestBuiltinNamesTheWordItsParseStoppedAt(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want string
		why  string
	}{
		// One word further along each time, which is the same rule and not
		// three: the expression ends and the next word is named.
		{`[ a b c ]`, "ash: b: unknown operand", "`a` is the expression"},
		{`[ -z a b ]`, "ash: b: unknown operand", "`-z a` is"},
		{`[ ! a b ]`, "ash: b: unknown operand", "and `! a` is"},
		{`[ -Q x y ]`, "ash: x: unknown operand", "`-Q` is, being no operator here"},
		{`[ -z a b c ]`, "ash: b: unknown operand", "the first leftover, not the last word taken"},
		{`[ a = b = c ]`, "ash: =: unknown operand", "which here is the second `=`"},
		// A binary operator this shell has, with nothing behind it: the
		// operator is named, because what is missing is its right operand.
		{`[ 1 -eq ]`, "ash: -eq: argument expected", "an arithmetic comparison"},
		{`[ g.f -ot ]`, "ash: -ot: argument expected", "and a file one"},
		{`[ a == ]`, "ash: ==: argument expected", "`==` is in this shell's set"},
		{`[ a =~ ]`, "ash: =~: unknown operand", "and `=~` is not"},
		// A connective is an operator missing its right operand too, and
		// that one is said with nothing named at all — at every length.
		{`[ x -a ]`, "ash: argument expected", "two words"},
		{`[ -z a -a ]`, "ash: argument expected", "three"},
		{`[ 1 -eq 1 -a ]`, "ash: argument expected", "and four"},
	} {
		out, status := run(t, tc.src+"\n")
		if !strings.HasPrefix(out, tc.want) {
			t.Errorf("%s = %q, want it to open %q — %s", tc.src, out, tc.want, tc.why)
		}
		if status != 2 {
			t.Errorf("%s: status %d, want 2", tc.src, status)
		}
	}
}
