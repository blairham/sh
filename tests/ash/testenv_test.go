// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"os"
	"testing"

	"github.com/blairham/sh/internal/testenv"
)

// This suite starts shells, so it runs in a home of its own — see
// internal/testenv for why, and dialect/dash's copy of this file for the two
// issues that made it necessary.
func TestMain(m *testing.M) {
	if testenv.Assembled() {
		os.Exit(m.Run())
	}
	os.Exit(testenv.Run("dialect/ash", m))
}
