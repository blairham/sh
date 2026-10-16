// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **A cursor past the word after the last** — Semantics.GetoptsCursorPastTheEnd.
// Measured 2026-10-15 under `-c` and from a script file against bash 5.3.20 and
// 3.2.57, ksh93u+ 2012-08-01, dash 0.5.12, BusyBox ash 1.37.0 (the pinned
// alpine image) and zsh 5.9.2. Each row prints what two calls leave, `$? $o
// $OPTIND` a call (#6575).
//
// The first row is the control: a cursor *at* `$# + 1` is the ordinary end of
// the options everywhere. The fourth holds the cursor at 5 and scans two given
// operands, so the count of the words scanned, and not `$#`, is what decides.
// The fifth reaches a cursor past the end with no assignment at all: the scan
// of the parameters leaves OPTIND at 3, and the next call is handed one
// operand. Neither goes through `set --` or `shift`, which restart the scan by
// themselves in dash and ash.
func TestGetoptsCursorPastTheEnd(t *testing.T) {
	const two = `; echo "$? $o $OPTIND"; getopts abc o; echo "$? $o $OPTIND"`
	lines := []string{
		"set -- -a -b -c; OPTIND=4; getopts abc o" + two,
		"set -- -a -b -c; OPTIND=5; getopts abc o" + two,
		"set -- -a -b -c; OPTIND=100; getopts abc o" + two,
		`OPTIND=5; getopts ab o -a -b; echo "$? $o $OPTIND"; getopts ab o -a -b; echo "$? $o $OPTIND"`,
		`set -- -a -b x; getopts ab o; getopts ab o; getopts ab o; getopts ab o -b; echo "$? $o $OPTIND"; getopts ab o -b; echo "$? $o $OPTIND"`,
	}
	want := map[string][]string{
		"bash": {"1 ? 4|1 ? 4", "1 ? 4|1 ? 4", "1 ? 4|1 ? 4", "1 ? 3|1 ? 3", "1 ? 2|1 ? 2"},
		"ksh":  {"1 ? 4|1 ? 4", "1 ? 5|1 ? 5", "1 ? 100|1 ? 100", "1 ? 5|1 ? 5", "1 ? 3|1 ? 3"},
		"zsh":  {"1  4|1  4", "1  5|1  5", "1  100|1  100", "1  5|1  5", "1 b 3|1 b 3"},
		"dash": {"1 ? 4|1 ? 4", "0 a 2|0 b 3", "0 a 2|0 b 3", "0 a 2|0 b 3", "0 b 2|1 ? 2"},
		"ash":  {"1 ? 4|1 ? 4", "0 a 2|0 b 3", "0 a 2|0 b 3", "0 a 2|0 b 3", "0 b 2|1 ? 2"},
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
