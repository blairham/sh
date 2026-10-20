// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"fmt"
	"strings"
	"testing"
)

// This dialect held dash's `set -o` table: dash's names, sorted. Three things
// were wrong at once and the listing is the one a script can see (#3366).
//
// Measured 2026-09-17 inside the pinned image
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b,
// BusyBox v1.37.0, a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`.

// theListing is BusyBox's own, in its own order — not a sort, and not dash's
// fourteen names. `errtrace` and `pipefail` are here and were missing; `emacs`
// and `nolog` are not options in this shell and were being written.
var theListing = []string{
	"errexit", "noglob", "ignoreeof", "monitor", "noexec", "xtrace",
	"verbose", "noclobber", "allexport", "notify", "nounset",
	"errtrace", "vi", "pipefail",
}

func TestTheOptionListingIsThisShellsOwnTable(t *testing.T) {
	out, status := run(t, "set -o\n")
	if status != 0 {
		t.Fatalf("set -o = %d: %s", status, out)
	}
	var got []string
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		name, _, ok := strings.Cut(line, " ")
		if !ok {
			t.Fatalf("row %q is not `name<pad>state`", line)
		}
		got = append(got, name)
	}
	if strings.Join(got, " ") != strings.Join(theListing, " ") {
		t.Errorf("set -o listed\n\t%v\nwant\n\t%v", got, theListing)
	}
	// A count is what does not catch this: dash's table has fourteen names
	// too, so the sorted listing was the right length and the wrong rows.
	if len(got) != len(theListing) {
		t.Errorf("listing is %d rows, want %d", len(got), len(theListing))
	}
}

// `set -o pipefail` was accepted and then not listed, so `set -o pipefail; set
// +o` did not say the shell was in it — a script that saves and restores state
// with that output lost the option outright.
func TestPipefailIsListedAfterItIsSet(t *testing.T) {
	out, status := run(t, "set -o pipefail\nset +o\n")
	if status != 0 {
		t.Fatalf("status %d: %s", status, out)
	}
	if !strings.Contains(out, "set -o pipefail") {
		t.Errorf("set +o wrote %q, want `set -o pipefail` in it: the listing "+
			"is what `eval \"$(set +o)\"` feeds back", out)
	}
}

// The names and letters this shell has and has not, each measured. The two
// halves are one test because a name accepted and a letter refused is exactly
// the state this dialect was in for `errtrace`.
func TestTheOptionNamesAndLettersAreThisShells(t *testing.T) {
	for _, tc := range []struct {
		word   string
		status int
	}{
		{"-o errtrace", 0},
		{"-o pipefail", 0},
		{"-o notify", 0},
		{"-o ignoreeof", 0},
		{"-o vi", 0},
		{"-o allexport", 0},
		// Two names this shell has not got, and they are the reason they are
		// no longer in the substrate's common table.
		{"-o emacs", 1},
		{"-o nolog", 1},
		{"-o functrace", 1},
		{"-o hashall", 1},
		// The letters. `-E` is taken and `-T` is not, which is why the two
		// are separate axes.
		{"-E", 0},
		{"-b", 0},
		{"-I", 0},
		{"-T", 2},
		{"-h", 2},
		{"-k", 2},
	} {
		out, _ := run(t, "( set "+tc.word+" ) 2>&1\n( set "+tc.word+" ) >/dev/null 2>&1\necho st=$?")
		if want := fmt.Sprintf("st=%d", tc.status); !strings.Contains(out, want) {
			t.Errorf("set %s said %q, want %s", tc.word, out, want)
		}
	}
}

// `$-` puts the letters in this shell's own order, which is neither a sort nor
// the order they were set in — and ours had `i` in front of `c` where BusyBox
// puts it after (#3256).
//
// Measured one letter at a time and then every settable letter at once:
// `set -EubaCvxI` under `-c` is `EubaCvxcI`, the `-i` invocation puts `i`
// between `c` and `I`, and a terminal with `-m` puts `m` between them too.
func TestDollarDashIsInThisShellsOrder(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want string
	}{
		{`echo "$-"`, ""},
		{`set -a; echo "$-"`, "a"},
		{`set -e -u; echo "$-"`, "ue"},
		{`set -f; echo "$-"`, "f"},
		{`set -E; echo "$-"`, "E"},
		{`set -b; echo "$-"`, "b"},
		{`set -I; echo "$-"`, "I"},
		{`set -EubaCvxI; echo "$-"`, "EubaCvxI"},
	} {
		out, status := run(t, tc.src)
		if status != 0 {
			t.Fatalf("%s = %d: %s", tc.src, status, out)
		}
		// The last line, because `-v` and `-x` write the line that reads
		// `$-` before its answer comes out.
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if got := lines[len(lines)-1]; got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}
