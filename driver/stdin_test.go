// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// A script arriving on standard input has no $0 to name, and the Stdin
// location fields let a dialect change shape there rather than substitute a
// name: down to the builtin's own name, or to a bracketed line the dialect
// uses nowhere else.
func TestStdinReshapesTheDiagnosticPrefix(t *testing.T) {
	for _, c := range []struct {
		name string
		dg   interp.Diagnostics
		want string
	}{
		{
			name: "the builtin speaks for itself",
			dg: interp.Diagnostics{
				Location:             interp.LocationTightLine,
				StdinLocation:        interp.LocationNameOnly,
				StdinBuiltinLocation: interp.LocationBuiltinNameOnly,
				ShiftTooMany:         "shift count is too big",
			},
			want: "shift: shift count is too big\n",
		},
		{
			name: "the line moves into brackets",
			dg: interp.Diagnostics{
				StdinBuiltinLocation: interp.LocationBracketLine,
				ShiftTooMany:         "shift: %[2]s: bad number",
			},
			want: "testsh[1]: shift: (null): bad number\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			sh := shell()
			sh.Semantics = interp.PosixSemantics()
			sh.Diagnostics = c.dg
			var o, e bytes.Buffer
			sh.Stdout, sh.Stderr = &o, &e
			driver.RunStdin(sh, "shift\n")
			if got := e.String(); got != c.want {
				t.Errorf("stderr %q, want %q", got, c.want)
			}
		})
	}

	// The same failure through -c keeps the ordinary prefix: the Stdin
	// fields are the route's, not the dialect's default.
	sh := shell()
	sh.Semantics = interp.PosixSemantics()
	sh.Diagnostics = interp.Diagnostics{
		Location:             interp.LocationTightLine,
		StdinLocation:        interp.LocationNameOnly,
		StdinBuiltinLocation: interp.LocationBuiltinNameOnly,
		ShiftTooMany:         "shift count is too big",
	}
	_, errs, _ := runArgs(t, sh, "testsh", "-c", "shift")
	if want := "testsh:1: shift count is too big\n"; errs != want {
		t.Errorf("-c stderr %q, want %q", errs, want)
	}
}
