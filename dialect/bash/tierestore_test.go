// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// An option restored by `local -` is the option **moving**, so the parameter it
// is tied to has to hear it.
//
// Measured 2026-09-22 on bash 5.3.20 under `LC_ALL=C` from a script file, which
// is `varenv21.sub`'s own shape:
//
//	IGNOREEOF=0
//	set -o ignoreeof
//	f() { local -; set +o ignoreeof; }
//	f
//	echo $IGNOREEOF        10
//
// The assignment turns the option on, the body turns it off — which unsets the
// parameter — and the **return** turns it back on, which assigns the parameter
// the tie's value. This shell wrote nothing there: the restore reached the
// option table directly and the tie was wired to `set` alone (#4163, #4047).
func TestALocalDashRestoreMovesTheTiedParameter(t *testing.T) {
	dir := t.TempDir()
	out, _ := runBash(t, dir, "IGNOREEOF=0\nset -o ignoreeof\nf() { local -; set +o ignoreeof; }\nf\necho \"[$IGNOREEOF]\"\n")
	if strings.TrimSpace(out) != "[10]" {
		t.Errorf("output %q, want [10]", out)
	}
	// And the option itself is back on, which is what the restore was for —
	// asserted beside the parameter so a tie that fired without the option
	// moving would fail here rather than read as a pass.
	out, _ = runBash(t, dir, "set -o ignoreeof\nf() { local -; set +o ignoreeof; shopt -o ignoreeof; }\nf\nshopt -o ignoreeof\n")
	lines := strings.Fields(strings.TrimSpace(out))
	if len(lines) != 4 || lines[1] != "off" || lines[3] != "on" {
		t.Errorf("output %q, want the option off inside the call and on after it", out)
	}
	// The other direction, so the tie cannot be one-way: a body that turns the
	// option *on* leaves the parameter where the restore puts it.
	out, _ = runBash(t, dir, "unset IGNOREEOF\nf() { local -; set -o ignoreeof; }\nf\necho \"[${IGNOREEOF-unset}]\"\n")
	if strings.TrimSpace(out) != "[unset]" {
		t.Errorf("output %q, want the parameter unset again", out)
	}
}
