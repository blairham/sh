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
// The whole of the dialect is one value, prelude included: what this file
// does is name it. The front end is shared, in driver, because how a shell is
// invoked is not part of what makes it what it is.
package main

import (
	"os"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/driver"
)

// shell is the whole of "which shell am I", as data.
func shell() driver.Shell {
	return driver.Shell{
		Name:         "zsh",
		Dialect:      zsh.Dialect(),
		Semantics:    zsh.Semantics(),
		Diagnostics:  zsh.Diagnostics(),
		Prelude:      zsh.Prelude(),
		Register:     zsh.Apply,
		PromptStyle:  zsh.PromptStyle(),
		EditorStyle:  zsh.EditorStyle(),
		KeyBindings:  zsh.KeyBindings,
		ViEditing:    zsh.ViEditing,
		RunWidget:    zsh.RunWidget,
		RunScheduled: zsh.RunScheduled,
		// What the editor waits on beside the terminal, and what happens when
		// one of those wakes: `zle -F`.
		WatchedDescriptors: zsh.WatchedDescriptors,
		DescriptorReady:    zsh.DescriptorReady,
		HistoryStyle:       zsh.HistoryStyle(),
		HookStyle:          zsh.HookStyle(),
	}
}

func main() { os.Exit(driver.Main(shell())) }
