// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command ash is a BusyBox ash dialect built on the core.
//
// It exists twice over. Like cmd/dash it proves the core is a library rather
// than a program with options: everything here uses only the public API, so it
// could be lifted into its own repository without the core changing.
//
// And it is the fifth of them, which is the claim this binary was added to
// test — "adding a shell adds a directory rather than editing the substrate"
// had never been tried since the original four. What it took is this file and
// dialect/ash; docs/spec/ash.md records what that cost and what it did not.
//
// There is no prelude beyond one variable, and no `pushd`: a dialect is as
// much what it declines to provide.
package main

import (
	"os"

	"github.com/blairham/sh/cmd/ash/internal/shell"
	"github.com/blairham/sh/driver"
)

func main() { os.Exit(driver.Main(shell.New(version))) }

// version is what this binary tells a protocol client it is. A var and
// not a const so the release build can stamp the tag over it with
// `-X main.version=`, which can only write to a variable; a checkout says
// so rather than inventing a number that will be wrong.
var version = "0.0.0-dev"
