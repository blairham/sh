// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A function whose name cannot be carried in the environment is refused by
// `export -f`, and the refusal does not stop the operand list.
//
// Measured 2026-09-23 on bash 5.3.20 and identically on the 5.3.15 in the image
// this shell's own suite is graded in. Found as four lines of `exportfunc.tests`
// — bash's own test for function names that cannot be exported — where this
// shell exported one at status 0, put `BASH_FUNC_foo=bar%%` in the environment,
// and the child then **imported and ran it** (#4143).
func TestExportRefusesAFunctionNameItCannotCarry(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name, which string
		refused     bool
	}{
		// The two the reference refuses, and the argument does not predict the
		// second: an environment entry's name may not hold an `=`, and `/` is
		// this shell's own line.
		{name: "an equals sign", which: "foo=bar", refused: true},
		{name: "a path", which: "/bin/echo", refused: true},
		// Everything else goes, which is what keeps the rule measured rather
		// than "a name that is not an identifier".
		{name: "a hyphen", which: "a-b"},
		{name: "a dot", which: "a.b"},
		{name: "a plus", which: "a+b"},
		{name: "a colon", which: "a:b"},
		{name: "a bang", which: "a!b"},
		{name: "an at sign", which: "@x"},
		{name: "a plain name", which: "ok_name"},
	} {
		t.Run(c.name, func(t *testing.T) {
			// The definition unquoted and the export quoted, which is the
			// shape bash's own suite writes: quoting the name on the
			// `function` line makes the quotes part of it.
			out, st := runBash(t, dir, "function "+c.which+" { :; }\nexport -f '"+c.which+"'\necho \"st=$?\"\n")
			if !c.refused {
				if strings.TrimSpace(out) != "st=0" {
					t.Errorf("output %q, want st=0 — this name exports", out)
				}
				return
			}
			wantWholeLines(t, out, "bash: line 2: export: "+c.which+": cannot export")
			if !strings.Contains(out, "st=1") {
				t.Errorf("output %q, want status 1", out)
			}
			_ = st
		})
	}
}

// Three things about the refusal, each measured and each a row a reading of the
// rule alone would get wrong.
func TestTheExportRefusalsShape(t *testing.T) {
	dir := t.TempDir()
	// It comes **after** the not-a-function check: a name holding an `=` that is
	// not a function draws `not a function` and not this.
	out, _ := runBash(t, dir, "export -f 'nosuch=name'\necho \"st=$?\"\n")
	wantWholeLines(t, out, "bash: line 1: export: nosuch=name: not a function")
	// It does not stop the operand list: the good name is exported and the
	// status is 1 at the end.
	out, _ = runBash(t, dir, "function ok { :; }\nfunction a=b { :; }\nexport -f 'a=b' ok\necho \"st=$?\"\nexport -pf\n")
	if !strings.Contains(out, "st=1") {
		t.Errorf("output %q, want status 1", out)
	}
	wantWholeLines(t, out, "declare -fx ok")
	if strings.Contains(out, "a=b") && !strings.Contains(out, "cannot export") {
		t.Errorf("output %q, want the refused name nowhere in the listing", out)
	}
	// And it is only the exporting direction — nothing is being put anywhere by
	// `export -nf`, so a name that could not be carried is not a refusal there.
	out, _ = runBash(t, dir, "function a=b { :; }\nexport -nf 'a=b'\necho \"st=$?\"\n")
	if strings.TrimSpace(out) != "st=0" {
		t.Errorf("output %q, want st=0", out)
	}
}

// `export -nf` takes the attribute back off, which it did not read at all.
//
// Measured: `export -f ab; export -nf ab` leaves nothing in the environment in
// bash, and this shell went on handing the function to every child — the letter
// reached the variable path and never the function one (#4143).
func TestExportMinusNTakesAFunctionOutOfTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	out, _ := runBash(t, dir, "function ab { :; }\nexport -f ab\nexport -nf ab\nexport -pf\necho done\n")
	for _, line := range strings.Split(out, "\n") {
		if line == "declare -fx ab" {
			t.Errorf("output %q, want the function no longer exported", out)
		}
	}
	// A name that is not a function is still refused either way round, which is
	// why that check stands in front of the split.
	out, _ = runBash(t, dir, "export -nf nosuch\necho \"st=$?\"\n")
	wantWholeLines(t, out, "bash: line 1: export: nosuch: not a function")
	if !strings.Contains(out, "st=1") {
		t.Errorf("output %q, want status 1", out)
	}
}
