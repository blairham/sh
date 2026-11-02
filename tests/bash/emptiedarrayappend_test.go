// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// An append to an indexed array emptied by `unset` starts at the first
// position, exactly as one to an array that never held an element does —
// however many elements went and wherever they stood. Measured 2026-11-02 on
// bash 5.3.20 under `-c`, this script, byte for byte. It is the other answer
// to interp.Semantics.AppendToAnEmptiedArrayStartsPastTheFirstPosition, the
// one ksh93 gives the opposite way (#7393).
func TestAnAppendToAnEmptiedArrayStartsAtTheFirstPosition(t *testing.T) {
	t.Parallel()
	src := "e=(7); unset 'e[0]'; e+=(z); declare -p e\n" +
		"e=(7 8); unset 'e[0]' 'e[1]'; e+=(z); declare -p e\n" +
		"e=([5]=7); unset 'e[5]'; e+=(z); declare -p e\n" +
		"e=(7); unset 'e[0]'; e+=(y z); declare -p e\n" +
		"e=(7); e=(); e+=(z); declare -p e\n" +
		"e=(7 8); unset 'e[1]'; e+=(z); declare -p e\n"
	want := `declare -a e=([0]="z")` + "\n" +
		`declare -a e=([0]="z")` + "\n" +
		`declare -a e=([0]="z")` + "\n" +
		`declare -a e=([0]="y" [1]="z")` + "\n" +
		`declare -a e=([0]="z")` + "\n" +
		`declare -a e=([0]="7" [1]="z")` + "\n"
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if out != want || st != 0 {
		t.Errorf("status %d, got\n%s\nwant\n%s", st, out, want)
	}
}
