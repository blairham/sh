// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// **A picked descriptor starts at 10 where the program is read from standard
// input, and at 11 everywhere else** (#5133). Measured 2026-10-01 on zsh 5.9.2
// with `-f < script` against `-c` and a script file: 10 is the script's own
// descriptor on the file route and is free on this one, so `exec {x}<f`,
// `sysopen -u` and `<(…)` all hand it out — and hand it out again once it is
// closed. The command-string row is the control.
func TestAPickedDescriptorStartsAtTenOnStandardInput(t *testing.T) {
	for _, c := range []struct {
		name        string
		route       interp.Route
		interactive bool
		src         string
		want        string
	}{
		{"exec {x}", interp.RouteStandardInput, false, `exec {x}</dev/null; exec {y}</dev/null; exec {x}<&-; exec {z}</dev/null; echo $x $y $z`, "10 11 10\n"},
		{"sysopen", interp.RouteStandardInput, false, `zmodload zsh/system; sysopen -r -u a /dev/null; echo $a`, "10\n"},
		{"a process substitution", interp.RouteStandardInput, false, `echo <(true)`, "/dev/fd/10\n"},
		{"inside a substitution", interp.RouteStandardInput, false, `echo $(exec {x}</dev/null; echo $x)`, "10\n"},
		{"control: a command string", interp.RouteCommandString, false, `exec {x}</dev/null; echo $x; echo <(true)`, "11\n/dev/fd/12\n"},
		// Standard input with somebody typing at it is the terminal's route:
		// `zsh -fi` on a pseudo-terminal says 11, the terminal holding 10.
		{"control: an interactive session", interp.RouteStandardInput, true, `exec {x}</dev/null; echo $x`, "11\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir(), Route: c.route, Interactive: c.interactive}, c.src)
			if err != nil {
				t.Fatal(err)
			}
			if out != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}
