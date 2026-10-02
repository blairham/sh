// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A `)` left over after a complete expression is the ordinary syntax error
// here, not the invalid-operator one: measured 2026-10-02 on bash 5.3.20,
// `foo="3)"; echo $((foo))` is `3): arithmetic syntax error in expression
// (error token is ")")`. See syntax.ErrArithUnmatchedCloseParen (#5145).
func TestAStrayCloseParenIsASyntaxError(t *testing.T) {
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	driver.MainArgs(sh, []string{"bash", "-c", `foo="3)"; echo $((foo))`})
	if want := `3): arithmetic syntax error in expression (error token is ")")`; !strings.Contains(errs.String(), want) {
		t.Errorf("got %q, want it to hold %q", errs.String(), want)
	}
}
