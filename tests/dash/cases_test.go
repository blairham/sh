// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/internal/shtest"
)

// TestCases runs every case file under cases/: a snippet and what dash says
// to it, kept as data. See internal/shtest for the format.
func TestCases(t *testing.T) {
	t.Parallel()
	shtest.Run(t, preset, "cases")
}
