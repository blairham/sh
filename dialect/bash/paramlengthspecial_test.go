// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/dialecttest"
)

// Where a length over `$-` or `$?` cannot use the whole expansion, this shell
// falls back and reads the `#` as the parameter `$#` with an operator on it.
// Measured 2026-09-12 on bash 5.3.15 from a script file under `env -i` with a
// scratch HOME, `set -- p q r` so `$#` is 3 and `$-` is two characters:
//
//	$ bash s.sh         # echo "${#-w}"
//	3
//	$ bash s.sh         # echo "${#-}"
//	2
//
// dash, ksh93, bash 3.2, bash-as-`sh` and BusyBox ash agree; zsh keeps the
// name and answers 4 and `bad substitution` (#1242).
func TestALengthOverASpecialNameYieldsToTheParameterHere(t *testing.T) {
	if bash.Dialect().ParamLengthOverASpecialNameIsFinal {
		t.Error("bash falls back to `$#` when text is left over behind the name")
	}
	for _, tc := range []struct{ src, out string }{
		{`set -- p q r; echo "${#-w}"`, "3\n"},
		{`set -- p q r; echo "${#?w}"`, "3\n"},
		{`set -- p q r; echo "${#-:-x}"`, "3\n"},
		{`set -- p q r; echo "${#?:-x}"`, "3\n"},
		// Bare, the name wins here too: `$-` is `hB` under `-c`, so this is a
		// length and not the 3 the fallback would give.
		{`set -- p q r; echo "${#?}"`, "1\n"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		if err != nil {
			t.Errorf("%q: %v", tc.src, err)
			continue
		}
		if out != tc.out {
			t.Errorf("%q: out = %q, want %q", tc.src, out, tc.out)
		}
	}
}
