// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package shell is the ksh binary's [driver.Shell], private to cmd/ksh so
// that the binary's tests in cmd/ksh/tests can build the same shell it runs.
package shell

import (
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/acpboot"
	"github.com/blairham/sh/internal/mcpboot"
)

// New is the whole of "which shell am I", as data.
func New(version string) driver.Shell {
	return driver.Shell{
		Name: "ksh",
		// The protocol server, which driver's `--acp` reaches. It is here
		// rather than in driver because internal/acp takes a Shell, so
		// driver cannot import it back — see driver/acp.go.
		ServeACP:   acpboot.ServeAs("ksh"),
		ConnectACP: acpboot.ConnectAs("ksh", "--"),
		ServeMCP:   mcpboot.ServeAs("ksh"),
		Version:    version,
		// Where this machine keeps the administrator's startup files. It
		// is the install's answer rather than the dialect's, which is why
		// it is named here; see driver.Shell.SystemStartupDirectory.
		SystemStartupDirectory: "/etc",
		Dialect:                ksh.Dialect(),
		Semantics:              ksh.Semantics(),
		Diagnostics:            ksh.Diagnostics(),
		Prelude:                ksh.Prelude(),
		Register:               ksh.Apply,
		PromptStyle:            ksh.PromptStyle(),
		EditorStyle:            ksh.EditorStyle(),
		HistoryStyle:           ksh.HistoryStyle(),
		// The script it is running is kept open on 10. See
		// driver.Shell.ScriptDescriptor.
		ScriptDescriptor: 10,
	}
}
