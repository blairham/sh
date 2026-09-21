// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command dash is a dash dialect built on the core.
//
// It exists to prove the core is a library rather than a program with options:
// everything here uses only the public API, so it could be lifted into its own
// repository without the core changing. That is the property worth protecting,
// and keeping this in cmd/ while importing nothing internal is what keeps
// proving it.
//
// There is no prelude, and that is the dialect rather than an omission: the
// `pushd` and `popd` the bash and zsh binaries define are exactly what dash
// does not have. A dialect is as much what it declines to provide.
package main

import (
	"os"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/acpboot"
	"github.com/blairham/sh/internal/mcpboot"
)

// shell is the whole of "which shell am I", as data.
func shell() driver.Shell {
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

func main() { os.Exit(driver.Main(shell())) }

// version is what this binary tells a protocol client it is. A var and
// not a const so the release build can stamp the tag over it with
// `-X main.version=`, which can only write to a variable; a checkout says
// so rather than inventing a number that will be wrong.
var version = "0.0.0-dev"
