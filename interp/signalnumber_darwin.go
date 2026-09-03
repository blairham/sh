// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin

package interp

// This machine's C library writes the number after the words: `Killed: 9`
// rather than `Killed`.
//
// The library's doing and not any shell's — bash and dash both print it here
// and neither prints it on a Linux machine, and two programs that share
// nothing but libc do not invent the same suffix. Scoped to the one platform
// it was measured on rather than to BSDs at large, which were not.
const signalDescriptionCarriesItsNumber = true
