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

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/driver"
)

// shell is the whole of "which shell am I", as data.
func shell() driver.Shell {
	return driver.Shell{
		Name:         "ash",
		Dialect:      ash.Dialect(),
		Semantics:    ash.Semantics(),
		Diagnostics:  ash.Diagnostics(),
		Prelude:      ash.Prelude(),
		Register:     ash.Apply,
		PromptStyle:  ash.PromptStyle(),
		EditorStyle:  ash.EditorStyle(),
		HistoryStyle: ash.HistoryStyle(),
	}
}

func main() { os.Exit(driver.Main(shell())) }
