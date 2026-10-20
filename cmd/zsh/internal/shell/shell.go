// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package shell is the zsh binary's [driver.Shell], private to cmd/zsh so
// that the binary's tests in cmd/zsh/tests can build the same shell it runs.
package shell

import (
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/acpboot"
	"github.com/blairham/sh/internal/mcpboot"
)

// New is the whole of "which shell am I", as data.
func New(version string) driver.Shell {
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
		// And an installed zsh's function library, appended to the default
		// $fpath after this installation's own (#6128): the install's
		// answer again, for the same reason.
		SystemFunctionDirectories: zsh.SystemFunctionDirectories,
		Dialect:                   zsh.Dialect(),
		Semantics:                 zsh.Semantics(),
		Diagnostics:               zsh.Diagnostics(),
		Prelude:                   zsh.Prelude(),
		PreludeTree:               zsh.PreludeTree(),
		CommandStringParameter:    "ZSH_EXECUTION_STRING",
		Register:                  zsh.Apply,
		PromptStyle:               zsh.PromptStyle(),
		EditorStyle:               zsh.EditorStyle(),
		KeyBindings:               zsh.KeyBindings,
		ViEditing:                 zsh.ViEditing,
		NamedWidgets:              zsh.NamedWidgets,
		RunWidget:                 zsh.RunWidget,
		RunCompletion:             zsh.RunCompletion,
		ListScrollPrompt:          zsh.ListScrollPrompt,
		RunScheduled:              zsh.RunScheduled,
		TransformTermcap:          zsh.TransformTermcap,
		HighlightLine:             zsh.RegionHighlights,
		StartLine:                 zsh.StartLine,
		PastedTextStyle:           zsh.PastedTextStyle,
		BeforeStartupFiles:        zsh.BeforeStartupFiles,
		// What the editor waits on beside the terminal, and what happens when
		// one of those wakes: `zle -F`.
		WatchedDescriptors: zsh.WatchedDescriptors,
		DescriptorReady:    zsh.DescriptorReady,
		HistoryStyle:       zsh.HistoryStyle(),
		HookStyle:          zsh.HookStyle(),
		MailStyle:          zsh.MailStyle(),
	}
}
