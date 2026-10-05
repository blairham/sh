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
	src.WriteString(`cd ` + dir + `; pushd . >/dev/null; unset DIRSTACK; DIRSTACK=(q); echo "${DIRSTACK[*]}"
unset GROUPS; GROUPS=(7); echo "${GROUPS[*]}"; declare -p GROUPS
unset RANDOM; RANDOM=(a b); declare -p RANDOM
`)
	want.WriteString("q\n7\ndeclare -a GROUPS=([0]=\"7\")\ndeclare -a RANDOM=([0]=\"a\" [1]=\"b\")\n")
	out, st := runBashPrelude(t, dir, src.String())
	if st != 0 || out != want.String() {
		t.Errorf("status %d, output:\n%s\nwant:\n%s", st, out, want.String())
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
