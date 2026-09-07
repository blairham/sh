// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command bash is a bash dialect built on the core.
//
// It exists to prove the core is a library rather than a program with options:
// everything here uses only the public API, so it could be lifted into its own
// repository without the core changing. That is the property worth protecting,
// and keeping this in cmd/ while importing nothing internal is what keeps
// proving it.
//
// The whole of the dialect is one value, prelude included: what this file
// does is name it. The front end is shared, in driver, because how a shell is
// invoked is not part of what makes it what it is.
package main

import (
	"os"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/driver"
)

// shell is the whole of "which shell am I", as data.
//
// It is a function rather than a literal in main so that the test exercises
// the same value the binary does, which is the only way the test says anything
// about the binary.
func shell() driver.Shell {
	return driver.Shell{
		Name:         "bash",
		Dialect:      bash.Dialect(),
		Semantics:    bash.Semantics(),
		Diagnostics:  bash.Diagnostics(),
		Prelude:      bash.Prelude(),
		Register:     bash.Apply,
		PromptStyle:  bash.PromptStyle(),
		EditorStyle:  bash.EditorStyle(),
		KeyBindings:  bash.KeyBindings,
		HistoryStyle: bash.HistoryStyle(),
	}
}

func main() { os.Exit(driver.Main(shell())) }
