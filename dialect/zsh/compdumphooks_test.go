// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"testing"
	"time"
)

// SetCompdumpForTest narrows the completion dump's racy window and, where
// identity is not empty, stands in for this build's identity, for the length
// of one test. Not for a parallel test: both are package state.
func SetCompdumpForTest(t *testing.T, racy time.Duration, identity string) {
	t.Helper()
	oldRacy, oldIdentity := compdumpRacyWindow, compdumpBuildIdentity
	compdumpRacyWindow = racy
	if identity != "" {
		compdumpBuildIdentity = func() string { return identity }
	}
	t.Cleanup(func() { compdumpRacyWindow, compdumpBuildIdentity = oldRacy, oldIdentity })
}

// CompdumpEncode and CompdumpDecode are the file format, for a test that
// plants a dump or reads one back.
var (
	CompdumpEncode = compdumpEncode
	CompdumpDecode = compdumpDecode
)
