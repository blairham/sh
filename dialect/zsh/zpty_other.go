// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !linux

package zsh

// zptySupported is false where internal/pty has no sequence for opening a
// pair, so the module is not registered at all. See registerZptyModule.
const zptySupported = false
