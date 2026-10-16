// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A `case` that runs no command of its own keeps the status its header's last
// command substitution left: the subject's for an empty arm that matched, the
// last of all for no arm matching. Measured 2026-10-16 under `-c` on ksh93u+
// — see Semantics.CaseWithoutACommandKeepsTheSubstitutionStatus (#6710).
func TestACaseRunningNoCommandKeepsTheSubstitutionStatus(t *testing.T) {
	const src = `case a in $(false)) echo m ;; esac; echo $?
case a in $(exit 3)) ;; $(exit 4)) ;; esac; echo $?
case $(false) in b) ;; esac; echo $?
case $(false) in *) ;; esac; echo $?
case a in $(false)|a) echo m ;; esac
case a in $(false)|a) ;; esac; echo $?
false; case a in a) ;; esac; echo $?
case a in $(exit 3)|$(exit 4)) ;; esac; echo $?
case $(false) in *) : ;; esac; echo $?
`
	out, _ := runKsh(t, t.TempDir(), src)
	if want := strings.ReplaceAll("1|4|1|1|m|0|0|3|0", "|", "\n") + "\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}
