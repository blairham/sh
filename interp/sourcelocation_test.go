// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A dialect may name the file a failing line was read from instead of the
// script: the sourced file while it runs, and the file a function was defined
// in when the function is called after the sourcing has finished. The name is
// the operand as the shell constructed it — `./inc.sh` as written — never the
// resolved absolute path, which would name a different string on every
// machine.
func TestALocationMayNameTheCurrentFile(t *testing.T) {
	for _, c := range []struct {
		name, inc, src, want string
	}{
		{
			// The issue's own shape: the line was already the sourced
			// file's, and the name must be too.
			"a failure at the top level of a sourced file",
			"nosuchcmd\n",
			". ./inc.sh\n",
			"./inc.sh:1: ",
		},
		{
			// The `.` sits on line 1 of the caller and the failure on line 3
			// of the file, so a test that reported the caller's line — or
			// the caller's name — could not pass by accident.
			"the sourced file's own line, not the caller's",
			"# a\n# b\nnosuchcmd\n",
			"# outer\n. ./inc.sh\n",
			"./inc.sh:3: ",
		},
		{
			// The function remembers its defining file after the sourcing
			// has finished, which is what r.funcFiles exists for.
			"a function defined in a sourced file, called later",
			"f() {\n  nosuchcmd\n}\n",
			". ./inc.sh\ntrue\nf\n",
			"./inc.sh:2: ",
		},
		{
			// The control: outside any sourcing the name does not move.
			"no sourcing, so the shell's own name",
			"",
			"true\nnosuchcmd\n",
			"testsh:2: ",
		},
		{
			// And after the sourced file finishes, the name is given back.
			"after the sourcing finishes",
			"true\n",
			". ./inc.sh\nnosuchcmd\n",
			"testsh:2: ",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			dg := Diagnostics{Location: LocationTightLine, LocationNamesTheCurrentFile: true}
			if got := sourceLocRun(t, c.inc, c.src, dg); !strings.HasPrefix(got, c.want) {
				t.Errorf("said %q, want it to start %q", got, c.want)
			}
		})
	}
}

// Without the answer, the shell's own name stays whatever file the line came
// from — which is what this printed for every dialect before the field
// existed, and is still two of the four.
func TestWithoutThatAnswerTheShellIsNamed(t *testing.T) {
	dg := Diagnostics{Location: LocationTightLine}
	got := sourceLocRun(t, "nosuchcmd\n", ". ./inc.sh\n", dg)
	if !strings.HasPrefix(got, "testsh:1: ") {
		t.Errorf("said %q, want it to start %q", got, "testsh:1: ")
	}
}

// The function's name wins over the file's where a dialect gives both
// answers: a failure inside the function names the function, and one at the
// sourced file's top level names the file. That is one dialect's measured
// pairing, asked here as the two flags rather than as the shell.
func TestTheFunctionNameWinsOverTheCurrentFile(t *testing.T) {
	dg := Diagnostics{
		Location:                    LocationTightLine,
		LocationNamesTheCurrentFile: true,
		LocationNamesTheFunction:    true,
	}
	got := sourceLocRun(t, "f() {\n  nosuchcmd\n}\nnosuchcmd\n", ". ./inc.sh\nf\n", dg)
	// The top level of the sourced file first, then the function.
	if !strings.HasPrefix(got, "./inc.sh:4: ") {
		t.Errorf("top level: said %q, want it to start %q", got, "./inc.sh:4: ")
	}
	if !strings.Contains(got, "\nf:1: ") {
		t.Errorf("in the function: said %q, want a line starting %q", got, "f:1: ")
	}
}

// sourceLocRun writes inc (when there is one) as inc.sh in a directory of its
// own, runs src there, and returns standard error.
func sourceLocRun(t *testing.T, inc, src string, dg Diagnostics) string {
	t.Helper()
	dir := t.TempDir()
	if inc != "" {
		if err := os.WriteFile(filepath.Join(dir, "inc.sh"), []byte(inc), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem := PosixSemantics()
	sem.DotMissingFileFatal = No
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "testsh",
		Stdout: &bytes.Buffer{}, Stderr: &buf, Dir: dir,
		// PATH deliberately empty of the directory: `.` reads `./inc.sh` as
		// a path, and nothing else here should be findable.
		Vars: map[string]string{"PATH": ""},
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
