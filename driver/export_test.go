// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// FrontEndForTest is frontEnd, reachable from the package's external tests.
//
// The assembly is what is being checked and it is not exported: a dialect's
// answers have to arrive, and the editor they arrive at is only built where
// there is a terminal.
func (sh Shell) FrontEndForTest(r *interp.Runner, name string, dg interp.Diagnostics) repl.Shell {
	return sh.frontEnd(r, name, dg)
}
