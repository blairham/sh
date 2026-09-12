// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/dialecttest"
)

// A length over `$!` reads here and is the length of an empty `$!`, which is
// `0`. Measured 2026-09-12 on bash 5.3.15 from a script file under `env -i`
// with a scratch HOME:
//
//	$ bash s.sh         # echo "[${#!}]"
//	[0]
//
// bash 3.2, bash-as-`sh`, dash, ksh93 and BusyBox ash agree. zsh refuses the
// shape outright, even though `$!` reads there and is `0` (#2415).
func TestALengthOverTheBangNameReadsHere(t *testing.T) {
	if bash.Dialect().ParamLengthRefusesTheBangName {
		t.Error("bash takes `!` as a name behind a `${#`")
	}
	for _, tc := range []struct{ src, out string }{
		{`echo "[${#!}]"`, "[0]\n"},
		// The value being measured is empty here, which is the whole of why
		// the answer is `0` and not the `1` a `$!` of `0` would give.
		{`echo "[$!]"`, "[]\n"},
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
