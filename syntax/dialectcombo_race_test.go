// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build race

package syntax_test

// raceDetector is whether this test binary was built with -race. There is no
// way to ask the toolchain at run time, so it is asked at build time: this
// file and its sibling are the two halves of the answer.
//
// dialectcombo_test.go is what reads it, and why is written there.
const raceDetector = true
