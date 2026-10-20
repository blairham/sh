// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// Under `ignorebraces`, and so under `emulate sh`, which sets it, `{name}` in
// front of a redirection is an ordinary word — the command — and no
// descriptor is picked. `ignoreclosebraces` leaves the reading alone.
// Measured 2026-10-15 on zsh 5.9.2 from script files (#6680).
func TestIgnoreBracesTakesTheFdVariableAway(t *testing.T) {
	for _, c := range []struct{ src, out, errs string }{
		{"setopt ignorebraces\nexec {w}</dev/null; echo w=$w", "", "command not found: {w}"},
		{"emulate sh\n{v}</dev/null; echo v=$v", "v=\n", "command not found: {v}"},
		{"setopt ignorebraces\nunsetopt ignorebraces\nexec {w}</dev/null; echo w=$w", "w=11\n", ""},
		{"setopt ignoreclosebraces\nexec {w}</dev/null; echo w=$w", "w=11\n", ""},
	} {
		var o, e bytes.Buffer
		driver.MainArgs(zshWriting(&o, &e), []string{"zsh", "-f", zshScriptFileAt(t, c.src+"\n")})
		out, errs := o.String(), e.String()
		if out != c.out || !strings.Contains(errs, c.errs) || (c.errs == "" && errs != "") {
			t.Errorf("%q: got %q and %q, want %q and %q", c.src, out, errs, c.out, c.errs)
		}
	}
}
