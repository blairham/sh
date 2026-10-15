// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux && !(mips || mipsle || mips64 || mips64le || ppc64 || ppc64le)

package fdset

// fionread is FIONREAD on the architectures that number it like x86.
const fionread = 0x541b
