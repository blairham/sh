// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// The other side of interp.Semantics.HashClearRefusesOperands: here `hash -r
// name` is two actions one call may ask for — clear the table, then hash that
// name — and the table comes back holding it.
//
// This is the control that makes the axis an axis. Measured 2026-09-26 under
// `env -i PATH=/usr/bin:/bin`: bash 5.3.20 and ksh93u+ are both a silent 0
// with the entry there, where zsh 5.9.2 is `too many arguments` at 1 with the
// table untouched. Without a row on this side, a change that refused the
// combination everywhere would pass zsh's own tests (#4744).
func TestHashClearStillHashesTheNameBesideIt(t *testing.T) {
	base := dialecttest.Base{Dir: t.TempDir(), Vars: map[string]string{"PATH": "/usr/bin:/bin"}}
	out, st, err := preset.Combined(t, base, "hash -r ls\necho st=$?\nhash\n")
	if err != nil {
		t.Fatal(err)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
	if !strings.Contains(out, "st=0") || !strings.Contains(out, "ls") {
		t.Errorf("out = %q, want st=0 and the listing naming ls", out)
	}
	// The positive control for the row above: the same listing after a bare
	// `hash -r` holds nothing, so "ls is in there" is the operand's doing
	// rather than a listing that always writes something.
	base.Dir = t.TempDir()
	out, st, err = preset.Combined(t, base, "hash ls\nhash -r\nhash\n")
	if err != nil {
		t.Fatal(err)
	}
	if st != 0 || strings.Contains(out, "/ls") {
		t.Errorf("after a bare hash -r the table = %q at %d, want no entry", out, st)
	}
}
