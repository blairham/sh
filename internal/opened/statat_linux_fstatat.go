// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux && (arm64 || riscv64 || loong64 || mips64 || mips64le)

package opened

import "syscall"

// The architectures whose standard library exports fstatat.
const statAtSupported = true

func fstatat(dirfd int, name string, st *syscall.Stat_t, flags int) error {
	return syscall.Fstatat(dirfd, name, st, flags)
}
