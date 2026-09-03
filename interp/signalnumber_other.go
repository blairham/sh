// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin

package interp

// Everywhere else the words stand alone: `Killed`, not `Killed: 9`.
const signalDescriptionCarriesItsNumber = false
