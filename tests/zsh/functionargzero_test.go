// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `functionargzero` decides what `$0` answers, and it is a **state** rather
// than this shell's fixed reading: the preset is the default, on, and a
// script may turn it off.
//
// Measured 2026-09-28 against /opt/homebrew/bin/zsh — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m`: *not a Go executable* — each
// construct run with the option **on**, **off**, and left at its default
// (#4436).
//
// **Every row that moves is an `unsetopt` row.** The option is on by default,
// so the nine rows with it on and the nine with it left alone agree under
// either reading; a grid that never turned it off reported no difference at
// all. That is why the table below is written as a pair per construct rather
// than as a list of calls.
func TestFunctionArgZeroDecidesWhatDollarZeroNames(t *testing.T) {
	for _, tc := range []struct{ name, body, on string }{
		{"a name() function", `f() { print "0=[$0]" }` + "\nf", "f"},
		{"a function keyword one", `function f { print "0=[$0]" }` + "\nf", "f"},
		{"an anonymous function", `() { print "0=[$0]" }`, "(anon)"},
		{"a nested call", `g() { print "0=[$0]" }` + "\n" + `f() { g }` + "\nf", "g"},
		{"an eval inside one", `f() { eval 'print "0=[$0]"' }` + "\nf", "f"},
		{"a subshell inside one", `f() { ( print "0=[$0]" ) }` + "\nf", "f"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// On, and left alone: the innermost call answers.
			for _, pre := range []string{"", "setopt functionargzero\n"} {
				out, _ := runZsh(t, t.TempDir(), pre+tc.body)
				if strings.TrimSpace(out) != "0=["+tc.on+"]" {
					t.Errorf("on: %q = %q, want 0=[%s]", pre+tc.body, out, tc.on)
				}
			}
			// Off: the shell's own name, which here is the script the
			// harness wrote. Asserted as "not the call's name", because the
			// path is the harness's and not the measurement's.
			out, _ := runZsh(t, t.TempDir(), "unsetopt functionargzero\n"+tc.body)
			if strings.Contains(out, "0=["+tc.on+"]") {
				t.Errorf("off: %q = %q, want the shell's own name", tc.body, out)
			}
			if !strings.Contains(out, "0=[") {
				t.Errorf("off: %q = %q, want a `0=[…]` line at all", tc.body, out)
			}
		})
	}
}

// TestFunctionArgZeroReachesASourcedFileToo: the row that settles the noun.
//
// The option is spelled `function`argzero, and a rule keyed on that word
// would leave a sourced file naming itself. It does not: with the option off,
// a sourced file reports the shell's own name, from the top level and from
// inside a function alike.
func TestFunctionArgZeroReachesASourcedFileToo(t *testing.T) {
	const inc = `print 'print "0=[$0]"' > inc.sh` + "\n"
	for _, tc := range []struct{ name, body string }{
		{"sourced at the top level", inc + ". ./inc.sh"},
		{"and sourced from a function", inc + `f() { . ./inc.sh }` + "\nf"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), tc.body)
			if strings.TrimSpace(out) != "0=[./inc.sh]" {
				t.Errorf("on: %q, want 0=[./inc.sh]", out)
			}
			out, _ = runZsh(t, t.TempDir(), "unsetopt functionargzero\n"+tc.body)
			if strings.Contains(out, "0=[./inc.sh]") {
				t.Errorf("off: %q, want the shell's own name and not the file", out)
			}
		})
	}
}

// TestTheTopLevelDollarZeroIsTheControl: with no call on the stack the option
// changes nothing, which is what says the rows above are about the *call* and
// not about `$0` wholesale.
func TestTheTopLevelDollarZeroIsTheControl(t *testing.T) {
	on, _ := runZsh(t, t.TempDir(), `setopt functionargzero`+"\n"+`print "0=[$0]"`)
	off, _ := runZsh(t, t.TempDir(), `unsetopt functionargzero`+"\n"+`print "0=[$0]"`)
	if on != off {
		t.Errorf("top level moved with the option: on=%q off=%q", on, off)
	}
}
