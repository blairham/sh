// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !race

package testenv

// RaceDetector is whether this binary was built with -race — see race.go.
const RaceDetector = false
