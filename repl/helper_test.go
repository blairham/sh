// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "github.com/blairham/sh/interp"

// newTestRunner is a Runner with just enough in it for the prompt questions.
func newTestRunner(vars map[string]string) *interp.Runner {
	sem := interp.PosixSemantics()
	return &interp.Runner{Semantics: &sem, Vars: vars}
}
