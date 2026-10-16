// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// bash writes the bytes a diagnostic quotes as they are, where zsh makes
// them visible (#6690): measured 2026-10-15 on bash 5.3, `failglob`'s `no
// match:`, a `${x?word}` message, `source` and `kill -s` all carry the raw
// `\x01` and tab.
func TestADiagnosticWritesTheTextItQuotesRaw(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`shopt -s failglob; echo $'zz\x01q'*`, "no match: zz\x01q*\n"},
		{`shopt -s failglob; echo $'zz\tq'*`, "no match: zz\tq*\n"},
		{`unset x; echo ${x?$'bo\x01om'}`, "x: bo\x01om\n"},
		{`source $'/no\x01pe'`, "/no\x01pe: No such file or directory\n"},
		{`kill -s $'X\x01' $$`, "kill: X\x01: invalid signal specification\n"},
	} {
		_, errs, _ := runAlias(t, tc.src)
		if !strings.HasSuffix(errs, tc.want) {
			t.Errorf("%s:\n errs %q\nwant it to end %q", tc.src, errs, tc.want)
		}
	}
}
