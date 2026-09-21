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
	"github.com/blairham/sh/internal/acpboot"
	"github.com/blairham/sh/internal/mcpboot"
)

// shell is the whole of "which shell am I", as data.
func shell() driver.Shell {
	return driver.Shell{
		Name: "zsh",
		// The protocol server, which driver's `--acp` reaches. It is here
		// rather than in driver because internal/acp takes a Shell, so
		// driver cannot import it back — see driver/acp.go.
		ServeACP:   acpboot.ServeAs("zsh"),
		ConnectACP: acpboot.ConnectAs("zsh", "--"),
		ServeMCP:   mcpboot.ServeAs("zsh"),
		Version:    version,
		// Where this machine keeps the administrator's startup files. It
		// is the install's answer rather than the dialect's, which is why
		// it is named here; see driver.Shell.SystemStartupDirectory.
		//
		// This shell is the one that cannot answer it with a constant.
		// zsh's system directory is chosen when zsh is built, and the two
		// platforms this runs on disagree — `/etc` on macOS, `/etc/zsh` on
		// Debian, where a constant `/etc` read the administrator's files
		// in none of the four slots (#3987). So the binary still names
		// where to look and the dialect says which of the two it is; see
		// zsh.SystemStartupDirectory for the measurements.
		SystemStartupDirectory: zsh.SystemStartupDirectory("/etc"),
		Dialect:                zsh.Dialect(),
		Semantics:              zsh.Semantics(),
		Diagnostics:            zsh.Diagnostics(),
		Prelude:                zsh.Prelude(),
		Register:               zsh.Apply,
		PromptStyle:            zsh.PromptStyle(),
		EditorStyle:            zsh.EditorStyle(),
		KeyBindings:            zsh.KeyBindings,
		ViEditing:              zsh.ViEditing,
		RunWidget:              zsh.RunWidget,
		RunCompletion:          zsh.RunCompletion,
		RunScheduled:           zsh.RunScheduled,
		HighlightLine:          zsh.RegionHighlights,
		StartLine:              zsh.ResetRegionHighlight,
		// What the editor waits on beside the terminal, and what happens when
		// one of those wakes: `zle -F`.
		WatchedDescriptors: zsh.WatchedDescriptors,
		DescriptorReady:    zsh.DescriptorReady,
		HistoryStyle:       zsh.HistoryStyle(),
		HookStyle:          zsh.HookStyle(),
	}
}

func main() { os.Exit(driver.Main(shell())) }

// version is what this binary tells a protocol client it is. A var and
// not a const so the release build can stamp the tag over it with
// `-X main.version=`, which can only write to a variable; a checkout says
// so rather than inventing a number that will be wrong.
var version = "0.0.0-dev"
