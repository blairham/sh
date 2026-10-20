// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// ksh93 reads the subscript of an element named by text as it arrived, so a
// `$` in it is a character the arithmetic refuses, and nothing is touched.
// The arithmetic's own route is the control: there an arrived `$` is
// expanded. Measured 2026-10-03 on ksh93u+ 2012-08-01 under `-c` (#5578). See
// interp.Semantics.ReferencedSubscriptIsExpanded.
func TestAReferencedSubscriptIsReadAsItArrived(t *testing.T) {
	out, _ := runKshEmptyArray(t, `a=(x y z); i=1; unset "a[\$i]"; echo ${a[@]}; read "a[\$i+1]" <<<R; echo ${a[@]}; b=(1 2 3); e="b[\$i]"; echo $(( $e ))`)
	for _, want := range []string{"unset: $i: arithmetic syntax error\n", "x y z\n", "read: $i+1: arithmetic syntax error\n", "x y z\n2\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q, want it to hold %q", out, want)
		}
	}
}
