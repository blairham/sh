// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **`set --` and `shift` restart the scan** —
// Semantics.GetoptsNewParametersRestartTheScan. Measured 2026-10-16 under `-c`
// against dash 0.5.12, BusyBox ash 1.37.0 (the pinned alpine image), bash
// 5.3.20, ksh93u+ and zsh 5.9.2 (#6653). Every row starts from two calls over
// `-a -b -c` and ends `echo "$? $o $OPTIND"`.
//
// The third row is OPTIND alone: the parameter is not reset, only the scan's
// position. The ninth and tenth order an assignment either side of the
// replacement, and the last three are controls — a function call with
// arguments, a `set --` inside one, and the `shift $((OPTIND-1))` idiom after
// a loop, none of which may change.
func TestGetoptsNewParametersRestartTheScan(t *testing.T) {
	const s = "set -- -a -b -c; getopts abcd o; getopts abcd o; "
	const e = `; echo "$? $o $OPTIND"`
	lines := []string{
		s + "set -- -a -b -c -d; getopts abcd o" + e,
		s + "shift; getopts abcd o" + e,
		s + "shift; echo $OPTIND",
		s + "set --; getopts abcd o -a -b -c -d" + e,
		s + "shift 0; getopts abcd o" + e,
		`set -- -abc; getopts abc o; set -- -abc; getopts abc o` + e,
		`f() { set -- -a -b -c; getopts abcd o; getopts abcd o; set -- -a -b -c -d; getopts abcd o` + e + `; }; f`,
		s + "OPTIND=3; set -- -a -b -c -d; getopts abcd o" + e,
		s + "set -- -a -b -c -d; OPTIND=3; getopts abcd o" + e,
		"f() { :; }; " + s + "f x y; getopts abcd o" + e,
		"f() { set -- q; }; " + s + "f; getopts abcd o" + e,
		`set -- -a -b x; while getopts ab o; do :; done; shift $((OPTIND-1)); echo "$# $OPTIND"`,
	}
	want := map[string][]string{
		"dash": {"0 a 2", "0 b 2", "3", "0 a 2", "0 a 2", "0 a 2", "0 a 2", "0 a 2", "0 c 4", "0 c 4", "0 c 4", "1 3"},
		"ash":  {"0 a 2", "0 b 2", "3", "0 a 2", "0 a 2", "0 a 2", "0 a 2", "0 a 2", "0 c 4", "0 c 4", "0 c 4", "1 3"},
		"bash": {"0 c 4", "1 ? 3", "3", "0 c 4", "0 c 4", "0 b 1", "0 c 4", "0 c 4", "0 c 4", "0 c 4", "0 c 4", "1 3"},
		"ksh":  {"0 c 4", "1 ? 3", "3", "0 c 4", "0 c 4", "0 b 1", "0 c 4", "0 c 4", "0 c 4", "0 c 4", "0 c 4", "1 3"},
		"zsh":  {"0 c 3", "1 b 3", "2", "0 c 3", "0 c 3", "0 b 1", "0 c 3", "0 d 4", "0 d 4", "0 c 3", "0 c 3", "1 3"},
	}
	for name, rows := range want {
		for i, w := range rows {
			out, _, err := presets[name].Combined(t, dialecttest.Base{Dir: t.TempDir()}, lines[i]+"\n")
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.ReplaceAll(strings.TrimSuffix(out, "\n"), "\n", "|"); got != w {
				t.Errorf("%s: %s: got %q, want %q", name, lines[i], got, w)
			}
		}
	}
}
