// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// bash keeps the array letter and makes the array of the one word. Measured
// 2026-10-03 on bash 5.3.20 under `-c` (#5630). See
// interp.Semantics.ArrayLetterWithAWordDeclaresAScalar.
func TestAnArrayLetterWithAWordMakesAnArrayOfOne(t *testing.T) {
	if out, st := runBash(t, t.TempDir(), `declare -a x=/y; declare -p x`); out != "declare -a x=([0]=\"/y\")\n" || st != 0 {
		t.Errorf("got %q at %d", out, st)
	}
}
