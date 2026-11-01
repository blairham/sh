// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The status of a `case` that runs no command of its own, after a command
// substitution in its header. Measured 2026-10-16 under `-c` on zsh 5.9.2,
// bash 5.3.20, dash 0.5.12, BusyBox ash 1.37.0 and ksh93u+ — see
// Semantics.CaseWithoutACommandKeepsTheSubstitutionStatus (#6710).
func TestACaseMatchingNothingIsZero(t *testing.T) {
	t.Parallel()
	out, _ := runZsh(t, t.TempDir(), caseStatusRows)
	if want := strings.ReplaceAll("0|0|0|0|m|0|0|0|0", "|", "\n") + "\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

const caseStatusRows = `case a in $(false)) echo m ;; esac; echo $?
case a in $(exit 3)) ;; $(exit 4)) ;; esac; echo $?
case $(false) in b) ;; esac; echo $?
case $(false) in *) ;; esac; echo $?
case a in $(false)|a) echo m ;; esac
case a in $(false)|a) ;; esac; echo $?
false; case a in a) ;; esac; echo $?
case a in $(exit 3)|$(exit 4)) ;; esac; echo $?
case $(false) in *) : ;; esac; echo $?
`
