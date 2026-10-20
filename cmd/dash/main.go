// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command dash is a dash dialect built on the core.
//
// It exists to prove the core is a library rather than a program with options:
// everything here uses only the public API, so it could be lifted into its own
// repository without the core changing. That is the property worth protecting,
// and keeping this in cmd/ while importing nothing internal is what keeps
// proving it.
//
// There is no prelude, and that is the dialect rather than an omission: the
// `pushd` and `popd` the bash and zsh binaries define are exactly what dash
// does not have. A dialect is as much what it declines to provide.
package main

import (
	"os"

	"github.com/blairham/sh/cmd/dash/internal/shell"
	"github.com/blairham/sh/driver"
)

func main() { os.Exit(driver.Main(shell.New(version))) }

// version is what this binary tells a protocol client it is. A var and
// not a const so the release build can stamp the tag over it with
// `-X main.version=`, which can only write to a variable; a checkout says
// so rather than inventing a number that will be wrong.
var version = "0.0.0-dev"
