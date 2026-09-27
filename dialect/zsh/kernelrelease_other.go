// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin

package zsh

// kernelRelease is nothing on every platform whose `$OSTYPE` does not carry
// one.
//
// Measured: Linux's is `linux-gnu`, which names a C library rather than a
// kernel version, so there is no release to ask for and asking would be
// answering a question this platform's triple does not pose. See buildTriple.
func kernelRelease() string { return "" }
