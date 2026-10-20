// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package shell is the ash binary's [driver.Shell], private to cmd/ash so
// that the binary's tests in cmd/ash/tests can build the same shell it runs.
package shell

import (
	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/acpboot"
	"github.com/blairham/sh/internal/mcpboot"
)

// New is the whole of "which shell am I", as data.
func New(version string) driver.Shell {
	return driver.Shell{
		Name: "ash",
		// The protocol server, which driver's `--acp` reaches. It is here
		// rather than in driver because internal/acp takes a Shell, so
		// driver cannot import it back — see driver/acp.go.
		ServeACP:   acpboot.ServeAs("ash"),
		ConnectACP: acpboot.ConnectAs("ash", "--"),
		ServeMCP:   mcpboot.ServeAs("ash"),
		Version:    version,
		// Where this machine keeps the administrator's startup files. It
		// is the install's answer rather than the dialect's, which is why
		// it is named here; see driver.Shell.SystemStartupDirectory.
		SystemStartupDirectory: "/etc",
		Dialect:                ash.Dialect(),
		Semantics:              ash.Semantics(),
		Diagnostics:            ash.Diagnostics(),
		Prelude:                ash.Prelude(),
		Register:               ash.Apply,
		PromptStyle:            ash.PromptStyle(),
		EditorStyle:            ash.EditorStyle(),
		HistoryStyle:           ash.HistoryStyle(),
	}
}
