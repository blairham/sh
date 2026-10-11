// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !race

package zsh

// raceDetector is whether this test binary was built with -race — see
// race_test.go.
const raceDetector = false
