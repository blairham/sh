// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// A substitution body that will not parse is refused while the command is
// *running*, so the front end never sees it — and it is still a parse failure
// on the same input, which one dialect names in the location (#3467).
//
// The front end's own refusal already carried the label; this is the same
// label reaching the message the interpreter writes, through Runner.InputName.
// They are the same string by construction rather than by agreement, which is
// the whole reason the runner is told rather than the interpreter deciding:
// nothing under interp knows a shell has a `-c` at all.
func TestASubstitutionRefusedUnderACommandStringNamesTheOrigin(t *testing.T) {
	run := func(t *testing.T, src string) string {
		t.Helper()
		sh := shell()
		dg := interp.Diagnostics{}
		// The dialect answer this turns on, and the wording the location
		// goes in front of.
		dg.NamesTheInputInLocation = true
		dg.SyntaxUnexpected = "syntax error near unexpected token `%[1]s'"
		sh.Diagnostics = dg
		var o, e bytes.Buffer
		sh.Stdout, sh.Stderr = &o, &e
		driver.RunCommand(sh, src, nil)
		return e.String()
	}
	t.Run("the origin is named", func(t *testing.T) {
		errs := run(t, "v=$(echo hi; for)")
		if !strings.Contains(errs, "-c: ") {
			t.Errorf("err = %q, want the origin named", errs)
		}
	})
	t.Run("and a dialect that names no origin is unchanged", func(t *testing.T) {
		sh := shell()
		sh.Diagnostics = interp.Diagnostics{
			SyntaxUnexpected: "syntax error near unexpected token `%[1]s'",
		}
		var o, e bytes.Buffer
		sh.Stdout, sh.Stderr = &o, &e
		driver.RunCommand(sh, "v=$(echo hi; for)", nil)
		if strings.Contains(e.String(), "-c") {
			t.Errorf("err = %q, want no origin named", e.String())
		}
	})
}
