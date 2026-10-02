// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// `inf` is a name like any other here: measured 2026-10-02 on bash 5.3.20,
// `inf=4; echo $(( inf )) $(( Inf ))` writes `4 0`. See
// interp.Semantics.ArithInfAndNaNAreConstants (#5145).
func TestInfIsAName(t *testing.T) {
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	driver.MainArgs(sh, []string{"bash", "-c", `inf=4; echo $(( inf )) $(( Inf ))`})
	if got, want := out.String(), "4 0\n"; got != want || errs.Len() != 0 {
		t.Errorf("got %q, %q, want %q", got, errs.String(), want)
	}
}
