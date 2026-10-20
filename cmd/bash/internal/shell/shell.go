// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package shell is the bash binary's [driver.Shell], private to cmd/bash so
// that the binary's tests in cmd/bash/tests can build the same shell it runs.
package shell

import (
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/acpboot"
	"github.com/blairham/sh/internal/mcpboot"
)

// New is the whole of "which shell am I", as data.
//
// It is a function rather than a literal in main so that the test exercises
// the same value the binary does, which is the only way the test says anything
// about the binary.
func New(version string) driver.Shell {
	return driver.Shell{
		Name: "bash",
		// The protocol server, which driver's `--acp` reaches. It is here
		// rather than in driver because internal/acp takes a Shell, so
		// driver cannot import it back — see driver/acp.go.
		ServeACP:   acpboot.ServeAs("bash"),
		ConnectACP: acpboot.ConnectAs("bash", "--"),
		ServeMCP:   mcpboot.ServeAs("bash"),
		Version:    version,
		// Where this machine keeps the administrator's startup files. It
		// is the install's answer rather than the dialect's, which is why
		// it is named here; see driver.Shell.SystemStartupDirectory.
		SystemStartupDirectory: "/etc",
		Dialect:                bash.Dialect(),
		Semantics:              bash.Semantics(),
		Diagnostics:            bash.Diagnostics(),
		Prelude:                bash.Prelude(),
		PreludeTree:            bash.PreludeTree(),
		Register:               bash.Apply,
		PromptStyle:            bash.PromptStyle(),
		EditorStyle:            bash.EditorStyle(),
		KeyBindings:            bash.KeyBindings,
		ViEditing:              bash.ViEditing,
		RunWidget:              bash.RunWidget,
		HistoryStyle:           bash.HistoryStyle(),
		HookStyle:              bash.HookStyle(),
	}
}
