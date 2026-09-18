// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/internal/testenv"
)

// An operand written with a slash in it was never searched for, so a dialect
// whose external sentence says *how* the name was found has a second wording
// for it — see Diagnostics.TypePathnameOperand.
//
// The field is empty in four of the five columns, whose two wordings are the
// same string, and the rows below are what says an empty one changes nothing.
func TestAPathnameOperandCanHaveASentenceOfItsOwn(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "onlyhere")
	if err := testenv.WriteExecutable(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, external, pathname string
		src, want                string
	}{
		{
			"the searched name keeps the dialect's own sentence",
			"%[1]s is found on the path at %[2]s", "%[1]s is %[2]s",
			`command -V onlyhere`, "onlyhere is found on the path at " + exe + "\n",
		},
		{
			"and a pathname operand takes the second wording",
			"%[1]s is found on the path at %[2]s", "%[1]s is %[2]s",
			`command -V ./onlyhere`, "./onlyhere is " + dir + "/./onlyhere\n",
		},
		{
			"an empty second wording leaves both to the first",
			"%[1]s is found on the path at %[2]s", "",
			`command -V ./onlyhere`, "./onlyhere is found on the path at " + dir + "/./onlyhere\n",
		},
		{
			"and `type` writes the same line",
			"%[1]s is found on the path at %[2]s", "%[1]s is %[2]s",
			`type ./onlyhere`, "./onlyhere is " + dir + "/./onlyhere\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			setup := func(r *Runner) {
				r.Env = []string{"PATH=" + dir}
				r.Dir = dir
				dg := Diagnostics{}
				if r.Diagnostics != nil {
					dg = *r.Diagnostics
				}
				dg.TypeExternal = c.external
				dg.TypePathnameOperand = c.pathname
				r.Diagnostics = &dg
			}
			if out, _ := run(t, c.src, setup); out != c.want {
				t.Errorf("%s = %q, want %q", c.src, out, c.want)
			}
		})
	}
}
