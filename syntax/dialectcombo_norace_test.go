// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !race

package syntax_test

// raceDetector is whether this test binary was built with -race — see the
// build-tagged sibling of this file, and dialectcombo_test.go for what turns
// on it.
const raceDetector = false
