// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package fdset

// fionread is FIONREAD, `_IOR('f', 127, int)`.
const fionread = 0x4004667f
