// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !linux

package interp

// A platform nobody has measured, so the table is the range: no extra names,
// and no numbers past the ones the shared table already carries.
//
// Zero is not "no signals" — it is read as "no bound of our own", and the
// shared table goes on answering for every number it names. Guessing a
// range here would be guessing which numbers a kernel nobody ran will take.
var platformSignals []signalEntry

const platformSignalMax = 0

// And no corrections to the shared table's default actions, for the same
// reason: a kernel nobody ran is a kernel nobody measured SIGIO on. The
// shared value stands, which is the BSD's (#3703).
var platformSignalDefaults map[string]bool

// And no unnamed numbers either, for the same reason the bound is zero: a
// kernel nobody ran has no range past the table, so nothing reaches this.
const platformUnnamedSignalsEndTheShell = false
