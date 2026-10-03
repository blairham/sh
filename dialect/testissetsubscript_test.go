// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// `test -v` and `[ -v ]` over an operand whose subscript will not evaluate:
// bash gives up the command, zsh the script, and ksh93 reports a failed
// builtin by name and carries on. Measured 2026-10-03 on bash 5.3.20, zsh
// 5.9.2 and ksh93u+ (#5590). See interp.Semantics.BadSubscriptToTestIsSet.
func TestATestIsSetOperandWithABadSubscript(t *testing.T) {
	const src = "a=(x y z)\ntest -v 'a[1/0]'; echo same\necho next $?\nf() { [ -v 'a[1/0]' ]; echo x; }; f\necho end\n"
	for _, c := range []struct {
		p    dialecttest.Preset
		want string
	}{
		{
			dialecttest.Preset{Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics, Diagnostics: bash.Diagnostics, Apply: bash.Apply},
			"bash: line 2: 1/0: division by 0 (error token is \"0\")\nnext 1\nbash: line 4: 1/0: division by 0 (error token is \"0\")\nend\n",
		},
		{
			dialecttest.Preset{Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics, Diagnostics: zsh.Diagnostics, Apply: zsh.Apply},
			"zsh:2: division by zero\n",
		},
		{
			dialecttest.Preset{Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics, Diagnostics: ksh.Diagnostics, Apply: ksh.Apply},
			"ksh[2]: test: 1/0: divide by zero\nsame\nnext 0\nksh[4]: [: 1/0: divide by zero\nx\nend\n",
		},
	} {
		t.Run(c.p.Name, func(t *testing.T) {
			out, _, err := c.p.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
			if err != nil {
				t.Fatal(err)
			}
			if out != c.want {
				t.Errorf("got %q\nwant %q", out, c.want)
			}
		})
	}
}
