// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

func optRun(t *testing.T, tweak func(*Semantics), dg Diagnostics, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.BadOptionToSpecialBuiltinFatal = No
	if tweak != nil {
		tweak(&sem)
	}
	r := &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}

// The bug: an option a builtin does not have was dropped without a word, so
// `export -Q x=1` exported x and said nothing about the -Q. A typo, or a shell
// whose options are not the ones the script was written for, went unnoticed.
func TestAnOptionABuiltinDoesNotHaveIsRefused(t *testing.T) {
	for _, name := range []string{"export", "readonly", "unset"} {
		out, st := optRun(t, nil, Diagnostics{}, name+` -Q x=1; echo "st=$?"; echo "[${x-unset}]"`)
		if !strings.Contains(out, "-Q") {
			t.Errorf("%s: said %q, want the option named", name, out)
		}
		if !strings.Contains(out, "st=2") {
			t.Errorf("%s: said %q, want status 2", name, out)
		}
		// And it did not go on to do the work.
		if strings.Contains(out, "[1]") {
			t.Errorf("%s: said %q, want the command not to have run", name, out)
		}
		_ = st
	}
}

// The options they do have still work, which is the half a refusal could break.
func TestTheOptionsTheyDoHaveStillWork(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`x=1; export x; unset -v x; echo "[${x-unset}]"`, "[unset]"},
		{`export -- y=2; echo "[$y]"`, "[2]"},
		{`unset -- z; echo "st=$?"`, "st=0"},
		{`readonly -- w=3; echo "[$w]"`, "[3]"},
		// A bare `-` is an operand, not an option.
		{`unset -; echo "st=$?"`, "st=0"},
	} {
		if out, _ := optRun(t, nil, Diagnostics{}, c.src); !strings.Contains(out, c.want) {
			t.Errorf("%s: said %q, want %q", c.src, out, c.want)
		}
	}
}

// Where the dialect says a special builtin's failure is fatal, the script stops
// there rather than carrying on with the next command.
func TestABadOptionCanEndTheScript(t *testing.T) {
	out, _ := optRun(t, func(s *Semantics) { s.BadOptionToSpecialBuiltinFatal = Yes },
		Diagnostics{}, `export -Q x; echo after`)
	if strings.Contains(out, "after") {
		t.Errorf("said %q, want the script to have stopped", out)
	}
	out, _ = optRun(t, func(s *Semantics) { s.BadOptionToSpecialBuiltinFatal = No },
		Diagnostics{}, `export -Q x; echo after`)
	if !strings.Contains(out, "after") {
		t.Errorf("said %q, want the script to carry on", out)
	}
}

// The usage line is per builtin and only two of the four print one.
func TestTheUsageLineFollowsWhereTheDialectPrintsOne(t *testing.T) {
	dg := Diagnostics{
		BuiltinBadOption: "%[1]s: %[2]s: invalid option",
		BuiltinUsage:     map[string]string{"export": "export: usage: export [-fn]"},
	}
	out, _ := optRun(t, nil, dg, `export -Q x`)
	if !strings.Contains(out, "export: usage: export [-fn]") {
		t.Errorf("said %q, want the usage line", out)
	}
	// A builtin with no entry gets none, rather than another's.
	out, _ = optRun(t, nil, dg, `unset -Q x`)
	if strings.Contains(out, "usage") {
		t.Errorf("said %q, want no usage line for a builtin with no entry", out)
	}
	// And with no map at all, none.
	out, _ = optRun(t, nil, Diagnostics{BuiltinBadOption: "%[1]s: %[2]s: invalid option"}, `export -Q x`)
	if strings.Contains(out, "usage") {
		t.Errorf("said %q, want no usage line", out)
	}
}
