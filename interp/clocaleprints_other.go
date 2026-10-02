// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin

package interp

// cLocalePrintsHighByte says whether a byte past ASCII is a printing
// character under the C locale, which is the C library's answer and not the
// shell's.
//
// None is, under glibc. Measured 2026-10-02 on zsh 5.9.2 inside
// `ghcr.io/blairham/sh/zsh@sha256:aab8255c…` (Debian, aarch64), through `(V)`
// under `LC_ALL=C` for every byte from 0x80 to 0xff: all 128 are written
// `\M-` and their low half. macOS's C locale answers differently; see
// clocaleprints_darwin.go.
func cLocalePrintsHighByte(byte) bool { return false }
