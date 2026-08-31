// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command zsh is a zsh dialect built on the core.
//
// It exists to prove the core is a library rather than a program with options:
// everything here uses only the public API, so it could be lifted into its own
// repository without the core changing. That is the property worth protecting,
// and keeping this in cmd/ while importing nothing internal is what keeps
// proving it.
//
// The whole of the dialect is one value. What is left below it is a prelude —
// dialect written as shell — and nothing else. The front end is shared, in
// driver, because how a shell is invoked is not part of what makes it zsh.
package main

import (
	"os"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/driver"
)

// prelude is the part of the dialect that needs no Go.
//
// A function shadows a builtin and an external command alike, so anything here
// replaces the core's answer without the core knowing. Most of a real dialect
// belongs in a file like this.
const prelude = `
pushd() { cd "$1"; }
popd()  { cd "$OLDPWD"; }
`

// shell is the whole of "which shell am I", as data.
func shell() driver.Shell {
	return driver.Shell{
		Name:        "zsh",
		Dialect:     zsh.Dialect(),
		Semantics:   zsh.Semantics(),
		Diagnostics: zsh.Diagnostics(),
		Prelude:     prelude,
		Register:    zsh.Apply,
	}
}

func main() { os.Exit(driver.Main(shell())) }
