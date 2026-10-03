// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// Through the front end, an element named by text has its subscript
// expanded before its arithmetic reads it. Measured 2026-10-03 under `-c`
// (#5578). See interp.Semantics.ReferencedSubscriptIsExpanded.
func TestAReferencedSubscriptIsExpandedThroughTheFrontEnd(t *testing.T) {
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
	driver.MainArgs(sh, []string{"bash", "-c", `a=(x y z); i=1; unset "a[\$i]"; echo ${a[@]}; b=(x y z); declare -n r="b[\$i]"; r=Q; echo ${b[@]}`})
	if got, want := out.String(), "x z\nx Q z\n"; got != want || errs.Len() > 0 {
		t.Errorf("got %q, %q, want %q", got, errs.String(), want)
	}
}
