// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !linux

package driver

import "os"

// A platform whose signal table was not measured adds nothing to the shared
// lists, exactly as die_other.go declines to invent a structure layout: the
// shared signals are the ones every kernel this builds for has.
var (
	platformThrowingSignals  []os.Signal
	platformDiscardedSignals []os.Signal
)
