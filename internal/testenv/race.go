// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build race

package testenv

// RaceDetector is whether this binary was built with -race. The toolchain
// cannot be asked at run time, so it is asked at build time: this file and
// norace.go are the two halves of the answer.
//
// It is here, once, rather than as a pair of files in each package that
// needs it, which is how it started: syntax and dialect/zsh each carried a
// copy before parsecache needed a third.
//
// What reads it is a test that pays for the detector without being able to
// use it — a sweep over the corpus run in one goroutine, where -race can find
// nothing and costs about ten times the work — or a count the detector's own
// bookkeeping inflates. Such a test reads less, or skips the count, under the
// detector, and says where the whole of it still runs: CI's macOS leg is the
// uninstrumented one.
const RaceDetector = true
