// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package gatedfs

import "os"

// A platform without the two calls dialect/zsh/internal/gatedfs/sys_unix.go uses, answering the way a
// shell with no such notion has to: nothing is asked about, and a removal is
// the standard library's.

func Writable(string) bool { return true }

func Unlink(path string) error { return os.Remove(path) }
