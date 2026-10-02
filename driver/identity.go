// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"syscall"

	"github.com/blairham/sh/interp"
)

// changeIdentity makes this process another user or group id. It lives here
// rather than in interp for the reason setUmask does: who the process is
// belongs to the process, and a binary that is a shell is the one place a
// line of script may change it.
//
// Each is one system call and the kernel's answer is the answer, which is
// what the reference does: as root, `EUID=1` takes effect, and `EUID=10` after
// it is refused, because the process is not root any more. Measured
// 2026-10-02 on zsh 5.9.2 in the suite's image, as root (#5157).
func changeIdentity(which interp.Identity, id int) error {
	switch which {
	case interp.IdentityUser:
		return syscall.Setuid(id)
	case interp.IdentityEffectiveUser:
		return syscall.Seteuid(id)
	case interp.IdentityGroup:
		return syscall.Setgid(id)
	default:
		return syscall.Setegid(id)
	}
}
