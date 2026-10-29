// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !linux

package driver

// lowDescriptorCeiling is 56 off Linux too, so that a script meets the same
// ceiling on every platform this shell ships for. Linux lowered it for a cost
// darwin does not have (see lowfds_linux.go); darwin follows for the contract,
// at the maintainer's decision of 2026-10-28 (#6491).
//
// The hold here costs only its fcntl calls, and less than #6491 estimated.
// Measured on macOS arm64, a zsh script file holding one `print`, instructions
// retired (median of 15): 38.80M at 99, 38.50M at 56 and 38.34M at 9, so the
// whole hold is about 0.46M and this ceiling takes 0.30M of it. Child CPU,
// paired over 301 interleaved runs: 3.385 to 3.361 ms, faster in 180 of 301.
//
// What it costs a script is what it costs on Linux: the runtime's descriptors
// now sit on 57 to 59 rather than 100 to 102, and those three numbers are the
// unplaceable ones. Every number the panel can name and every number `{v}`
// allocates is still below 56.
const lowDescriptorCeiling = 56
