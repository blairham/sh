// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestUnsetEndsEveryProducedParameter is #6039: in this shell `unset` takes
// the special behavior away from a produced parameter, so whatever is assigned
// afterwards is an ordinary variable — for a produced array as much as for a
// scalar, and with none of the letters the listing was told. Measured
// 2026-10-05 on bash 5.3 (/opt/homebrew/bin/bash), every special the manual
// says loses its properties, assigned `1+1` after `unset`.
func TestUnsetEndsEveryProducedParameter(t *testing.T) {
	dir := t.TempDir()
	var src strings.Builder
	var want strings.Builder
	for _, name := range []string{
		"RANDOM", "SRANDOM", "SECONDS", "BASHPID", "HISTCMD", "LINENO",
		"EPOCHSECONDS", "FUNCNAME", "DIRSTACK", "GROUPS",
	} {
		src.WriteString("unset " + name + "; " + name + "=1+1; declare -p " + name + "\n")
		want.WriteString("declare -- " + name + "=\"1+1\"\n")
	}
	out, st := runBashPrelude(t, dir, src.String())
	if st != 0 || out != want.String() {
		t.Errorf("status %d, output:\n%s\nwant:\n%s", st, out, want.String())
	}
	// The array spellings, each in a shell of its own: a scalar assignment
	// ends the producer as well, so one run holding both would let the
	// first row answer for the rest.
	for _, tc := range []struct{ src, want string }{
		{`pushd . >/dev/null; unset DIRSTACK; DIRSTACK=(q); echo "${DIRSTACK[*]}"`, "q"},
		{`pushd . >/dev/null; unset DIRSTACK; DIRSTACK+=(x); declare -p DIRSTACK`, `declare -a DIRSTACK=([0]="x")`},
		{`unset GROUPS; GROUPS=(7); echo "${GROUPS[*]}"`, "7"},
		{`unset GROUPS; GROUPS[3]=7; declare -p GROUPS`, `declare -a GROUPS=([3]="7")`},
		{`unset GROUPS; read -a GROUPS <<< "a b"; declare -p GROUPS`, `declare -a GROUPS=([0]="a" [1]="b")`},
		{`unset RANDOM; RANDOM=(a b); declare -p RANDOM`, `declare -a RANDOM=([0]="a" [1]="b")`},
		{`unset GROUPS; GROUPS[1]+=x; declare -p GROUPS`, `declare -a GROUPS=([1]="x")`},
		{`unset GROUPS; GROUPS+=(x); declare -p GROUPS`, `declare -a GROUPS=([0]="x")`},
		{`unset GROUPS; mapfile -t GROUPS <<< m; declare -p GROUPS`, `declare -a GROUPS=([0]="m")`},
		{`unset GROUPS; read "GROUPS[2]" <<< r; declare -p GROUPS`, `declare -a GROUPS=([2]="r")`},
		{`unset GROUPS; printf -v "GROUPS[1]" %s p; declare -p GROUPS`, `declare -a GROUPS=([1]="p")`},
		{`unset GROUPS; declare GROUPS[3]=7; declare -p GROUPS`, `declare -a GROUPS=([3]="7")`},
		{`unset GROUPS; declare -A GROUPS=([k]=v); declare -p GROUPS`, `declare -A GROUPS=([k]="v" )`},
		{`unset GROUPS; declare -A GROUPS; declare -p GROUPS`, `declare -A GROUPS`},
		{`unset GROUPS; for GROUPS in f; do :; done; declare -p GROUPS`, `declare -- GROUPS="f"`},
	} {
		out, st := runBashPrelude(t, dir, "cd "+dir+"\n"+tc.src)
		if st != 0 || out != tc.want+"\n" {
			t.Errorf("%s: status %d, output %q, want %q", tc.src, st, out, tc.want+"\n")
		}
	}
}

// TestAWriteToAProducedArrayIsStillAMessage is the control: without `unset`
// the same writes are a message to the producer, and it discards them.
// Measured the same day: `GROUPS=(7)` leaves the groups, and `DIRSTACK=(q)`
// with nothing pushed leaves the stack.
func TestAWriteToAProducedArrayIsStillAMessage(t *testing.T) {
	dir := t.TempDir()
	out, st := runBashPrelude(t, dir, `cd `+dir+`
DIRSTACK=(q); echo "${#DIRSTACK[@]}"; [ "${DIRSTACK[0]}" = "$PWD" ] && echo pwd
GROUPS=(notanumber); [ "${GROUPS[0]}" != notanumber ] && echo groups`)
	if want := "1\npwd\ngroups\n"; st != 0 || out != want {
		t.Errorf("status %d, output %q, want %q", st, out, want)
	}
}
