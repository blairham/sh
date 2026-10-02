// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **The parameters that are not names are rows of every listing that walks
// the table** (#5157). Measured 2026-10-01 on zsh 5.9.2 under `-f -c`, `env
// -i PATH=/usr/bin:/bin`, byte for byte. See registerTheSpecialParameterListing.
func TestTheSpecialParametersAreListed(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"typeset +", `typeset + | /usr/bin/head -8`,
			"integer 10 readonly !\ninteger 10 readonly '#'\ninteger 10 readonly '$'\narray readonly '*'\n" +
				"readonly -\n0\ninteger 10 readonly '?'\narray readonly @\n",
		},
		{"typeset -a", `typeset -a | /usr/bin/head -3`, "'*'=(  )\n@=(  )\nargv=(  )\n"},
		{"a named -p writes nothing", `typeset -p '#'; echo st=$?`, "st=0\n"},
		{"a pattern", `typeset +m '[#?0]'`, "integer 10 readonly '#'\n0\ninteger 10 readonly '?'\n"},
		{"with parameters", `set -- a 'b c'; typeset -m '[*@#]'`, "'#'=2\n'*'=( a 'b c' )\n@=( a 'b c' )\n"},
		{"HISTCHARS", `typeset +m HISTCHARS; typeset -p HISTCHARS`, "HISTCHARS\ntypeset HISTCHARS='!^#'\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			// With the prelude, which is where a fresh shell's `histchars`
			// comes from.
			out, _, err := preset.CombinedWithPrelude(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
			if err != nil {
				t.Fatal(err)
			}
			if out != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}
