// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package tests_test

import (
	"github.com/blairham/sh/driver"

	binshell "github.com/blairham/sh/cmd/dash/internal/shell"
)

// shell is the binary's own Shell, built as cmd/dash's main builds it.
func shell() driver.Shell { return binshell.New("0.0.0-dev") }
