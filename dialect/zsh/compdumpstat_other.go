// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !linux

package zsh

import "io/fs"

// compdumpIdentity has nothing beyond the size and the modification time to
// offer on a system whose stat this package does not read.
func compdumpIdentity(fs.FileInfo) (ctime int64, ino, dev uint64) { return 0, 0, 0 }
