// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package shell is the dash binary's [driver.Shell], importable so that the
// binary's tests can live outside package main.
package shell

import (
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/acpboot"
	"github.com/blairham/sh/internal/mcpboot"
)

// New is the whole of "which shell am I", as data. version is the binary's
// own, stamped into package main by the release build.
func New(version string) driver.Shell {
	return driver.Shell{
		Name: "dash",
		// The protocol server, which driver's `--acp` reaches. It is here
		// rather than in driver because internal/acp takes a Shell, so
		// driver cannot import it back — see driver/acp.go.
		ServeACP:   acpboot.ServeAs("dash"),
		ConnectACP: acpboot.ConnectAs("dash", "--"),
		ServeMCP:   mcpboot.ServeAs("dash"),
		Version:    version,
		// Where this machine keeps the administrator's startup files. It
		// is the install's answer rather than the dialect's, which is why
		// it is named here; see driver.Shell.SystemStartupDirectory.
		SystemStartupDirectory: "/etc",
		Dialect:                dash.Dialect(),
		Semantics:              dash.Semantics(),
		Diagnostics:            dash.Diagnostics(),
		Prelude:                dash.Prelude(),
		Register:               dash.Apply,
		PromptStyle:            dash.PromptStyle(),
		EditorStyle:            dash.EditorStyle(),
		HistoryStyle:           dash.HistoryStyle(),
	}
}
