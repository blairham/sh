// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build race

package zsh

// raceDetector is whether this test binary was built with -race, asked at
// build time because the toolchain cannot be asked at run time; this file and
// norace_test.go are the two halves of the answer. The byte counts in
// listreadercost_test.go read it: the detector's own bookkeeping allocates
// beside every allocation it watches, so a bound measured without it does not
// hold under it.
const raceDetector = true
