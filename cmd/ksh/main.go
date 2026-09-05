// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command ksh is a ksh93 dialect built on the core.
//
// It exists to prove the core is a library rather than a program with options:
// everything here uses only the public API, so it could be lifted into its own
// repository without the core changing. That is the property worth protecting,
// and keeping this in cmd/ while importing nothing internal is what keeps
// proving it.
//
// There is no prelude, and that is the dialect rather than an omission: the
// `pushd` and `popd` the bash and zsh binaries define are not ksh93's. What
// ksh93 does differently here it does by removal — Apply takes `local` away,
// because ksh93 spells that `typeset` and a script that finds `local` working
// would be relying on something the real shell does not have.
package main

import (
	"os"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/driver"
)

// shell is the whole of "which shell am I", as data.
func shell() driver.Shell {
	return driver.Shell{
		Name:         "ksh",
		Dialect:      ksh.Dialect(),
		Semantics:    ksh.Semantics(),
		Diagnostics:  ksh.Diagnostics(),
		Prelude:      ksh.Prelude(),
		Register:     ksh.Apply,
		PromptStyle:  ksh.PromptStyle(),
		EditorStyle:  ksh.EditorStyle(),
		HistoryStyle: ksh.HistoryStyle(),
	}
}

func main() { os.Exit(driver.Main(shell())) }
