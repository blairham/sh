// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// `setopt warncreateglobal` writes a sentence when an assignment inside a
// function creates a parameter nothing else had, and `setopt warnnestedvar`
// writes one when an assignment reaches a parameter belonging to a scope
// outside the function it was written in.
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f`, 2026-09-26, from
// a script file, reading standard error alone. Every row runs in both states
// of the option; the *off* state is the control and already agreed before
// either was wired, and in both states the assignment itself behaves the same
// way — which is the whole reason these are lints (#4553, #4554).
func TestWarnCreateGlobalNamesTheParameterAndTheFunction(t *testing.T) {
	const src = "setopt warncreateglobal\nf() { gv=1 }\nf\nprint \"gv=$gv\"\n"
	out, st, errs := runZshSplit(t, t.TempDir(), src)
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if want := "f: scalar parameter gv created globally in function f\n"; errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}
	if want := "gv=1\n"; out != want {
		t.Errorf("stdout = %q, want %q — the assignment is right either way", out, want)
	}
}

func TestWarnNestedVarNamesTheParameterAndTheFunction(t *testing.T) {
	const src = "setopt warnnestedvar\n" +
		"outer() { local lv=1; inner; print \"lv=$lv\" }\ninner() { lv=2 }\nouter\n"
	out, st, errs := runZshSplit(t, t.TempDir(), src)
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if want := "inner: scalar parameter lv set in enclosing scope in function inner\n"; errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}
	if want := "lv=2\n"; out != want {
		t.Errorf("stdout = %q, want %q — inner really does reach outer's local", out, want)
	}
}

// The control both rows share, and it is the row the whole pair rests on: with
// the options off the same two scripts say nothing at all and compute exactly
// what they computed with them on.
func TestTheScopeLintsSayNothingWithTheOptionsOff(t *testing.T) {
	for _, tc := range []struct{ name, src, out string }{
		{
			"warncreateglobal",
			"unsetopt warncreateglobal\nf() { gv=1 }\nf\nprint \"gv=$gv\"\n", "gv=1\n",
		},
		{
			"warnnestedvar",
			"unsetopt warnnestedvar\n" +
				"outer() { local lv=1; inner; print \"lv=$lv\" }\ninner() { lv=2 }\nouter\n", "lv=2\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, t.TempDir(), tc.src)
			if errs != "" {
				t.Errorf("stderr = %q, want nothing", errs)
			}
			if out != tc.out || st != 0 {
				t.Errorf("stdout = %q status %d, want %q at 0", out, st, tc.out)
			}
		})
	}
}

// The rows that say what each lint is *about*, which is a forgotten `local`
// rather than every write a function makes.
func TestTheScopeLintsSpareADeliberateGlobal(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"typeset -g", "typeset -g tg=1"},
		{"local", "local lv=1"},
		{"export", "export ev=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "setopt warncreateglobal warnnestedvar\ng() { " + tc.body + " }\ng\n"
			_, st, errs := runZshSplit(t, t.TempDir(), src)
			if errs != "" || st != 0 {
				t.Errorf("stderr = %q status %d, want nothing at 0", errs, st)
			}
		})
	}
}

// One body draws one of each when both options are on, which is what says the
// two are complementary rather than two spellings of one question: the first
// assignment creates the name and the second reaches it in the scope the
// first put it in.
func TestTheTwoScopeLintsAreComplementary(t *testing.T) {
	const src = "setopt warncreateglobal warnnestedvar\nq() { gq=1; gq=2 }\nq\n"
	_, st, errs := runZshSplit(t, t.TempDir(), src)
	want := "q: scalar parameter gq created globally in function q\n" +
		"q: scalar parameter gq set in enclosing scope in function q\n"
	if errs != want || st != 0 {
		t.Errorf("stderr = %q status %d, want %q at 0", errs, st, want)
	}
}

// An array literal is an `array parameter` to the sentence where a scalar
// assignment is a `scalar` one, and a subshell is a shell of its own rather
// than a continuation of the call that made it.
func TestTheScopeLintsWordAndScopeTheirRows(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an array literal",
			"setopt warncreateglobal\ni() { arr=(a b) }\ni\n",
			"i: array parameter arr created globally in function i\n",
		},
		{
			"a subshell draws nothing",
			"setopt warncreateglobal warnnestedvar\nt() { (tv=1) }\nt\n",
			"",
		},
		{
			"but a call inside one names itself",
			"setopt warncreateglobal\nt() { (f5() { tv5=1 }; f5) }\nt\n",
			"f5: scalar parameter tv5 created globally in function f5\n",
		},
		{
			"and a block inside a function is not a scope",
			"setopt warncreateglobal\nb() { { bv=1 } }\nb\n",
			"b: scalar parameter bv created globally in function b\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, st, errs := runZshSplit(t, t.TempDir(), tc.src)
			if errs != tc.want || st != 0 {
				t.Errorf("stderr = %q status %d, want %q at 0", errs, st, tc.want)
			}
		})
	}
}

// And both states are reported back on the surfaces a script reads.
func TestTheScopeLintOptionsAreReportedBack(t *testing.T) {
	const src = `[[ -o warncreateglobal ]] && print a=on || print a=off
setopt warncreateglobal warnnestedvar
print b=${options[warncreateglobal]} c=${options[warnnestedvar]}`
	out, st := runZsh(t, t.TempDir(), src)
	if want := "a=off\nb=on c=on\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}
