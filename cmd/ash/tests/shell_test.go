// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package tests_test

import (
	binshell "github.com/blairham/sh/cmd/ash/internal/shell"
	"github.com/blairham/sh/driver"
)

// shell is the binary's own Shell, built as cmd/ash's main builds it.
func shell() driver.Shell { return binshell.New("0.0.0-dev") }
