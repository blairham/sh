// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux && !(amd64 || ppc64 || ppc64le || s390x || arm64 || riscv64 || loong64 || mips64 || mips64le)

package opened

import "syscall"

// The 32-bit architectures, where the call is fstatat64 into a struct of its
// own rather than into syscall.Stat_t. Nothing here asks; a caller asks by
// name, which is what every caller did before StatAt existed.
const statAtSupported = false

func fstatat(int, string, *syscall.Stat_t, int) error { return errStatAtUnsupported }
